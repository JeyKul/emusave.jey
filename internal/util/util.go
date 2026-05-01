package util

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var invalidFileChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)

func EncodeBase64(s string) string {
	if s == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func DecodeBase64(s string) string {
	if s == "" {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}

func SanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	s = invalidFileChars.ReplaceAllString(s, "_")
	s = strings.ReplaceAll(s, "..", "_")
	if s == "" {
		return "Unknown Game"
	}
	return s
}

func BackupTimestamp() string {
	return time.Now().Format("2006-01-02_15-04-05")
}

func HomeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	if h := os.Getenv("USERPROFILE"); h != "" {
		return h
	}
	return "."
}

func ExpandUserPath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(HomeDir(), p[2:])
	}
	return p
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func DefaultRyujinxConfigPath() string {
	return filepath.Join(HomeDir(), ".config", "Ryujinx", "Config.json")
}
