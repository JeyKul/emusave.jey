package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Game struct {
	Key         string
	ROMPath     string
	TitleName   string
	TitleID     string
	SavePath    string
	SaveOptions []string
}

var titleIDRe = regexp.MustCompile(`(?i)\b(0100[0-9a-f]{12})\b`)

func ScanGameFiles(dirs []string) ([]Game, error) {
	var games []Game
	seen := map[string]struct{}{}

	for _, root := range dirs {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}

		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".nsp" && ext != ".xci" {
				return nil
			}

			if _, ok := seen[path]; ok {
				return nil
			}
			seen[path] = struct{}{}

			titleID := guessTitleID(path)
			key := titleID
			if key == "" {
				key = path
			}

			games = append(games, Game{
				Key:       key,
				ROMPath:   path,
				TitleName: guessTitleName(path),
				TitleID:   titleID,
			})
			return nil
		})
	}

	return games, nil
}

func guessTitleName(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.ReplaceAll(base, "_", " ")
	base = regexp.MustCompile(`\s+`).ReplaceAllString(base, " ")
	return strings.TrimSpace(base)
}

func guessTitleID(path string) string {
	m := titleIDRe.FindStringSubmatch(filepath.Base(path))
	if len(m) > 1 {
		return strings.ToUpper(m[1])
	}
	return ""
}
