package backup

import (
	"fmt"
	"os"
	"strings"
	"time"

	"emusave.jey/internal/appconfig"
	"emusave.jey/internal/scanner"
	"emusave.jey/internal/syncdest"
)

func Run(cfg *appconfig.Config, g scanner.Game) (string, error) {
	savePath := strings.TrimSpace(g.SavePath)
	if savePath == "" {
		return "", fmt.Errorf("no save path set for %s", g.TitleName)
	}

	st, err := os.Stat(savePath)
	if err != nil {
		return "", fmt.Errorf("save path does not exist: %s", savePath)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("save path is not a directory: %s", savePath)
	}

	target, err := syncdest.Run(cfg, syncdest.Request{
		TitleID:   g.TitleID,
		TitleName: g.TitleName,
		ROMPath:   g.ROMPath,
		SavePath:  g.SavePath,
	})
	if err != nil {
		return "", err
	}

	key := g.Key
	if key == "" {
		key = g.TitleID
	}
	if key == "" {
		key = g.ROMPath
	}

	m := cfg.Games[key]
	m.TitleID = g.TitleID
	m.TitleName = g.TitleName
	m.ROMPath = g.ROMPath
	m.ConfirmedSavePath = g.SavePath
	m.DetectedCandidates = append([]string{}, g.SaveOptions...)
	m.LastBackupTime = time.Now().Format(time.RFC3339)
	m.LastBackupTarget = target
	cfg.Games[key] = m

	return target, nil
}
