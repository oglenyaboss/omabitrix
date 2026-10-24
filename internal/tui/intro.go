package tui

import (
	"fmt"
	"image/color"
	"math"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

const (
	introDT   = 33 * time.Millisecond
	introHold = 72
	introOut  = 12

	ringW, ringH = 24, 13
	ringGap      = 0.14
	ringTh       = 0.16
	introLockGap = 4
	introBlockH  = 22
)

var (
	cBitrix = compat.AdaptiveColor{Light: lipgloss.Color("#0a8fc2"), Dark: lipgloss.Color("#2fc6f6")}
	cGlow   = compat.AdaptiveColor{Light: lipgloss.Color("#06324a"), Dark: lipgloss.Color("#ffffff")}
)

type introMsg struct{}

func introTick() tea.Cmd {
	return tea.Tick(introDT, func(time.Time) tea.Msg { return introMsg{} })
}

type intro struct {
	f    int
	got  [3]bool
	errs [3]string
	out  int
}

// PlayIntro — заставка при запуске (без deep-link).
func (m *Model) PlayIntro() { m.intro = &intro{} }

func (in intro) loaded() bool { return in.got[0] && in.got[1] && in.got[2] }

func (m Model) stepIntro() (tea.Model, tea.Cmd) {
	if m.intro == nil {
		return m, nil
	}
	in := *m.intro
	in.f++
	if in.out == 0 && in.f >= introHold && in.loaded() && m.ready {
		in.out = in.f
	}
	if in.out > 0 && in.f-in.out >= introOut {
		m.intro = nil
		return m, nil
	}
	m.intro = &in
	return m, introTick()
}

func (m *Model) introGot(msg reloadMsg) {
	if m.intro == nil {
		return
	}
	in := *m.intro
	for i, ok := range [3]bool{msg.chats, msg.tasks, msg.notify} {
		if ok {
			in.got[i] = true
			in.errs[i] = strings.Join(msg.errs, "; ")
		}
	}
	m.intro = &in
}

func (m Model) viewIntro() string {
	p := introPalette()
	lines := m.introFrame(p).lines()
	in := m.intro
	if in.out == 0 {
		return strings.Join(lines, "\n")
	}
	cut := int(math.Round(easeInOut(clamp01(float64(in.f-in.out)/introOut)) * float64(m.height)))
	screen := strings.Split(m.screen(), "\n")
	for y := 0; y < cut && y < len(lines); y++ {
		lines[y] = ""
		if y < len(screen) {
			lines[y] = screen[y]
		}
	}
	if cut < len(lines) {
		g := newGrid(m.width, 1)
		for x := 0; x < m.width; x++ {
			g.set(x, 0, '━', p.blue.mix(p.teal, float64(x)/float64(max(1, m.width-1))).mix(p.glow, 0.35))
		}
		lines[cut] = g.lines()[0]
	}
	return strings.Join(lines, "\n")
}

func (m Model) introFrame(p introPal) *grid {
	w, h, f := m.width, m.height, m.intro.f
	g := newGrid(w, h)
	word := Logo("omabitrix")
	wordW := len([]rune(word[0]))
	lockW := ringW + introLockGap + wordW
	items := m.introChecklist()

	switch {
	case w >= lockW+2 && h >= introBlockH:
		x0, y0 := (w-lockW)/2, (h-introBlockH)/2
		slide := easeInOut(clamp01(float64(f-22) / 12))
		ringX := int(math.Round(lerp(float64(w-ringW)/2, float64(x0), slide)))
		wx := x0 + ringW + introLockGap
		drawWordmark(g, wx, y0+3, word, f, p)
		drawRing(g, ringX, y0, f, p)
		drawTyped(g, wx+1, y0+10, f, p)
		drawChecklist(g, wx, y0+15, items, f, p)
		drawHint(g, wx, y0+20, f, p)
	case w >= ringW+2 && h >= introBlockH+1:
		y0 := (h - introBlockH - 1) / 2
		drawRing(g, (w-ringW)/2, y0, f, p)
		title := "o m a b i t r i x"
		drawTitle(g, (w-len([]rune(title)))/2, y0+14, title, f, p)
		drawTyped(g, (w-len([]rune(logoSub)))/2, y0+15, f, p)
		cx := max(1, (w-36)/2)
		drawChecklist(g, cx, y0+17, items, f, p)
		drawHint(g, cx, y0+22, f, p)
	default:
		drawTitle(g, 1, 0, "omabitrix", f, p)
		drawTyped(g, 1, 1, f, p)
		drawChecklist(g, 1, 3, items, f, p)
		drawHint(g, 1, 8, f, p)
	}
	return g
}

var brailleBit = [4][2]int{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

func drawRing(g *grid, ox, oy, f int, p introPal) {
	arc := (1 - ringGap) * 2 * math.Pi
	sweep := easeOut(clamp01(float64(f)/20)) * arc
	head := sweep
	if f > 20 {
		head = math.Mod(arc+float64(f-20)*0.09, 2*math.Pi)
	}
	pop := 0.0
	if f >= 20 {
		pop = easeBack(clamp01(float64(f-20) / 6))
	}
	cx, cy := float64(ringW), float64(ringH*2)
	ax, ay := cx-1.5, cy-1
	for row := 0; row < ringH; row++ {
		for col := 0; col < ringW; col++ {
			bits := 0
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					x := float64(col*2+dx) + 0.5 - cx
					y := float64(row*4+dy) + 0.5 - cy
					if ringDot(x, y, ax, ay, sweep) || arrowDot(x, y, ax, ay, arc, pop) {
						bits |= brailleBit[dy][dx]
					}
				}
			}
			if bits == 0 {
				continue
			}
			a := ringAngle(float64(col*2+1)-cx, float64(row*4+2)-cy)
			base := p.blue.mix(p.teal, (1-math.Cos(a))/2)
			tail := math.Mod(head-a+2*math.Pi, 2*math.Pi)
			glow := math.Pow(max(0, 1-tail/1.2), 2)
			g.set(ox+col, oy+row, rune(0x2800+bits), base.mix(p.glow, glow))
		}
	}
	digits := bigText("24", true)
	dx := (ringW - len([]rune(digits[0]))) / 2
	for r, line := range digits {
		for c, ch := range []rune(line) {
			if ch == ' ' {
				continue
			}
			px := r*7 + c/2
			at := 12 + (px*37+px*px*11)%12
			if f < at {
				continue
			}
			col := p.fg
			if f < at+3 {
				col = p.blue.mix(p.glow, 0.6)
			}
			g.set(ox+dx+c, oy+4+r, ch, col)
		}
	}
}

func ringAngle(x, y float64) float64 {
	a := math.Atan2(x, -y)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a
}

func ringDot(x, y, ax, ay, sweep float64) bool {
	d := math.Hypot(x/ax, y/ay)
	return d <= 1 && d >= 1-ringTh && ringAngle(x, y) <= sweep
}

func arrowDot(x, y, ax, ay, at, scale float64) bool {
	if scale <= 0 {
		return false
	}
	rm := 1 - ringTh/2
	px, py := ax*rm*math.Sin(at), -ay*rm*math.Cos(at)
	tx, ty := ax*math.Cos(at), ay*math.Sin(at)
	tl := math.Hypot(tx, ty)
	tx, ty = tx/tl, ty/tl
	nx, ny := math.Sin(at), -math.Cos(at)
	tipX, tipY := px+tx*8*scale, py+ty*8*scale
	b1x, b1y := px+nx*6*scale, py+ny*6*scale
	b2x, b2y := px-nx*6*scale, py-ny*6*scale
	s1 := cross(x, y, tipX, tipY, b1x, b1y)
	s2 := cross(x, y, b1x, b1y, b2x, b2y)
	s3 := cross(x, y, b2x, b2y, tipX, tipY)
	return (s1 >= 0 && s2 >= 0 && s3 >= 0) || (s1 <= 0 && s2 <= 0 && s3 <= 0)
}

func cross(px, py, ax, ay, bx, by float64) float64 {
	return (bx-ax)*(py-ay) - (by-ay)*(px-ax)
}

func drawWordmark(g *grid, ox, oy int, word []string, f int, p introPal) {
	wordW := len([]rune(word[0]))
	reveal := easeOut(clamp01(float64(f-30)/12)) * float64(wordW+2)
	if f < 30 {
		return
	}
	shine := -100.0
	if t := (f - 44) % 75; f >= 44 && t < 18 {
		shine = -8 + float64(t)*float64(wordW+16)/18
	}
	for r, line := range word {
		for c, ch := range []rune(line) {
			if ch == ' ' || float64(c) >= reveal {
				continue
			}
			base := p.blue.mix(p.teal, float64(c)/float64(wordW-1))
			if ch != '█' {
				base = base.mix(p.border, 0.45)
			}
			edge := max(0, 1-(reveal-float64(c))/3)
			band := max(0, 1-math.Abs(float64(c)+float64(r)*1.5-shine)/5) * 0.9
			g.set(ox+c, oy+r, ch, base.mix(p.glow, max(edge, band)))
		}
	}
}

func drawTitle(g *grid, ox, oy int, title string, f int, p introPal) {
	rs := []rune(title)
	n := int(float64(f-26) * 1.2)
	for i, ch := range rs {
		if i >= n {
			break
		}
		g.set(ox+i, oy, ch, p.blue.mix(p.teal, float64(i)/float64(max(1, len(rs)-1))))
	}
}

func drawTyped(g *grid, ox, oy, f int, p introPal) {
	rs := []rune(logoSub)
	n := int(float64(f-38) * 1.5)
	if n < 0 {
		return
	}
	brand := len([]rune("bitrix24"))
	for i, ch := range rs {
		if i >= n {
			break
		}
		col := p.dim
		if i < brand {
			col = p.blue
		}
		g.set(ox+i, oy, ch, col)
	}
	if n < len(rs) || (f/8)%2 == 0 {
		g.set(ox+min(n, len(rs)), oy, '▌', p.teal)
	}
}

type checkItem struct {
	label, value string
	state        int
}

const (
	checkWait = iota
	checkOK
	checkErr
)

func (m Model) introChecklist() [4]checkItem {
	in := m.intro
	host := m.api.Portal
	if u, err := url.Parse(m.api.Portal); err == nil && u.Host != "" {
		host = u.Host
	}
	portal := checkItem{label: "портал", value: host}
	var errs int
	for i := range in.got {
		if in.got[i] && in.errs[i] == "" {
			portal.state = checkOK
		}
		if in.errs[i] != "" {
			errs++
		}
	}
	if errs == len(in.got) {
		portal.state = checkErr
	}
	branch := func(i int, label, value string) checkItem {
		switch {
		case in.errs[i] != "":
			return checkItem{label, in.errs[i], checkErr}
		case in.got[i]:
			return checkItem{label, value, checkOK}
		}
		return checkItem{label, "…", checkWait}
	}
	return [4]checkItem{
		portal,
		branch(0, "чаты", fmt.Sprintf("%d · непрочитанных: %d", len(m.chats), m.unreadChats())),
		branch(1, "задачи", fmt.Sprintf("открыто: %d · просрочено: %d", len(m.tasks), len(m.buckets.Overdue))),
		branch(2, "уведы", fmt.Sprintf("новых: %d", m.notifyBadge())),
	}
}

var introSpin = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func drawChecklist(g *grid, ox, oy int, items [4]checkItem, f int, p introPal) {
	for i, it := range items {
		at := 42 + 3*i
		if f < at {
			continue
		}
		icon, ic, vc := introSpin[(f/2)%len(introSpin)], p.blue, p.dim
		switch it.state {
		case checkOK:
			icon, ic = '✓', p.teal
		case checkErr:
			icon, ic, vc = '✗', p.urgent, p.urgent
		}
		lc := p.fg
		if f < at+3 {
			ic, lc, vc = p.dim, p.dim, p.dim
		}
		g.set(ox, oy+i, icon, ic)
		g.text(ox+2, oy+i, it.label, lc)
		g.text(ox+11, oy+i, it.value, vc)
	}
}

func drawHint(g *grid, ox, oy, f int, p introPal) {
	if f >= 54 {
		g.text(ox, oy, "любая клавиша — пропустить", p.border)
	}
}

type rgb struct{ r, g, b uint8 }

func toRGB(c color.Color) rgb {
	r, g, b, _ := c.RGBA()
	return rgb{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

func (a rgb) mix(b rgb, t float64) rgb {
	t = clamp01(t)
	l := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return rgb{l(a.r, b.r), l(a.g, b.g), l(a.b, b.b)}
}

type introPal struct{ blue, teal, glow, fg, dim, border, urgent rgb }

func introPalette() introPal {
	return introPal{
		blue: toRGB(cBitrix), teal: toRGB(cAccent), glow: toRGB(cGlow),
		fg: toRGB(cFg), dim: toRGB(cDim), border: toRGB(cBorder), urgent: toRGB(cUrgent),
	}
}

type cell struct {
	ch    rune
	c     rgb
	bg    rgb
	hasBg bool
}

type grid struct {
	w, h  int
	cells []cell
}

func newGrid(w, h int) *grid {
	g := &grid{w: w, h: h, cells: make([]cell, w*h)}
	for i := range g.cells {
		g.cells[i].ch = ' '
	}
	return g
}

func (g *grid) set(x, y int, ch rune, c rgb) {
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return
	}
	cl := &g.cells[y*g.w+x]
	cl.ch, cl.c = ch, c
}

func (g *grid) fillBg(x0, y0, x1, y1 int, c rgb) {
	for y := max(y0, 0); y <= min(y1, g.h-1); y++ {
		for x := max(x0, 0); x <= min(x1, g.w-1); x++ {
			cl := &g.cells[y*g.w+x]
			cl.bg, cl.hasBg = c, true
		}
	}
}

func (g *grid) text(x, y int, s string, c rgb) {
	for _, ch := range s {
		g.set(x, y, ch, c)
		x++
	}
}

func (g *grid) lines() []string {
	out := make([]string, g.h)
	for y := range out {
		out[y] = g.span(y, 0, g.w)
	}
	return out
}

// span — кусок строки [x0, x1) с собственным сбросом стиля.
func (g *grid) span(y, x0, x1 int) string {
	var b strings.Builder
	var fg, bg rgb
	fgOn, bgOn := false, false
	for _, cl := range g.cells[y*g.w+x0 : y*g.w+x1] {
		if cl.hasBg != bgOn || (cl.hasBg && cl.bg != bg) {
			if cl.hasBg {
				fmt.Fprintf(&b, "\x1b[48;2;%d;%d;%dm", cl.bg.r, cl.bg.g, cl.bg.b)
			} else {
				b.WriteString("\x1b[49m")
			}
			bg, bgOn = cl.bg, cl.hasBg
		}
		if cl.ch != ' ' && (!fgOn || cl.c != fg) {
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm", cl.c.r, cl.c.g, cl.c.b)
			fg, fgOn = cl.c, true
		}
		b.WriteRune(cl.ch)
	}
	if fgOn || bgOn {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func clamp01(t float64) float64 { return max(0, min(1, t)) }

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func easeOut(t float64) float64 { return 1 - math.Pow(1-t, 3) }

func easeInOut(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2
}

func easeBack(t float64) float64 {
	const c = 1.70158
	return 1 + (c+1)*math.Pow(t-1, 3) + c*math.Pow(t-1, 2)
}
