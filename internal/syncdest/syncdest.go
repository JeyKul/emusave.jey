package syncdest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"emusave.jey/internal/appconfig"
	"emusave.jey/internal/smbclient"
	"emusave.jey/internal/util"
)

type BackupMeta struct {
	TitleID      string `json:"title_id"`
	TitleName    string `json:"title_name"`
	ROMPath      string `json:"rom_path"`
	SavePath     string `json:"save_path"`
	LastBackup   string `json:"last_backup"`
	BackupFolder string `json:"backup_folder"`
	Backend      string `json:"backend"`
}

type Request struct {
	TitleID   string
	TitleName string
	ROMPath   string
	SavePath  string
}

func Run(cfg *appconfig.Config, req Request) (string, error) {
	switch cfg.SyncMode {
	case appconfig.SyncModeSMB:
		return runSMB(cfg, req)
	case appconfig.SyncModeLocal:
		return runLocal(cfg, req)
	case appconfig.SyncModeCommand:
		return runCommand(cfg, req)
	default:
		return "", fmt.Errorf("unsupported sync mode: %s", cfg.SyncMode)
	}
}

func runSMB(cfg *appconfig.Config, req Request) (string, error) {
	password := util.DecodeBase64(cfg.SMB.PasswordBase64)
	client, err := smbclient.Connect(cfg.SMB, password)
	if err != nil {
		return "", err
	}
	defer client.Close()

	gameDir := util.SanitizeFileName(req.TitleName)
	timestamp := util.BackupTimestamp()
	base := path.Clean(path.Join(cfg.SMB.BaseDir, gameDir))
	backupDir := path.Join(base, timestamp)

	if err := client.CopyDir(req.SavePath, backupDir); err != nil {
		return "", err
	}

	meta := BackupMeta{
		TitleID:      req.TitleID,
		TitleName:    req.TitleName,
		ROMPath:      req.ROMPath,
		SavePath:     req.SavePath,
		LastBackup:   time.Now().Format(time.RFC3339),
		BackupFolder: backupDir,
		Backend:      "smb",
	}

	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", err
	}

	if err := client.WriteFile(path.Join(base, "game-config.json"), b); err != nil {
		return "", err
	}

	return fmt.Sprintf("\\\\%s\\%s\\%s", cfg.SMB.Host, cfg.SMB.Share, strings.ReplaceAll(backupDir, "/", `\`)), nil
}

func runLocal(cfg *appconfig.Config, req Request) (string, error) {
	basePath := util.ExpandUserPath(cfg.Local.BasePath)
	if strings.TrimSpace(basePath) == "" {
		return "", fmt.Errorf("local backup path is empty")
	}

	gameDir := util.SanitizeFileName(req.TitleName)
	timestamp := util.BackupTimestamp()
	base := filepath.Join(basePath, gameDir)
	backupDir := filepath.Join(base, timestamp)

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create local backup dir %s: %w", backupDir, err)
	}

	if err := copyDir(req.SavePath, backupDir); err != nil {
		return "", err
	}

	meta := BackupMeta{
		TitleID:      req.TitleID,
		TitleName:    req.TitleName,
		ROMPath:      req.ROMPath,
		SavePath:     req.SavePath,
		LastBackup:   time.Now().Format(time.RFC3339),
		BackupFolder: backupDir,
		Backend:      "local",
	}

	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(filepath.Join(base, "game-config.json"), b, 0o644); err != nil {
		return "", fmt.Errorf("write local game-config.json: %w", err)
	}

	return backupDir, nil
}

func runCommand(cfg *appconfig.Config, req Request) (string, error) {
	cmdStr := strings.TrimSpace(cfg.Command.Command)
	if cmdStr == "" {
		return "", fmt.Errorf("custom command is empty")
	}

	timestamp := util.BackupTimestamp()
	gameName := util.SanitizeFileName(req.TitleName)

	repl := map[string]string{
		"{save_path}":  req.SavePath,
		"{title_name}": gameName,
		"{title_id}":   req.TitleID,
		"{rom_path}":   req.ROMPath,
		"{timestamp}":  timestamp,
	}

	for k, v := range repl {
		cmdStr = strings.ReplaceAll(cmdStr, k, shellEscape(v))
	}

	cmd := exec.Command("sh", "-c", cmdStr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("custom command failed: %w\n%s", err, string(out))
	}

	return "custom-command:" + timestamp, nil
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func copyDir(srcDir, dstDir string) error {
	st, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("stat local save dir %s: %w", srcDir, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("save path is not a directory: %s", srcDir)
	}

	return filepath.Walk(srcDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		target := filepath.Join(dstDir, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		in, err := os.Open(p)
		if err != nil {
			return fmt.Errorf("open local file %s: %w", p, err)
		}
		defer in.Close()

		out, err := os.Create(target)
		if err != nil {
			return fmt.Errorf("create local target file %s: %w", target, err)
		}
		defer out.Close()

		if _, err := io.Copy(out, in); err != nil {
			return fmt.Errorf("copy file %s -> %s: %w", p, target, err)
		}

		return out.Chmod(info.Mode())
	})
}
