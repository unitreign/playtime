package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unitreign/playtime/internal/db"
	"github.com/unitreign/playtime/internal/ui"
)

const (
	appName  = "playtime"
	hookFile = "playtime-hook.sh"
	version  = "0.1.0"
)

func main() {
	if len(os.Args) < 2 {
		runUI()
		return
	}
	switch os.Args[1] {
	case "session":
		runSession(os.Args[2:])
	case "hooks":
		runHooks(os.Args[2:])
	case "uninstall":
		runUninstall()
	case "version":
		fmt.Printf("%s %s\n", appName, version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runUI() {
	store, err := openStore()
	if err != nil {
		die("open db: %v", err)
	}
	defer store.Close()

	romsRoot := romsDir()
	if err := ui.Run(store, romsRoot); err != nil {
		die("ui: %v", err)
	}
}

func runSession(args []string) {
	if len(args) < 3 {
		die("usage: playtime session <start|end> <system> <rom_path> [rom_name]")
	}
	action, system, romPath := args[0], args[1], args[2]
	romName := filepath.Base(romPath)
	if len(args) >= 4 {
		romName = args[3]
	}

	store, err := openStore()
	if err != nil {
		// Must not crash ES
		os.Exit(0)
	}
	defer store.Close()

	switch action {
	case "start":
		store.StartSession(system, romPath, romName)
	case "end":
		store.EndSession(romPath)
	default:
		die("unknown session action: %s", action)
	}
}

func runHooks(args []string) {
	if len(args) == 0 {
		die("usage: playtime hooks <install|repair|remove>")
	}
	switch args[0] {
	case "install", "repair":
		installHooks(selfDir(), scriptsPath())
	case "remove":
		removeHooks(scriptsPath())
	default:
		die("unknown hooks action: %s", args[0])
	}
}

// installHooks writes a single dedicated hook script into the ES scripts dir.
// Batocera/Knulli ES calls every script in scripts/ for every event:
//   $1=action  $2=system  $3=emulator  $4=rom_path  $5=gameinfo_json
func installHooks(appDir, scriptsDir string) {
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		die("mkdir %s: %v", scriptsDir, err)
	}

	bin := filepath.Join(appDir, appName)

	hook := fmt.Sprintf(`#!/bin/sh
# Managed by PlayTime — do not edit manually.
case "$1" in
  gameStart)
    "%s" session start "$2" "$4" "$(basename "$4")" &
    ;;
  gameStop)
    "%s" session end "$2" "$4" "$(basename "$4")"
    ;;
esac
`, bin, bin)

	dest := filepath.Join(scriptsDir, hookFile)
	existing, _ := os.ReadFile(dest)
	if string(existing) == hook {
		// Already up to date
	} else {
		if err := os.WriteFile(dest, []byte(hook), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "hook write %s: %v\n", dest, err)
		}
	}

	updateGamelist(appDir)
	fmt.Println("hooks installed")
}

func removeHooks(scriptsDir string) {
	path := filepath.Join(scriptsDir, hookFile)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "remove hook: %v\n", err)
	}
	fmt.Println("hooks removed")
}

func runUninstall() {
	removeHooks(scriptsPath())
	fmt.Printf("uninstalled. data remains at %s\n", dataDir())
	fmt.Println("delete that folder manually to wipe all play history.")
}

// --- gamelist.xml ---

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

	// Update if entry exists, otherwise append
	found := false
	for i, g := range gl.Games {
		if g.Path == entry.Path {
			gl.Games[i] = entry
			found = true
			break
		}
	}
	if !found {
		gl.Games = append(gl.Games, entry)
	}

	out, err := xml.MarshalIndent(gl, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(glPath, append([]byte(xml.Header), out...), 0644)
}

// --- paths ---

func cfw() string {
	return strings.ToUpper(os.Getenv("CFW"))
}

func dataDir() string {
	_ = cfw()
	return "/userdata/system/configs/playtime"
}

func romsDir() string {
	_ = cfw()
	return "/userdata/roms"
}

func scriptsPath() string {
	_ = cfw()
	return "/userdata/system/scripts"
}

func selfDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func openStore() (*db.Store, error) {
	return db.Open(dataDir())
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
