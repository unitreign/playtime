package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/unitreign/playtime/internal/gamelist"
	"github.com/veandco/go-sdl2/img"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

var embeddedFont []byte

const (
	minDuration   = 10
	maxRows       = 8
	fps           = 60
	titleMaxChars = 20 // names longer than this scroll
	tickerSpeed   = 3  // frames per pixel of scroll
	tickerPause   = 90 // pixels of "pause" before scrolling starts
)

var fontCandidates = []string{
	"./font.ttf",
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/TTF/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/freefont/FreeSans.ttf",
}

type Screen struct {
	Label     string
	Games     []gamelist.GameStats
	TotalSecs int
}

type state struct {
	screens   []Screen
	screenIdx int
	scroll    int
	tab       int // 0=Games 1=Stats

	tickerTick  int
	tickerShift int

	covers map[string]*sdl.Texture

	win  *sdl.Window
	rend *sdl.Renderer
	font *ttf.Font
	sm   *ttf.Font
	w, h int32
}

// colors
var (
	colTitle    = sdl.Color{R: 225, G: 225, B: 225, A: 255}
	colSubtitle = sdl.Color{R: 110, G: 110, B: 110, A: 255}
	colAccent   = sdl.Color{R: 200, G: 200, B: 200, A: 255}
	colDim      = sdl.Color{R: 75, G: 75, B: 75, A: 255}
	colTab      = sdl.Color{R: 200, G: 200, B: 200, A: 255}
	colTabDim   = sdl.Color{R: 60, G: 60, B: 60, A: 255}
)

func Run(romsRoot string, fontData []byte) error {
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

	for i := 0; i < sdl.NumJoysticks(); i++ {
		if sdl.IsGameController(i) {
			sdl.GameControllerOpen(i)
		}
	}

	w, h := win.GetSize()
	baseFontSize := max(18, int(float32(h)*0.05))
	font, err := openFont(baseFontSize)
	if err != nil {
		return fmt.Errorf("open font: %w", err)
	}
	defer font.Close()

	smSize := max(14, int(float32(baseFontSize)*0.8))
	sm, err := openFont(smSize)
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

	if err := s.loadData(romsRoot); err != nil {
		return err
	}

	return s.loop()
}

func (s *state) loadData(romsRoot string) error {
	allGames, err := gamelist.AllGames(romsRoot, minDuration)
	if err != nil {
		return err
	}

	for _, g := range allGames {
		if g.CoverPath != "" {
			if tex, err := img.LoadTexture(s.rend, g.CoverPath); err == nil {
				s.covers[g.RomPath] = tex
			}
		}
	}

	sideScreens := map[string]string{
		"mpv":         "Videos",
		"sh":          "Tools",
		"tools":       "Tools",
		"odcommander": "Tools",
	}

	type sideData struct {
		games []gamelist.GameStats
		total int
	}
	sidesByLabel := map[string]*sideData{}
	bySystem := map[string][]gamelist.GameStats{}

	var games []gamelist.GameStats
	gamesTotal := 0
	for _, g := range allGames {
		sideLabel := ""
		if label, ok := sideScreens[g.System]; ok {
			sideLabel = label
		} else if strings.HasPrefix(g.RomPath, "/userdata/roms/tools/") {
			sideLabel = "Tools"
		}

		if sideLabel != "" {
			if sidesByLabel[sideLabel] == nil {
				sidesByLabel[sideLabel] = &sideData{}
			}
			sidesByLabel[sideLabel].games = append(sidesByLabel[sideLabel].games, g)
			sidesByLabel[sideLabel].total += g.TotalSecs
		} else {
			games = append(games, g)
			gamesTotal += g.TotalSecs
			bySystem[g.System] = append(bySystem[g.System], g)
		}
	}

	s.screens = []Screen{{
		Label:     "All Games",
		Games:     games,
		TotalSecs: gamesTotal,
	}}

	seen := map[string]bool{}
	for _, g := range games {
		if seen[g.System] {
			continue
		}
		seen[g.System] = true
		sysGames := bySystem[g.System]
		total := 0
		for _, sg := range sysGames {
			total += sg.TotalSecs
		}
		s.screens = append(s.screens, Screen{
			Label:     g.System,
			Games:     sysGames,
			TotalSecs: total,
		})
	}

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
				case sdl.K_RETURN:
					s.switchTab()
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
				case sdl.CONTROLLER_BUTTON_BACK:
					s.switchTab()
				case sdl.CONTROLLER_BUTTON_B, sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_START:
					return nil
				}
			}
		}

		// Advance ticker
		s.tickerTick++
		if s.tickerTick >= tickerSpeed {
			s.tickerTick = 0
			s.tickerShift++
		}

		s.rend.SetDrawColor(10, 12, 20, 255)
		s.rend.Clear()
		s.render()
		s.rend.Present()

		sdl.Delay(1000 / fps)
	}
}

func (s *state) render() {
	headerH := max(s.h/10, int32(28))
	footerH := max(s.h/16, int32(20))
	rowsH := s.h - headerH - footerH
	adaptiveMinRowH := max(s.h/6, int32(40))
	rowH := int32(rowsH) / int32(clamp(int(rowsH)/int(adaptiveMinRowH), 1, maxRows))
	visible := int(rowsH) / int(rowH)

	s.renderHeader(headerH)

	if s.tab == 0 {
		s.renderRows(headerH, rowH, visible)
	} else {
		s.renderStats(headerH, rowsH)
	}

	s.renderFooter(footerH, visible)
}

func (s *state) renderHeader(h int32) {
	s.rend.SetDrawColor(10, 12, 20, 255)
	s.rend.FillRect(&sdl.Rect{X: 0, Y: 0, W: s.w, H: h})
	s.rend.SetDrawColor(20, 24, 38, 255)
	s.rend.DrawLine(0, h-1, s.w, h-1)

	pad := s.w / 64

	// Left: app name
	s.drawText(s.sm, "PLAYTIME", pad, h/2, true, colDim)

	// Right: current screen label
	scr := s.screens[s.screenIdx]
	lw, _, _ := s.sm.SizeUTF8(scr.Label)
	s.drawText(s.sm, scr.Label, s.w-pad-int32(lw), h/2, true, colAccent)
}

func (s *state) renderRows(offsetY, rowH int32, visible int) {
	scr := s.screens[s.screenIdx]
	games := scr.Games

	if len(games) == 0 {
		midY := offsetY + (s.h-offsetY)/2
		msg := "No play sessions recorded yet."
		sub := "Play a game — it will appear here."
		mw, _, _ := s.font.SizeUTF8(msg)
		sw, _, _ := s.sm.SizeUTF8(sub)
		s.drawText(s.font, msg, s.w/2-int32(mw)/2, midY-int32(s.font.Height()), false, sdl.Color{R: 130, G: 130, B: 130, A: 255})
		s.drawText(s.sm, sub, s.w/2-int32(sw)/2, midY+4, false, colDim)
		return
	}

	end := s.scroll + visible
	if end > len(games) {
		end = len(games)
	}
	for i, g := range games[s.scroll:end] {
		y := offsetY + int32(i)*rowH
		s.renderRow(g, y, rowH)
	}
}

func (s *state) renderRow(g gamelist.GameStats, y, rowH int32) {
	pad := s.w / 64

	// Separator
	s.rend.SetDrawColor(15, 18, 32, 255)
	s.rend.DrawLine(0, y+rowH-1, s.w, y+rowH-1)

	// --- Layout ---
	coverSize := rowH - rowH/6
	coverX := int32(s.w/22) + pad
	coverY := y + (rowH-coverSize)/2

	nameX := coverX + coverSize + pad
	nameAreaW := s.w / 3

	// 3 stat columns, right-aligned from screen edge
	rightEdge := s.w - pad
	colW := (rightEdge - (nameX + nameAreaW + pad)) / 3
	col2Right := rightEdge
	col1Right := col2Right - colW
	col0Right := col1Right - colW

	// Vertical alignment: title row and subtitle row
	titleY := y + rowH/2 - int32(s.font.Height())/2 - 1
	subtitleY := titleY + int32(s.font.Height()) + 2

	// --- Cover ---
	if tex, ok := s.covers[g.RomPath]; ok {
		_, _, imgW, imgH, _ := tex.Query()
		dstW, dstH := coverSize, coverSize
		if imgW > 0 && imgH > 0 {
			if imgH >= imgW {
				dstW = int32(float32(imgW) * float32(coverSize) / float32(imgH))
			} else {
				dstH = int32(float32(imgH) * float32(coverSize) / float32(imgW))
			}
		}
		dstX := coverX + (coverSize-dstW)/2
		dstY := coverY + (coverSize-dstH)/2
		s.rend.Copy(tex, nil, &sdl.Rect{X: dstX, Y: dstY, W: dstW, H: dstH})
	} else {
		s.rend.SetDrawColor(30, 35, 55, 255)
		s.rend.FillRect(&sdl.Rect{X: coverX, Y: coverY, W: coverSize, H: coverSize})
	}

	// --- Name (clipped, scrolls if > titleMaxChars) ---
	fontH := int32(s.font.Height())
	clipRect := sdl.Rect{X: nameX, Y: titleY, W: nameAreaW, H: fontH}
	s.rend.SetClipRect(&clipRect)

	shift := int32(0)
	if len([]rune(g.RomName)) > titleMaxChars {
		textW, _, _ := s.font.SizeUTF8(g.RomName)
		maxShift := int32(textW) - nameAreaW
		if maxShift > 0 {
			cycle := int(maxShift) + tickerPause
			raw := s.tickerShift % cycle
			if raw > tickerPause {
				shift = int32(raw - tickerPause)
				if shift > maxShift {
					shift = maxShift
				}
			}
		}
	}
	s.drawText(s.font, g.RomName, nameX-shift, titleY, false, colTitle)
	s.rend.SetClipRect(nil)

	// --- Console (subtitle) ---
	s.drawText(s.sm, g.System, nameX, subtitleY, false, colSubtitle)

	// --- Stat columns (right side) ---
	// Column 0: Total Time
	totalStr := formatDuration(g.TotalSecs)
	tw, _, _ := s.font.SizeUTF8(totalStr)
	s.drawText(s.font, totalStr, col0Right-int32(tw), titleY, false, colTitle)
	lw, _, _ := s.sm.SizeUTF8("Total")
	s.drawText(s.sm, "Total", col0Right-int32(lw), subtitleY, false, colSubtitle)

	// Column 1: Avg Time
	avgStr := formatDuration(g.AvgSecs)
	aw, _, _ := s.font.SizeUTF8(avgStr)
	s.drawText(s.font, avgStr, col1Right-int32(aw), titleY, false, colTitle)
	alw, _, _ := s.sm.SizeUTF8("Avg")
	s.drawText(s.sm, "Avg", col1Right-int32(alw), subtitleY, false, colSubtitle)

	// Column 2: Times opened
	opensStr := fmt.Sprintf("%d", g.Plays)
	ow, _, _ := s.font.SizeUTF8(opensStr)
	s.drawText(s.font, opensStr, col2Right-int32(ow), titleY, false, colTitle)
	olw, _, _ := s.sm.SizeUTF8("Opened")
	s.drawText(s.sm, "Opened", col2Right-int32(olw), subtitleY, false, colSubtitle)
}

func (s *state) renderStats(offsetY, areaH int32) {
	totalSecs, games, consoles, opens, avgSecs := s.statsData()

	pad := s.w / 64
	x := pad * 4

	type stat struct {
		value string
		label string
	}
	stats := []stat{
		{formatDuration(totalSecs), "Total Play Time"},
		{fmt.Sprintf("%d", games), "Games Played"},
		{fmt.Sprintf("%d", consoles), "Consoles"},
		{fmt.Sprintf("%d", opens), "Times Opened"},
		{formatDuration(avgSecs), "Avg Session"},
	}

	// 2-column grid
	colW := s.w / 2
	rowH := areaH / 3
	for i, st := range stats {
		col := int32(i % 2)
		row := int32(i / 2)
		cx := x + col*colW
		cy := offsetY + row*rowH + rowH/4

		s.drawText(s.font, st.value, cx, cy, false, colTitle)
		s.drawText(s.sm, st.label, cx, cy+int32(s.font.Height())+4, false, colSubtitle)
	}
}

func (s *state) statsData() (totalSecs, games, consoles, opens, avgSecs int) {
	if len(s.screens) == 0 {
		return
	}
	allGames := s.screens[0].Games
	totalSecs = s.screens[0].TotalSecs
	games = len(allGames)
	for _, g := range allGames {
		opens += g.Plays
	}
	if opens > 0 {
		avgSecs = totalSecs / opens
	}
	skip := map[string]bool{"All Games": true, "Videos": true, "Tools": true}
	for _, scr := range s.screens {
		if !skip[scr.Label] {
			consoles++
		}
	}
	return
}

func (s *state) renderFooter(h int32, visible int) {
	y := s.h - h
	s.rend.SetDrawColor(20, 24, 38, 255)
	s.rend.DrawLine(0, y, s.w, y)

	pad := s.w / 64

	// Tabs
	gamesColor := colTabDim
	statsColor := colTabDim
	if s.tab == 0 {
		gamesColor = colTab
	} else {
		statsColor = colTab
	}

	gw, _, _ := s.sm.SizeUTF8("Games")
	s.drawText(s.sm, "Games", pad, y+h/2, true, gamesColor)
	s.drawText(s.sm, "Stats", pad+int32(gw)+pad*3, y+h/2, true, statsColor)

	// Right: game count (Games tab only)
	if s.tab == 0 && len(s.screens) > 0 {
		scr := s.screens[s.screenIdx]
		var pos string
		if len(scr.Games) == 0 {
			pos = "0 games"
		} else {
			pos = fmt.Sprintf("%d / %d", s.scroll+1, len(scr.Games))
		}
		pw, _, _ := s.sm.SizeUTF8(pos)
		s.drawText(s.sm, pos, s.w-pad-int32(pw), y+h/2, true, colDim)
	}
}

// --- navigation ---

func (s *state) scrollUp() {
	if s.scroll > 0 {
		s.scroll--
		s.resetTicker()
	}
}

func (s *state) scrollDown() {
	games := s.screens[s.screenIdx].Games
	if s.scroll < len(games)-1 {
		s.scroll++
		s.resetTicker()
	}
}

func (s *state) prevScreen() {
	if len(s.screens) < 2 {
		return
	}
	s.screenIdx = (s.screenIdx - 1 + len(s.screens)) % len(s.screens)
	s.scroll = 0
	s.resetTicker()
}

func (s *state) nextScreen() {
	if len(s.screens) < 2 {
		return
	}
	s.screenIdx = (s.screenIdx + 1) % len(s.screens)
	s.scroll = 0
	s.resetTicker()
}

func (s *state) switchTab() {
	if s.tab == 0 {
		s.tab = 1
	} else {
		s.tab = 0
	}
	s.resetTicker()
}

func (s *state) resetTicker() {
	s.tickerShift = 0
	s.tickerTick = 0
}

// --- helpers ---

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
	for _, p := range fontCandidates {
		if _, err := os.Stat(p); err == nil {
			return ttf.OpenFont(p, size)
		}
	}
	return nil, fmt.Errorf("no font available")
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

