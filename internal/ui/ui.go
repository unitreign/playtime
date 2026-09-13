package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/unitreign/playtime/internal/db"
	"github.com/unitreign/playtime/internal/gamelist"
	"github.com/veandco/go-sdl2/img"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// embeddedFont is set by main via Run's fontData parameter.
var embeddedFont []byte

const (
	minDuration = 10 // sessions under 10s are ignored
	maxRows     = 8  // cap for very tall outputs (HDMI)
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
	dm, err := sdl.GetDesktopDisplayMode(0)
	if err != nil {
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

	// Open all connected game controllers so ControllerButtonEvents are generated.
	for i := 0; i < sdl.NumJoysticks(); i++ {
		if sdl.IsGameController(i) {
			sdl.GameControllerOpen(i)
		}
	}

	w, h := win.GetSize()

	// 0.05×h with an 18px floor — slightly larger for readability on small screens.
	baseFontSize := max(18, int(float32(h)*0.05))
	font, err := openFont(baseFontSize)
	if err != nil {
		return fmt.Errorf("open font: %w", err)
	}
	defer font.Close()

	sm, err := openFont(max(14, int(float32(baseFontSize)*0.8)))
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

	systems, err := store.Systems(minDuration)
	if err != nil {
		return err
	}

	gamelists, _ := gamelist.LoadAll(romsRoot)

	// Override stored romName with scraped title and preload covers for every game.
	for i, g := range allGames {
		if entry, ok := gamelists[g.RomPath]; ok {
			if entry.Name != "" {
				allGames[i].RomName = entry.Name
			}
			if coverPath := entry.CoverPath(filepath.Dir(g.RomPath)); coverPath != "" {
				if tex, err := img.LoadTexture(s.rend, coverPath); err == nil {
					s.covers[g.RomPath] = tex
				}
			}
		}
	}

	// Systems routed to their own side screen, excluded from All Games.
	// Multiple system keys can share a label — they merge into one screen.
	sideScreens := map[string]string{
		"mpv":   "Videos",
		"sh":    "Tools",
		"tools": "Tools",
	}

	type sideData struct{ games []db.GameStats; total int }
	sidesByLabel := map[string]*sideData{}

	var games []db.GameStats
	gamesTotal := 0
	for _, g := range allGames {
		if label, ok := sideScreens[g.System]; ok {
			if sidesByLabel[label] == nil {
				sidesByLabel[label] = &sideData{}
			}
			sidesByLabel[label].games = append(sidesByLabel[label].games, g)
			sidesByLabel[label].total += g.TotalSecs
		} else {
			games = append(games, g)
			gamesTotal += g.TotalSecs
		}
	}

	s.screens = make([]Screen, 0, 2+len(systems))
	s.screens = append(s.screens, Screen{
		Label:     "All Games",
		Games:     games,
		TotalSecs: gamesTotal,
	})

	for _, sys := range systems {
		if _, excluded := sideScreens[sys]; excluded {
			continue
		}
		sysGames, _ := store.TopGamesBySystem(sys, minDuration)
		for i, g := range sysGames {
			if entry, ok := gamelists[g.RomPath]; ok && entry.Name != "" {
				sysGames[i].RomName = entry.Name
			}
		}
		total := 0
		for _, g := range sysGames {
			total += g.TotalSecs
		}
		s.screens = append(s.screens, Screen{
			Label:     sys,
			Games:     sysGames,
			TotalSecs: total,
		})
	}

	// Side screens appended last in stable order.
	for _, label := range []string{"Videos", "Tools"} {
		if d := sidesByLabel[label]; d != nil && len(d.games) > 0 {
			s.screens = append(s.screens, Screen{
				Label:     label,
				Games:     d.games,
				TotalSecs: d.total,
			})
		}
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

			case *sdl.ControllerDeviceEvent:
				if e.Type == sdl.CONTROLLERDEVICEADDED {
					sdl.GameControllerOpen(int(e.Which))
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
				// B=east, A=south — Anbernic physical "B" (cancel) may report as either
				// depending on driver mapping, so accept both. Start also exits.
				case sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_START:
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

	// Adaptive header/footer — at least 28/20px so text isn't cramped on 272p screens.
	headerH := max(s.h/10, int32(28))
	footerH := max(s.h/16, int32(20))
	rowsH := s.h - headerH - footerH

	// Adaptive row height: s.h/6 gives ~45px on 272p (5 rows) and ~80px on 480p (5 rows).
	adaptiveMinRowH := max(s.h/6, int32(40))
	rowH := int32(rowsH) / int32(clamp(int(rowsH)/int(adaptiveMinRowH), 1, maxRows))
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
	s.drawText(s.sm, "PLAYTIME", pad, h/2, true, sdl.Color{R: 160, G: 160, B: 160, A: 255})

	// Nav indicator — truly centered, ASCII arrows (safe for any embedded font)
	if len(s.screens) > 1 {
		label := fmt.Sprintf("< %s >", scr.Label)
		lw, _, _ := s.font.SizeUTF8(label)
		s.drawText(s.font, label, s.w/2-int32(lw)/2, h/2, true, sdl.Color{R: 200, G: 200, B: 200, A: 255})
	}

	// Total time — right
	timeStr := formatDuration(scr.TotalSecs)
	tw, _, _ := s.sm.SizeUTF8(timeStr)
	s.drawText(s.sm, timeStr, s.w-pad-int32(tw), h/2, true, sdl.Color{R: 200, G: 200, B: 200, A: 255})
}

func (s *state) renderRows(scr Screen, offsetY, rowH int32, visible int) {
	games := scr.Games

	if len(games) == 0 {
		midY := offsetY + (s.h-offsetY)/2
		msg := "No play sessions recorded yet."
		sub := "Play a game — it will appear here after the session ends."
		mw, _, _ := s.font.SizeUTF8(msg)
		sw, _, _ := s.sm.SizeUTF8(sub)
		s.drawText(s.font, msg, s.w/2-int32(mw)/2, midY-int32(s.font.Height()), false, sdl.Color{R: 130, G: 130, B: 130, A: 255})
		s.drawText(s.sm, sub, s.w/2-int32(sw)/2, midY+4, false, sdl.Color{R: 80, G: 80, B: 80, A: 255})
		return
	}

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
	s.drawText(s.sm, rankStr, rankW-pad, y+rowH/2, true, sdl.Color{R: 70, G: 70, B: 70, A: 255})

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

	s.drawText(s.font, g.RomName, infoX, nameY, false, sdl.Color{R: 225, G: 225, B: 225, A: 255})
	s.drawText(s.sm, fmt.Sprintf("%s · avg %s", g.System, formatDuration(g.AvgSecs)),
		infoX, sysY, false, sdl.Color{R: 110, G: 110, B: 110, A: 255})

	// Stats — right aligned
	timeStr := formatDuration(g.TotalSecs)
	playsStr := fmt.Sprintf("%d plays", g.Plays)

	tw, _, _ := s.font.SizeUTF8(timeStr)
	pw, _, _ := s.sm.SizeUTF8(playsStr)
	rightEdge := s.w - pad

	timeY := y + rowH/2 - int32(s.font.Height())/2 - 1
	playsY := timeY + int32(s.font.Height()) + 2

	s.drawText(s.font, timeStr, rightEdge-int32(tw), timeY, false, sdl.Color{R: 225, G: 225, B: 225, A: 255})
	s.drawText(s.sm, playsStr, rightEdge-int32(pw), playsY, false, sdl.Color{R: 110, G: 110, B: 110, A: 255})
}

func (s *state) renderFooter(scr Screen, h int32, visible int) {
	y := s.h - h
	s.rend.SetDrawColor(20, 24, 38, 255)
	s.rend.DrawLine(0, y, s.w, y)

	pad := int32(s.w) / 64

	s.drawText(s.sm, "B  exit    L  R  switch", pad, y+h/2, true, sdl.Color{R: 75, G: 75, B: 75, A: 255})

	var pos string
	if len(scr.Games) == 0 {
		pos = "0 games"
	} else {
		pos = fmt.Sprintf("%d / %d", s.scroll+1, len(scr.Games))
	}
	pw, _, _ := s.sm.SizeUTF8(pos)
	s.drawText(s.sm, pos, s.w-pad-int32(pw), y+h/2, true, sdl.Color{R: 75, G: 75, B: 75, A: 255})
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
	if len(s.screens) < 2 {
		return
	}
	s.screenIdx = (s.screenIdx - 1 + len(s.screens)) % len(s.screens)
	s.scroll = 0
}

func (s *state) nextScreen() {
	if len(s.screens) < 2 {
		return
	}
	s.screenIdx = (s.screenIdx + 1) % len(s.screens)
	s.scroll = 0
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
		rw, err := sdl.RWFromMem(embeddedFont)
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
