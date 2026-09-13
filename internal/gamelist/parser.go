package gamelist

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Game struct {
	Path      string `xml:"path"`
	Name      string `xml:"name"`
	Image     string `xml:"image"`
	Thumbnail string `xml:"thumbnail"`
	GameTime  int    `xml:"gametime"`
	PlayCount int    `xml:"playcount"`
}

// GameStats is a resolved, display-ready summary for one game.
type GameStats struct {
	RomPath   string
	RomName   string
	System    string
	TotalSecs int
	Plays     int
	AvgSecs   int
	CoverPath string // absolute path, empty if none found
}

type gameList struct {
	Games []Game `xml:"game"`
}

func (g *Game) coverPath(romDir string) string {
	for _, rel := range []string{g.Image, g.Thumbnail} {
		if rel == "" {
			continue
		}
		if abs := resolve(rel, romDir); fileExists(abs) {
			return abs
		}
	}
	return ""
}

func loadSystem(romDir, system string, minDuration int) ([]GameStats, error) {
	f, err := os.Open(filepath.Join(romDir, "gamelist.xml"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var gl gameList
	if err := xml.NewDecoder(f).Decode(&gl); err != nil {
		return nil, err
	}

	var out []GameStats
	for _, g := range gl.Games {
		if g.GameTime <= minDuration {
			continue
		}
		romPath := resolve(g.Path, romDir)
		name := g.Name
		if name == "" {
			base := filepath.Base(romPath)
			name = strings.TrimSuffix(base, filepath.Ext(base))
		}
		avg := 0
		if g.PlayCount > 0 {
			avg = g.GameTime / g.PlayCount
		}
		out = append(out, GameStats{
			RomPath:   romPath,
			RomName:   name,
			System:    system,
			TotalSecs: g.GameTime,
			Plays:     g.PlayCount,
			AvgSecs:   avg,
			CoverPath: g.coverPath(romDir),
		})
	}
	return out, nil
}

// AllGames returns all games with gametime > minDuration across every system
// directory under romsRoot, sorted by total play time descending.
func AllGames(romsRoot string, minDuration int) ([]GameStats, error) {
	entries, err := os.ReadDir(romsRoot)
	if err != nil {
		return nil, err
	}

	var all []GameStats
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		system := e.Name()
		games, err := loadSystem(filepath.Join(romsRoot, system), system, minDuration)
		if err != nil {
			continue // no gamelist.xml or unreadable — skip silently
		}
		all = append(all, games...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].TotalSecs > all[j].TotalSecs
	})
	return all, nil
}

func resolve(path, romDir string) string {
	switch {
	case filepath.IsAbs(path):
		return path
	case strings.HasPrefix(path, "./"):
		return filepath.Join(romDir, path[2:])
	case strings.HasPrefix(path, "~/"):
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	default:
		return filepath.Join(romDir, path)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
