package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unitreign/playtime/internal/gamelist"
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

// updateGamelist adds/updates the PlayTime entry in the tools gamelist.xml.
func updateGamelist(appDir string) {
	toolsDir := filepath.Dir(appDir) // /userdata/roms/tools/
	folderName := filepath.Base(appDir)
	err := gamelist.Register(filepath.Join(toolsDir, "gamelist.xml"), gamelist.Entry{
		Path:      "./" + folderName + "/playtime.sh",
		Name:      "PlayTime",
		Desc:      "Local gameplay time tracker. See how long you've played every game.",
		Image:     "./" + folderName + "/icons/playtime.png",
		Thumbnail: "./" + folderName + "/icons/playtime.png",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "gamelist: %v\n", err)
	}
}

// --- paths ---

func romsDir() string {
	if d := os.Getenv("PLAYTIME_ROMS"); d != "" {
		return d
	}
	return "/userdata/roms"
}

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
