package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"emusave.jey/internal/util"
)

type SyncMode string

const (
	SyncModeSMB     SyncMode = "smb"
	SyncModeLocal   SyncMode = "local"
	SyncModeCommand SyncMode = "command"
)

type SMBConfig struct {
	Host           string `json:"host"`
	Share          string `json:"share"`
	BaseDir        string `json:"base_dir"`
	Username       string `json:"username"`
	PasswordBase64 string `json:"password_base64"`
	Domain         string `json:"domain"`
}

type LocalConfig struct {
	BasePath string `json:"base_path"`
}

type CommandConfig struct {
	Command string `json:"command"`
}

type GameMapping struct {
	TitleID            string   `json:"title_id"`
	TitleName          string   `json:"title_name"`
	ROMPath            string   `json:"rom_path"`
	DetectedCandidates []string `json:"detected_candidates"`
	ConfirmedSavePath  string   `json:"confirmed_save_path"`
	LastBackupTime     string   `json:"last_backup_time"`
	LastBackupTarget   string   `json:"last_backup_target"`
}

type Config struct {
	FirstRunCompleted bool                   `json:"first_run_completed"`
	RyujinxConfigPath string                 `json:"ryujinx_config_path"`
	RyujinxDataRoot   string                 `json:"ryujinx_data_root"`
	GameDirs          []string               `json:"game_dirs"`
	SyncMode          SyncMode               `json:"sync_mode"`
	SMB               SMBConfig              `json:"smb"`
	Local             LocalConfig            `json:"local"`
	Command           CommandConfig          `json:"command"`
	Games             map[string]GameMapping `json:"games"`
}

func DefaultConfig() *Config {
	return &Config{
		SyncMode: SyncModeSMB,
		Games:    map[string]GameMapping{},
	}
}

func ConfigDir() string {
	return filepath.Join(util.HomeDir(), ".config", "emusave.jey")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func Load() (*Config, error) {
	p := ConfigPath()
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	if cfg.Games == nil {
		cfg.Games = map[string]GameMapping{}
	}
	cfg.RyujinxConfigPath = strings.TrimSpace(cfg.RyujinxConfigPath)
	cfg.RyujinxDataRoot = strings.TrimSpace(cfg.RyujinxDataRoot)
	cfg.Local.BasePath = strings.TrimSpace(cfg.Local.BasePath)
	cfg.Command.Command = strings.TrimSpace(cfg.Command.Command)
	return cfg, nil
}

func Save(cfg *Config) error {
	if err := util.EnsureDir(ConfigDir()); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), b, 0o600)
}
