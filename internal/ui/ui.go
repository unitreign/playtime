package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/unitreign/playtime/internal/db"
	"github.com/unitreign/playtime/internal/gamelist"
	"github.com/veandco/go-sdl2/img"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// embeddedFont is set by main via Run's fontData parameter.
var embeddedFont []byte

const (
	minDuration = 10  // sessions under 10s are ignored
	minRowH     = 86  // minimum row height in pixels (targets ~5 rows on 480p)
	maxRows     = 8   // cap for very tall outputs (HDMI)
	fps         = 60
)

// fontCandidates — searched in order, first hit wins.
var fontCandidates = []string{
	"./font.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/TTF/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/freefont/FreeSans.ttf",
}

// Screen represents one L/R navigation view.
type Screen struct {
	Label     string // "All Games" | system name
	Games     []db.GameStats
	TotalSecs int
}

type state struct {
	screens   []Screen
	screenIdx int // current L/R position
	scroll    int // top visible row index

	covers map[string]*sdl.Texture // rom_path → loaded texture

	win  *sdl.Window
	rend *sdl.Renderer
	font *ttf.Font
	sm   *ttf.Font // small font
	w, h int32
}

func Run(store *db.Store, romsRoot string, fontData []byte) error {
	embeddedFont = fontData
	if err := sdl.Init(sdl.INIT_VIDEO | sdl.INIT_GAMECONTROLLER); err != nil {
		return fmt.Errorf("sdl init: %w", err)
	}
	defer sdl.Quit()

	if err := ttf.Init(); err != nil {
		return fmt.Errorf("ttf init: %w", err)
	}
	defer ttf.Quit()

	if err := img.Init(img.INIT_PNG | img.INIT_JPG); err != nil {
		return fmt.Errorf("img init: %w", err)
	}
	defer img.Quit()

	// Detect real display resolution
	var dm sdl.DisplayMode
	if err := sdl.GetDesktopDisplayMode(0, &dm); err != nil {
		dm.W, dm.H = 640, 480
	}

	win, err := sdl.CreateWindow(
		"PlayTime",
		sdl.WINDOWPOS_UNDEFINED, sdl.WINDOWPOS_UNDEFINED,
		dm.W, dm.H,
		sdl.WINDOW_FULLSCREEN_DESKTOP,
	)
	if err != nil {
		return fmt.Errorf("create window: %w", err)
	}
	defer win.Destroy()

	rend, err := sdl.CreateRenderer(win, -1, sdl.RENDERER_ACCELERATED|sdl.RENDERER_PRESENTVSYNC)
	if err != nil {
		return fmt.Errorf("create renderer: %w", err)
	}
	defer rend.Destroy()

	w, h := win.GetSize()

	baseFontSize := int(float32(h) * 0.033) // ~16px at 480p
	font, err := openFont(baseFontSize)
	if err != nil {
		return fmt.Errorf("open font: %w", err)
	}
	defer font.Close()

	sm, err := openFont(int(float32(baseFontSize) * 0.7))
	if err != nil {
		return fmt.Errorf("open small font: %w", err)
	}
	defer sm.Close()

	s := &state{
		win: win, rend: rend,
		font: font, sm: sm,
		w: int32(w), h: int32(h),
		covers: make(map[string]*sdl.Texture),
	}

	if err := s.loadData(store, romsRoot); err != nil {
		return err
	}

	return s.loop()
}

func (s *state) loadData(store *db.Store, romsRoot string) error {
	allGames, err := store.TopGames(minDuration)
	if err != nil {
		return err
	}
	totalSecs, _ := store.TotalSecs(minDuration)

	systems, err := store.Systems(minDuration)
	if err != nil {
		return err
	}

	// Load gamelists for cover art
	gamelists, _ := gamelist.LoadAll(romsRoot)

	// Pre-load cover textures for all games
	for _, g := range allGames {
		if gl, ok := gamelists[g.RomPath]; ok {
			coverPath := gl.CoverPath(filepath.Dir(g.RomPath))
			if coverPath != "" {
				if tex, err := img.LoadTexture(s.rend, coverPath); err == nil {
					s.covers[g.RomPath] = tex
				}
			}
		}
	}

	s.screens = make([]Screen, 0, 1+len(systems))
	s.screens = append(s.screens, Screen{
		Label:     "All Games",
		Games:     allGames,
		TotalSecs: totalSecs,
	})

	for _, sys := range systems {
		games, _ := store.TopGamesBySystem(sys, minDuration)
		total := 0
		for _, g := range games {
			total += g.TotalSecs
		}
		s.screens = append(s.screens, Screen{
			Label:     sys,
			Games:     games,
			TotalSecs: total,
		})
	}
	return nil
}

func (s *state) loop() error {
	for {
		// --- input ---
		for ev := sdl.PollEvent(); ev != nil; ev = sdl.PollEvent() {
			switch e := ev.(type) {
			case *sdl.QuitEvent:
				return nil

			case *sdl.KeyboardEvent:
				if e.Type != sdl.KEYDOWN {
					continue
				}
				switch e.Keysym.Sym {
				case sdl.K_UP:
					s.scrollUp()
				case sdl.K_DOWN:
					s.scrollDown()
				case sdl.K_LEFT:
					s.prevScreen()
				case sdl.K_RIGHT:
					s.nextScreen()
				case sdl.K_ESCAPE, sdl.K_b:
					return nil
				}

			case *sdl.ControllerButtonEvent:
				if e.Type != sdl.CONTROLLERBUTTONDOWN {
					continue
				}
				switch e.Button {
				case sdl.CONTROLLER_BUTTON_DPAD_UP:
					s.scrollUp()
				case sdl.CONTROLLER_BUTTON_DPAD_DOWN:
					s.scrollDown()
				case sdl.CONTROLLER_BUTTON_LEFTSHOULDER:
					s.prevScreen()
				case sdl.CONTROLLER_BUTTON_RIGHTSHOULDER:
					s.nextScreen()
				case sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_START:
					return nil
				}
			}
		}

		// --- render ---
		s.rend.SetDrawColor(10, 12, 20, 255) // #0A0C14
		s.rend.Clear()
		s.render()
		s.rend.Present()

		sdl.Delay(1000 / fps)
	}
}

func (s *state) render() {
	scr := s.screens[s.screenIdx]

	headerH := s.h / 17 // ~28px at 480p
	footerH := s.h / 24 // ~20px at 480p
	rowsH := s.h - headerH - footerH

	rowH := int32(rowsH) / int32(clamp(int(rowsH)/minRowH, 1, maxRows))
	visible := int(rowsH) / int(rowH)

	s.renderHeader(scr, headerH)
	s.renderRows(scr, headerH, rowH, visible)
	s.renderFooter(scr, footerH, visible)
}

func (s *state) renderHeader(scr Screen, h int32) {
	// Background bar
	s.rend.SetDrawColor(10, 12, 20, 255)
	s.rend.FillRect(&sdl.Rect{X: 0, Y: 0, W: s.w, H: h})

	// Separator line
	s.rend.SetDrawColor(20, 24, 38, 255)
	s.rend.DrawLine(0, h-1, s.w, h-1)

	pad := int32(s.w) / 64

	// Title — left
	s.drawText(s.sm, "PLAYTIME", pad, h/2, true, sdl.Color{R: 184, G: 148, B: 60, A: 255})

	// Nav indicator — center
	if len(s.screens) > 1 {
		label := fmt.Sprintf("◂  %s  ▸", scr.Label)
		s.drawText(s.sm, label, s.w/2, h/2, true, sdl.Color{R: 46, G: 52, B: 72, A: 255})
	}

	// Total time — right
	timeStr := formatDuration(scr.TotalSecs)
	tw, _ := s.sm.SizeUTF8(timeStr)
	s.drawText(s.sm, timeStr, s.w-pad-int32(tw), h/2, false, sdl.Color{R: 184, G: 148, B: 60, A: 255})
}

func (s *state) renderRows(scr Screen, offsetY, rowH int32, visible int) {
	games := scr.Games
	end := s.scroll + visible
	if end > len(games) {
		end = len(games)
	}

	for i, g := range games[s.scroll:end] {
		y := offsetY + int32(i)*rowH
		rank := s.scroll + i + 1
		s.renderRow(g, rank, y, rowH)
	}
}

func (s *state) renderRow(g db.GameStats, rank int, y, rowH int32) {
	pad := int32(s.w) / 64

	// Separator
	s.rend.SetDrawColor(15, 18, 32, 255)
	s.rend.DrawLine(0, y+rowH-1, s.w, y+rowH-1)

	// Rank number
	rankStr := fmt.Sprintf("%d", rank)
	rankW := int32(s.w) / 22 // fixed rank column width
	s.drawText(s.sm, rankStr, rankW-pad, y+rowH/2, true, sdl.Color{R: 37, G: 40, B: 64, A: 255})

	// Cover
	coverSize := rowH - rowH/6
	coverX := rankW + pad
	coverY := y + (rowH-coverSize)/2
	coverRect := &sdl.Rect{X: coverX, Y: coverY, W: coverSize, H: coverSize}

	if tex, ok := s.covers[g.RomPath]; ok {
		s.rend.Copy(tex, nil, coverRect)
	} else {
		// Placeholder: filled rect
		s.rend.SetDrawColor(30, 35, 55, 255)
		s.rend.FillRect(coverRect)
	}

	// Info column
	infoX := coverX + coverSize + pad
	nameY := y + rowH/2 - int32(s.font.Height())/2 - 1
	sysY := nameY + int32(s.font.Height()) + 2

	s.drawText(s.font, g.RomName, infoX, nameY, false, sdl.Color{R: 200, G: 196, B: 186, A: 255})
	s.drawText(s.sm, fmt.Sprintf("%s · avg %s", g.System, formatDuration(g.AvgSecs)),
		infoX, sysY, false, sdl.Color{R: 46, G: 51, B: 72, A: 255})

	// Stats — right aligned
	timeStr := formatDuration(g.TotalSecs)
	playsStr := fmt.Sprintf("%d plays", g.Plays)

	tw, _ := s.font.SizeUTF8(timeStr)
	pw, _ := s.sm.SizeUTF8(playsStr)
	rightEdge := s.w - pad

	timeY := y + rowH/2 - int32(s.font.Height())/2 - 1
	playsY := timeY + int32(s.font.Height()) + 2

	s.drawText(s.font, timeStr, rightEdge-int32(tw), timeY, false, sdl.Color{R: 216, G: 212, B: 202, A: 255})
	s.drawText(s.sm, playsStr, rightEdge-int32(pw), playsY, false, sdl.Color{R: 37, G: 40, B: 64, A: 255})
}

func (s *state) renderFooter(scr Screen, h int32, visible int) {
	y := s.h - h
	s.rend.SetDrawColor(20, 24, 38, 255)
	s.rend.DrawLine(0, y, s.w, y)

	pad := int32(s.w) / 64

	s.drawText(s.sm, "B  exit", pad, y+h/2, false, sdl.Color{R: 32, G: 36, B: 56, A: 255})

	pos := fmt.Sprintf("%d / %d", s.scroll+1, len(scr.Games))
	pw, _ := s.sm.SizeUTF8(pos)
	s.drawText(s.sm, pos, s.w-pad-int32(pw), y+h/2, false, sdl.Color{R: 32, G: 36, B: 56, A: 255})
}

// drawText renders a UTF-8 string. vertCenter=true centers vertically on y.
func (s *state) drawText(font *ttf.Font, text string, x, y int32, vertCenter bool, color sdl.Color) {
	surf, err := font.RenderUTF8Blended(text, color)
	if err != nil {
		return
	}
	defer surf.Free()

	tex, err := s.rend.CreateTextureFromSurface(surf)
	if err != nil {
		return
	}
	defer tex.Destroy()

	dst := &sdl.Rect{X: x, Y: y, W: surf.W, H: surf.H}
	if vertCenter {
		dst.Y = y - surf.H/2
	}
	s.rend.Copy(tex, nil, dst)
}

// --- navigation ---

func (s *state) scrollUp() {
	if s.scroll > 0 {
		s.scroll--
	}
}

func (s *state) scrollDown() {
	games := s.screens[s.screenIdx].Games
	if s.scroll < len(games)-1 {
		s.scroll++
	}
}

func (s *state) prevScreen() {
	if s.screenIdx > 0 {
		s.screenIdx--
		s.scroll = 0
	}
}

func (s *state) nextScreen() {
	if s.screenIdx < len(s.screens)-1 {
		s.screenIdx++
		s.scroll = 0
	}
}

// --- helpers ---

func formatDuration(secs int) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	m := secs / 60
	if m < 60 {
		return fmt.Sprintf("%dm %ds", m, secs%60)
	}
	h := m / 60
	return fmt.Sprintf("%dh %02dm", h, m%60)
}

// openFont loads the embedded font first; falls back to filesystem candidates.
func openFont(size int) (*ttf.Font, error) {
	if len(embeddedFont) > 0 {
		rw, err := sdl.RWFromMem(unsafe.Pointer(&embeddedFont[0]), len(embeddedFont))
		if err == nil {
			f, err := ttf.OpenFontRW(rw, 1, size)
			if err == nil {
				return f, nil
			}
		}
	}
	// Filesystem fallback
	for _, p := range fontCandidates {
		if _, err := os.Stat(p); err == nil {
			return ttf.OpenFont(p, size)
		}
	}
	return nil, fmt.Errorf("no font available; embed failed and no fallback found")
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
