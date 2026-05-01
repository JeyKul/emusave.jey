package ryu

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type ConfigFile struct {
	GameDirs []string `json:"game_dirs"`
}

type scoredPath struct {
	Path  string
	Score int
}

func ParseConfig(configPath string) (*ConfigFile, error) {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg ConfigFile
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}

	for i := range cfg.GameDirs {
		cfg.GameDirs[i] = strings.TrimSpace(cfg.GameDirs[i])
	}
	return &cfg, nil
}

func GuessDefaultDataRoot() string {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "Ryujinx")
		}
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "Ryujinx")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "Ryujinx")
}

func SaveRoot(dataRoot string) string {
	return filepath.Join(dataRoot, "bis", "user", "save")
}

func DiscoverAllSaveCandidates(saveRoot string) ([]string, error) {
	var matches []string

	if _, err := os.Stat(saveRoot); err != nil {
		return matches, err
	}

	err := filepath.Walk(saveRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || !info.IsDir() {
			return nil
		}
		if filepath.Base(path) == "0" {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(matches)
	return unique(matches), nil
}

func DetectSaveCandidatesForGame(saveRoot string, titleID string, romPath string) ([]string, error) {
	all, err := DiscoverAllSaveCandidates(saveRoot)
	if err != nil {
		return nil, err
	}

	titleID = strings.TrimSpace(strings.ToUpper(titleID))
	romBase := strings.ToLower(strings.TrimSuffix(filepath.Base(romPath), filepath.Ext(romPath)))

	scored := make([]scoredPath, 0, len(all))
	for _, p := range all {
		parent := strings.ToUpper(filepath.Base(filepath.Dir(p)))
		score := 0

		if titleID != "" && parent == titleID {
			score += 100
		}

		if strings.Contains(strings.ToLower(p), romBase) {
			score += 10
		}

		scored = append(scored, scoredPath{
			Path:  p,
			Score: score,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Path < scored[j].Path
		}
		return scored[i].Score > scored[j].Score
	})

	out := make([]string, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.Path)
	}
	return unique(out), nil
}

func OpaqueSaveFolderName(path string) string {
	return filepath.Base(filepath.Dir(path))
}

func ParseOpaqueSaveFolderName(path string) int64 {
	v, _ := strconv.ParseInt(OpaqueSaveFolderName(path), 16, 64)
	return v
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
