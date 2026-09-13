package gamelist

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

type Game struct {
	Path      string `xml:"path"`
	Name      string `xml:"name"`
	Thumbnail string `xml:"thumbnail"` // box art — preferred
	Image     string `xml:"image"`     // screenshot/mix — fallback
}

type gameList struct {
	Games []Game `xml:"game"`
}

// CoverPath returns the absolute path to the best available cover art.
// Prefers <image> (full art) over <thumbnail> (small box art).
func (g *Game) CoverPath(romDir string) string {
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

// LoadSystem parses gamelist.xml in romDir and returns a map of
// absolute rom path → *Game.
func LoadSystem(romDir string) (map[string]*Game, error) {
	f, err := os.Open(filepath.Join(romDir, "gamelist.xml"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var gl gameList
	if err := xml.NewDecoder(f).Decode(&gl); err != nil {
		return nil, err
	}

	m := make(map[string]*Game, len(gl.Games))
	for i := range gl.Games {
		g := &gl.Games[i]
		m[resolve(g.Path, romDir)] = g
	}
	return m, nil
}

// LoadAll walks romsRoot (e.g. /userdata/roms), loads every system's
// gamelist.xml, and returns a merged map of absolute rom path → *Game.
func LoadAll(romsRoot string) (map[string]*Game, error) {
	entries, err := os.ReadDir(romsRoot)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*Game)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		games, err := LoadSystem(filepath.Join(romsRoot, e.Name()))
		if err != nil {
			continue // no gamelist.xml — skip silently
		}
		for k, v := range games {
			result[k] = v
		}
	}
	return result, nil
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
