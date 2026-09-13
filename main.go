package main

import (
	_ "embed"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unitreign/playtime/internal/ui"
)

//go:embed font.ttf
var fontData []byte

const (
	appName = "playtime"
	version = "1.0.0"
)

func main() {
	if len(os.Args) < 2 {
		runUI()
		return
	}
	switch os.Args[1] {
	case "hooks":
		// Registers PlayTime in the tools gamelist.xml so it appears in Knulli's menu.
		installHooks(selfDir())
	case "version":
		fmt.Printf("%s %s\n", appName, version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runUI() {
	if err := ui.Run(romsDir(), fontData); err != nil {
		die("ui: %v", err)
	}
}

func installHooks(appDir string) {
	updateGamelist(appDir)
	fmt.Println("ok")
}

// --- gamelist.xml registration ---

type gamelistXML struct {
	XMLName xml.Name  `xml:"gameList"`
	Games   []gameXML `xml:"game"`
}

type gameXML struct {
	Path      string `xml:"path"`
	Name      string `xml:"name"`
	Desc      string `xml:"desc"`
	Image     string `xml:"image"`
	Thumbnail string `xml:"thumbnail"`
}

// updateGamelist adds/updates the PlayTime entry in the tools gamelist.xml.
// Skips the write if the entry is already correct — avoids triggering a
// Knulli reload on every launch, which causes a first-launch crash.
func updateGamelist(appDir string) {
	toolsDir := filepath.Dir(appDir) // /userdata/roms/tools/
	glPath := filepath.Join(toolsDir, "gamelist.xml")
	folderName := filepath.Base(appDir) // "PlayTime"

	entry := gameXML{
		Path:      "./" + folderName + "/playtime.sh",
		Name:      "PlayTime",
		Desc:      "Local gameplay time tracker. See how long you've played every game.",
		Image:     "./" + folderName + "/icons/playtime.png",
		Thumbnail: "./" + folderName + "/icons/playtime.png",
	}

	var gl gamelistXML
	data, err := os.ReadFile(glPath)
	if err == nil {
		xml.Unmarshal(data, &gl)
	}

	found := false
	changed := false
	for i, g := range gl.Games {
		if g.Path == entry.Path {
			if g != entry {
				gl.Games[i] = entry
				changed = true
			}
			found = true
			break
		}
	}
	if !found {
		gl.Games = append(gl.Games, entry)
		changed = true
	}
	if !changed {
		return
	}

	out, err := xml.MarshalIndent(gl, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(glPath, append([]byte(xml.Header), out...), 0644)
}

// --- paths ---

func romsDir() string { return "/userdata/roms" }

func selfDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
