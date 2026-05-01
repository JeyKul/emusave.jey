package smbclient

import (
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hirochachacha/go-smb2"

	"emusave.jey/internal/appconfig"
)

type Client struct {
	conn net.Conn
	sess *smb2.Session
	fs   *smb2.Share
	cfg  appconfig.SMBConfig
}

type RemoteEntry struct {
	Name  string
	Path  string
	IsDir bool
}

func Connect(cfg appconfig.SMBConfig, password string) (*Client, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, fmt.Errorf("SMB host is empty")
	}
	if strings.TrimSpace(cfg.Share) == "" {
		return nil, fmt.Errorf("SMB share is empty")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return nil, fmt.Errorf("SMB username is empty")
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(cfg.Host, "445"), 8*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dial SMB host %s: %w", cfg.Host, err)
	}

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     cfg.Username,
			Password: password,
			Domain:   cfg.Domain,
		},
	}

	sess, err := d.Dial(conn)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("authenticate to SMB: %w", err)
	}

	fs, err := sess.Mount(cfg.Share)
	if err != nil {
		_ = sess.Logoff()
		_ = conn.Close()
		return nil, fmt.Errorf("mount share %s: %w", cfg.Share, err)
	}

	return &Client{
		conn: conn,
		sess: sess,
		fs:   fs,
		cfg:  cfg,
	}, nil
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	if c.fs != nil {
		_ = c.fs.Umount()
	}
	if c.sess != nil {
		_ = c.sess.Logoff()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func cleanRemote(parts ...string) string {
	items := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ReplaceAll(p, "\\", "/")
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "/")
		if p != "" {
			items = append(items, p)
		}
	}
	if len(items) == 0 {
		return "."
	}
	return path.Clean(strings.Join(items, "/"))
}

func (c *Client) MkdirAll(remoteDir string) error {
	remoteDir = cleanRemote(remoteDir)
	if remoteDir == "." {
		return nil
	}

	parts := strings.Split(remoteDir, "/")
	cur := ""
	for _, p := range parts {
		if cur == "" {
			cur = p
		} else {
			cur = cur + "/" + p
		}

		err := c.fs.Mkdir(cur, 0o755)
		if err != nil {
			if _, statErr := c.fs.Stat(cur); statErr != nil {
				return fmt.Errorf("create remote directory %s: %w", cur, err)
			}
		}
	}
	return nil
}

func (c *Client) WriteFile(remotePath string, data []byte) error {
	remotePath = cleanRemote(remotePath)
	if err := c.MkdirAll(path.Dir(remotePath)); err != nil {
		return err
	}

	f, err := c.fs.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote file %s: %w", remotePath, err)
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("write remote file %s: %w", remotePath, err)
	}
	return nil
}

func (c *Client) CopyDir(localDir, remoteDir string) error {
	remoteDir = cleanRemote(remoteDir)

	st, err := os.Stat(localDir)
	if err != nil {
		return fmt.Errorf("stat local save dir %s: %w", localDir, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("local save path is not a directory: %s", localDir)
	}

	if err := c.MkdirAll(remoteDir); err != nil {
		return err
	}

	return filepath.Walk(localDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return err
		}

		rel, err := filepath.Rel(localDir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		remotePath := cleanRemote(remoteDir, filepath.ToSlash(rel))

		if info.IsDir() {
			return c.MkdirAll(remotePath)
		}

		if err := c.MkdirAll(path.Dir(remotePath)); err != nil {
			return err
		}

		src, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open local file %s: %w", p, err)
		}
		defer src.Close()

		dst, err := c.fs.Create(remotePath)
		if err != nil {
			return fmt.Errorf("create remote file %s: %w", remotePath, err)
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		if err != nil {
			return fmt.Errorf("copy %s to SMB %s: %w", p, remotePath, err)
		}
		return nil
	})
}

func (c *Client) TestWrite(baseDir string) error {
	testDir := cleanRemote(baseDir, ".emusave_test")
	testFile := cleanRemote(testDir, "write-test.txt")

	if err := c.MkdirAll(testDir); err != nil {
		return err
	}
	if err := c.WriteFile(testFile, []byte("emusave.jey test")); err != nil {
		return err
	}
	return nil
}

func (c *Client) ListDir(remoteDir string) ([]RemoteEntry, error) {
	remoteDir = cleanRemote(remoteDir)
	if remoteDir == "." {
		remoteDir = ""
	}

	entries, err := c.fs.ReadDir(remoteDir)
	if err != nil {
		return nil, fmt.Errorf("read remote dir %s: %w", remoteDir, err)
	}

	var out []RemoteEntry
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		if !e.IsDir() {
			continue
		}

		full := cleanRemote(remoteDir, name)
		if full == "." {
			full = ""
		}

		out = append(out, RemoteEntry{
			Name:  name,
			Path:  full,
			IsDir: e.IsDir(),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	return out, nil
}
