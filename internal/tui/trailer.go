package tui

import (
	"context"
	"fmt"
	"image"
	"math"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// TrailerPing — когда демо-портал присылает сообщение от Анны: сразу после
// ответа в «Команде продукта», чтобы тост попал в кадр.
const TrailerPing = 26 * time.Second

// Таймлайн трейлера, секунды от старта.
const (
	atDesk    = 5.6
	atToast   = 6.4
	atUntoast = 8.3
	atKeys    = 8.0
	atOpen    = 9.2
	atClose   = 39.4
	atRoll    = 40.0
	atOutro   = 46.2

	rollStep = 0.62

	hookPx = 36
	capPx  = 18
	rollPx = 44
	keyPx  = 22
)

var trBg = rgb{0x0d, 0x12, 0x15}

type trailerCap struct {
	from, to   float64
	word, desc string
}

var trailerCaps = []trailerCap{
	{6.6, 9.2, "демон", "omabitrixd следит за порталом: тосты и счётчик в баре"},
	{9.4, 12.4, "старт", "окно поверх всего, данные грузятся под заставку"},
	{12.4, 16.2, "пульт", "весь день на одном экране: время, ближайший срок, канбан, неделя"},
	{16.2, 21.6, "задачи", "канбан по стадиям: L двигает карточку, enter открывает"},
	{21.6, 26.7, "чаты", "ответить, не снимая рук с клавиатуры"},
	{26.7, 29.7, "тосты", "новое сообщение всплывает поверх любой вкладки"},
	{29.7, 35.7, "картинки", "превью прямо в истории и на весь экран, и это всё ещё терминал"},
	{35.7, atClose, "уведы", "уведомления с переходом в задачу"},
}

var trailerRoll = []struct{ word, desc string }{
	{"поиск", "по всей истории чата"},
	{"лайки", "ответы-цитаты, правка, удаление"},
	{"файлы", "скачать, открыть, отправить с подписью"},
	{"ctrl+v", "скриншот из буфера сразу в чат"},
	{"мышь", "клики, колесо, перетаскивание карточек"},
	{"сроки", "дедлайны и просрочки на пульте"},
	{"тишина", "dnd-окно для тостов"},
	{"вебхук", "один входящий вебхук, без oauth и серверов"},
	{"-setup", "установка одной командой"},
}

type trailerStep struct {
	at   float64
	key  string
	text string
}

type pressed struct {
	label string
	at    float64
}

type trailerMsg time.Time

func trailerTick() tea.Cmd {
	return tea.Tick(introDT, func(t time.Time) tea.Msg { return trailerMsg(t) })
}

// Trailer — ролик для README целиком в терминале: кинетическая типографика
// символами и живая модель на демо-портале, которой сценарий жмёт клавиши.
type Trailer struct {
	app     Model
	appOn   bool
	w, h    int
	start   time.Time
	t       float64
	steps   []trailerStep
	next    int
	keys    []pressed
	badge   int
	badgeAt float64
}

func NewTrailer(api *API, ctx context.Context) Trailer {
	app := NewModel(api, ctx)
	app.PlayIntro()
	return Trailer{app: app, steps: trailerScript(), badge: 2}
}

func trailerScript() []trailerStep {
	var s []trailerStep
	key := func(at float64, keys ...string) {
		for i, k := range keys {
			s = append(s, trailerStep{at: at + float64(i)*0.35, key: k})
		}
	}
	typ := func(at float64, text string) {
		for i, r := range []rune(text) {
			s = append(s, trailerStep{at: at + float64(i)*0.055, text: string(r)})
		}
	}
	key(16.3, "2")
	key(17.3, "j")
	key(18.1, "L")
	key(19.6, "enter")
	key(21.0, "esc")
	key(21.8, "3", "g", "j")
	key(23.2, "enter")
	key(24.3, "r")
	typ(24.7, "Гляну пуши до обеда 👌")
	key(26.3, "enter")
	key(29.8, "esc", "esc", "g")
	key(31.0, "enter")
	key(32.4, "v", "k", "k")
	key(33.7, "enter")
	key(35.2, "esc", "esc")
	key(36.0, "4")
	key(36.9, "j")
	key(37.5, "j")
	return s
}

func trailerKey(st trailerStep) tea.KeyPressMsg {
	switch st.key {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	s := st.key + st.text
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func (tr Trailer) Init() tea.Cmd { return trailerTick() }

func (tr Trailer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case trailerMsg:
		return tr.advance(time.Time(msg))
	case tea.WindowSizeMsg:
		tr.w, tr.h = msg.Width, msg.Height
		if !tr.appOn {
			return tr, nil
		}
		tr, cmd := tr.toApp(tr.appSize())
		return tr, cmd
	case tea.KeyPressMsg:
		if s := msg.String(); s == "q" || s == "esc" || s == "ctrl+c" {
			return tr, tea.Quit
		}
		return tr, nil
	case tea.MouseMsg:
		return tr, nil
	}
	if !tr.appOn {
		return tr, nil
	}
	tr, cmd := tr.toApp(msg)
	return tr, cmd
}

func (tr Trailer) toApp(msg tea.Msg) (Trailer, tea.Cmd) {
	mm, cmd := tr.app.Update(msg)
	tr.app = mm.(Model)
	return tr, cmd
}

func (tr Trailer) appSize() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: tr.w - 6, Height: tr.h - 10}
}

func (tr Trailer) advance(now time.Time) (tea.Model, tea.Cmd) {
	if tr.start.IsZero() {
		tr.start = now
	}
	tr.t = now.Sub(tr.start).Seconds()
	cmds := []tea.Cmd{trailerTick()}
	if !tr.appOn && tr.t >= atOpen && tr.w > 0 {
		tr.appOn = true
		var cmd tea.Cmd
		tr, cmd = tr.toApp(tr.appSize())
		cmds = append(cmds, cmd, tr.app.Init())
	}
	for tr.appOn && tr.next < len(tr.steps) && tr.steps[tr.next].at <= tr.t {
		st := tr.steps[tr.next]
		tr.next++
		if st.key != "" {
			tr.keys = append(tr.keys, pressed{st.key, tr.t})
		}
		var cmd tea.Cmd
		tr, cmd = tr.toApp(trailerKey(st))
		cmds = append(cmds, cmd)
	}
	if n := tr.badgeCount(); n != tr.badge {
		tr.badge, tr.badgeAt = n, tr.t
	}
	return tr, tea.Batch(cmds...)
}

func (tr Trailer) badgeCount() int {
	switch {
	case tr.appOn && tr.app.ready:
		return tr.app.unreadChats()
	case tr.t >= atToast:
		return 3
	}
	return 2
}

func (tr Trailer) View() tea.View {
	v := tea.NewView(tr.render())
	v.AltScreen = true
	return v
}

func (tr Trailer) render() string {
	if tr.w <= 0 || tr.h <= 0 {
		return ""
	}
	p := introPalette()
	g := newGrid(tr.w, tr.h)
	back, front := newDots(tr.w, tr.h), newDots(tr.w, tr.h)
	frame := int(tr.t * 30)
	switch {
	case tr.t < atDesk:
		if tr.t < atDesk-0.1 {
			tr.drawHook(g, front, p)
			front.flush(g)
			glitch(g, easeIn(clamp01((tr.t-5.05)/0.45)), frame, p)
		}
	case tr.t < atRoll:
		tr.drawWallpaper(back, p)
		date := tr.drawDeskClock(back, p)
		back.flush(g)
		date(g)
		tr.drawBar(g, p)
		tr.drawDeskToast(g, p)
		crop, open := tr.drawWindow(g, p)
		tr.drawSuperB(g, front, p)
		tr.drawCaption(g, front, p, frame)
		tr.drawKeys(g, p)
		front.flush(g)
		lines := g.lines()
		if open && tr.appOn {
			tr.splice(lines, g, crop)
		}
		return strings.Join(lines, "\n")
	case tr.t < atOutro:
		tr.drawRoll(g, front, p, frame)
		front.flush(g)
	default:
		tr.drawOutro(g, p)
	}
	return strings.Join(g.lines(), "\n")
}

// --- хук: bitrix24 / в браузере → в терминале ---

func (tr Trailer) drawHook(g *grid, d *dotBuf, p introPal) {
	t := tr.t
	top := textMask("bitrix24", hookPx)
	was := textMask("в браузере", hookPx)
	now := textMask("в терминале", hookPx)
	oy := (d.h - (top.h*2 + 6 + 12)) / 2
	x1, xa, xb := (d.w-top.w)/2, (d.w-was.w)/2, (d.w-now.w)/2
	y1, y2 := oy, oy+top.h+6

	if t < 0.45 {
		if int(t*5)%2 == 0 {
			g.set(x1/2, (y1+top.base)/4-1, '█', p.teal)
		}
		return
	}
	for _, pt := range top.pts {
		q := clamp01((t - 0.45 - float64(pt[0])/float64(top.w)*0.5) / 0.35)
		if q == 0 {
			continue
		}
		e := easeOut(q)
		c := p.blue.mix(p.teal, float64(pt[0])/float64(top.w)*0.4).mix(p.glow, 1-e)
		d.set(x1+pt[0], y1+pt[1]-int(math.Round((1-e)*18)), trBg.mix(c, e))
	}

	dim := p.fg.mix(p.dim, 0.35)
	if t < 3.0 {
		adv := float64(was.w) / float64(len([]rune("в браузере")))
		for _, pt := range was.pts {
			at := 1.4 + math.Floor(float64(pt[0])/adv)*0.07
			if t >= at {
				d.set(xa+pt[0], y2+pt[1], dim.mix(p.glow, 1-clamp01((t-at)/0.2)))
			}
		}
	}
	if sv := 1 - clamp01((t-3.0)/0.2); t >= 2.5 && sv > 0 {
		sy := y2 + was.base - hookPx*27/100
		end := int(clamp01((t-2.5)/0.3)*float64(was.w+12)) - 6
		for x := -6; x < end; x++ {
			for k := -1; k <= 1; k++ {
				d.set(xa+x, sy+k, trBg.mix(p.urgent, sv))
			}
		}
	}
	if t < 3.0 || len(was.pts) == 0 {
		return
	}
	used := make([]bool, len(was.pts))
	shine := -100.0
	if t >= 4.1 {
		shine = -20 + (t-4.1)/0.6*float64(now.w+40)
	}
	for j, pt := range now.pts {
		s := (j * 7919) % len(was.pts)
		used[s] = true
		src := was.pts[s]
		q := clamp01((t - 3.0 - hash01(j, 5)*0.3) / 0.6)
		e := easeInOut(q)
		arc := math.Sin(math.Pi*e) * (hash01(j, 6) - 0.5) * 40
		x := lerp(float64(xa+src[0]), float64(xb+pt[0]), e)
		y := lerp(float64(y2+src[1]), float64(y2+pt[1]), e) + arc
		fin := p.teal.mix(p.blue, float64(pt[0])/float64(now.w)*0.7)
		band := max(0, 1-math.Abs(float64(pt[0])+float64(pt[1])*0.5-shine)/10) * 0.85
		c := dim.mix(fin, e).mix(p.glow, max(math.Sin(math.Pi*e)*0.8, band))
		d.set(int(math.Round(x)), int(math.Round(y)), c)
	}
	if fade := 1 - clamp01((t-3.0)/0.3); fade > 0 {
		for s, pt := range was.pts {
			if !used[s] {
				d.set(xa+pt[0], y2+pt[1], trBg.mix(dim, fade))
			}
		}
	}

	sub := []rune("чаты · задачи · уведомления — с клавиатуры")
	row := (y2+now.h)/4 + 2
	sx := (g.w - len(sub)) / 2
	n := int((t - 4.0) * 55)
	for i, ch := range sub {
		if i >= n {
			break
		}
		g.set(sx+i, row, ch, p.dim)
	}
	if n >= 0 && (n < len(sub) || int(t*4)%2 == 0) {
		g.set(sx+min(n, len(sub)), row, '▌', p.teal)
	}
}

var trNoise = []rune("░▒▓<>/\\|=+*#%")

func glitch(g *grid, amt float64, frame int, p introPal) {
	if amt <= 0 {
		return
	}
	src := append([]cell(nil), g.cells...)
	hot := []rgb{p.blue, p.teal, p.urgent}
	for y := 0; y < g.h; y++ {
		shift := 0
		if hash01(y, frame) < amt*0.7 {
			shift = int((hash01(y, frame+7) - 0.5) * 40 * amt)
		}
		for x := 0; x < g.w; x++ {
			c := cell{ch: ' '}
			if sx := x - shift; sx >= 0 && sx < g.w {
				c = src[y*g.w+sx]
			}
			if r := hash01(x+y*g.w, frame); r < amt*0.05 {
				c = cell{ch: trNoise[int(r*1e6)%len(trNoise)], c: hot[int(r*1e7)%len(hot)]}
			}
			g.cells[y*g.w+x] = c
		}
	}
}

// --- десктоп: обои, бар, тост, SUPER+B, окно ---

func (tr Trailer) drawWallpaper(d *dotBuf, p introPal) {
	vis := clamp01((tr.t-atDesk)/0.6) * (1 - clamp01((tr.t-atClose-0.3)/0.3))
	if vis <= 0 {
		return
	}
	span := float64((tr.h-7)*4 - 8)
	for i := 0; i < 220; i++ {
		x := int(hash01(i, 1) * float64(d.w))
		y := 8 + int(math.Mod(hash01(i, 2)*span-tr.t*2.5+span*16, span))
		tw := 0.5 + 0.5*math.Sin(tr.t*1.6+hash01(i, 3)*2*math.Pi)
		d.set(x, y, trBg.mix(p.teal.mix(p.blue, hash01(i, 4)), (0.1+0.3*tw)*vis))
	}
}

// drawDeskClock — часы-виджет на обоях, гаснут, когда открывается окно.
func (tr Trailer) drawDeskClock(d *dotBuf, p introPal) func(*grid) {
	vis := easeOut(clamp01((tr.t-atDesk-0.2)/0.5)) * (1 - clamp01((tr.t-atOpen+0.2)/0.3))
	if vis <= 0 {
		return func(*grid) {}
	}
	now := time.Now()
	m := textMask(now.Format("15:04"), 60)
	ox, oy := (d.w-m.w)/2, 6*4-m.top
	for _, pt := range m.pts {
		c := p.teal.mix(p.blue, float64(pt[0])/float64(max(1, m.w)))
		d.set(ox+pt[0], oy+pt[1], trBg.mix(c, 0.5*vis))
	}
	date := weekdayShort[now.Weekday()] + " · " + fmt.Sprint(now.Day()) + " " + monthGen[now.Month()-1]
	return func(g *grid) {
		y := (oy+m.bot)/4 + 2
		g.text((g.w-len([]rune(date)))/2-1, y, strings.Repeat(" ", len([]rune(date))+2), trBg)
		g.text((g.w-len([]rune(date)))/2, y, date, trBg.mix(p.dim, vis))
	}
}

func (tr Trailer) drawBar(g *grid, p introPal) {
	in := easeOut(clamp01((tr.t - atDesk) / 0.4))
	out := easeInOut(clamp01((tr.t - atClose - 0.3) / 0.3))
	half := int(math.Round(float64(g.w) / 2 * (in - out)))
	if half <= 0 {
		return
	}
	bar := trBg.mix(p.border, 0.3)
	g.fillBg(0, 0, g.w-1, 0, bar)
	g.text(2, 0, "󰣇", p.teal)
	for i := 1; i <= 5; i++ {
		c := p.dim
		if i == 1 {
			c = p.teal
		}
		g.text(3+i*3, 0, fmt.Sprint(i), c)
	}
	clock := clockLabel(time.Now())
	g.text((g.w-len([]rune(clock)))/2, 0, clock, p.fg)
	sys := "󰂯   󰤨   󰁹 87%"
	sx := g.w - len([]rune(sys)) - 2
	g.text(sx, 0, sys, p.dim)
	badge := fmt.Sprintf("󰭹 %d", tr.badge)
	bx := sx - len([]rune(badge)) - 4
	hot := 0.0
	if tr.badgeAt > 0 {
		hot = clamp01(1 - (tr.t-tr.badgeAt)/0.8)
	}
	if hot > 0 {
		g.fillBg(bx-1, 0, bx+len([]rune(badge)), 0, bar.mix(p.teal, 0.45*hot))
	}
	g.text(bx, 0, badge, p.fg.mix(p.teal, 0.6).mix(p.glow, hot))
	for x := 0; x < g.w; x++ {
		if x < g.w/2-half || x >= g.w/2+half {
			g.cells[x] = cell{ch: ' '}
		}
	}
}

func (tr Trailer) drawDeskToast(g *grid, p introPal) {
	in := easeOut(clamp01((tr.t - atToast) / 0.35))
	out := easeInOut(clamp01((tr.t - atUntoast) / 0.3))
	if in == 0 || out == 1 {
		return
	}
	w := 50
	x := g.w - w - 2 + int(math.Round((1-in+out)*float64(w+3)))
	trBox(g, x, 2, x+w-1, 6, func(int, int) rgb { return p.teal })
	g.fillBg(x+1, 3, x+w-2, 5, trBg.mix(p.border, 0.2))
	g.text(x+2, 3, "󰭹  Bitrix24", p.teal)
	g.text(x+w-8, 3, "сейчас", p.dim)
	g.text(x+5, 4, "Анна Смирнова", p.fg)
	g.text(x+5, 5, "Тогда жду билд к четвергу", p.dim)
}

func (tr Trailer) drawSuperB(g *grid, d *dotBuf, p introPal) {
	k := tr.t - atKeys
	if k < 0 || tr.t >= atOpen {
		return
	}
	vis := easeOut(clamp01(k/0.25)) * (1 - clamp01((tr.t-(atOpen-0.3))/0.3))
	caps := []struct {
		m        dotMask
		down, up float64
	}{{textMask("SUPER", keyPx), 0.45, 0.95}, {textMask("B", keyPx), 0.62, 0.95}}
	width := func(m dotMask) int { return (m.w+1)/2 + 6 }
	x := (g.w - width(caps[0].m) - 5 - width(caps[1].m)) / 2
	y := g.h/2 + 3
	for i, c := range caps {
		w := width(c.m)
		down := k >= c.down && k < c.up
		yy, hot, edge := y, 0.0, p.dim
		if down {
			yy, hot, edge = y+1, 1-clamp01((k-c.down)/0.4)*0.6, p.teal
			g.fillBg(x+1, yy+1, x+w-2, yy+5, trBg.mix(p.teal, 0.22*vis))
		}
		border := trBg.mix(edge.mix(p.glow, hot*0.5), vis)
		trBox(g, x, yy, x+w-1, yy+6, func(int, int) rgb { return border })
		label := trBg.mix(p.fg.mix(p.glow, hot), vis)
		ox, oy := (x+3)*2, (yy+1)*4+(20-(c.m.bot-c.m.top+1))/2-c.m.top
		for _, pt := range c.m.pts {
			d.set(ox+pt[0], oy+pt[1], label)
		}
		x += w
		if i == 0 {
			g.text(x+2, y+3, "+", trBg.mix(p.dim, vis))
			x += 5
		}
	}
}

type rect struct{ x0, y0, x1, y1 int }

func (tr Trailer) winRect() rect { return rect{2, 2, tr.w - 3, tr.h - 7} }

// drawWindow — окно как в Hyprland: popin с перелётом, градиентная рамка
// крутится; возвращает видимую часть внутренности под приложение.
func (tr Trailer) drawWindow(g *grid, p introPal) (rect, bool) {
	var s, vis float64
	switch t := tr.t; {
	case t < atOpen:
		return rect{}, false
	case t < atOpen+0.45:
		q := (t - atOpen) / 0.45
		s, vis = 0.6+0.4*easeBack(q), easeOut(clamp01(q*2))
	case t < atClose:
		s, vis = 1, 1
	case t < atClose+0.35:
		q := (t - atClose) / 0.35
		s, vis = 1-0.45*q*q*q, 1-q
	default:
		return rect{}, false
	}
	w := tr.winRect()
	cx, cy := float64(w.x0+w.x1)/2, float64(w.y0+w.y1)/2
	hw, hh := float64(w.x1-w.x0)/2*s, float64(w.y1-w.y0)/2*s
	r := rect{
		max(int(math.Round(cx-hw)), 0), max(int(math.Round(cy-hh)), 1),
		min(int(math.Round(cx+hw)), g.w-1), min(int(math.Round(cy+hh)), g.h-6),
	}
	for y := r.y0 + 1; y < r.y1; y++ {
		for x := r.x0 + 1; x < r.x1; x++ {
			g.cells[y*g.w+x] = cell{ch: ' '}
		}
	}
	pulse := 1 - clamp01((tr.t-atOpen)/0.6)
	trBox(g, r.x0, r.y0, r.x1, r.y1, func(x, y int) rgb {
		a := math.Atan2(float64(y)-cy, (float64(x)-cx)/2.2)
		c := p.teal.mix(p.blue, (1+math.Sin(a+tr.t*1.2))/2).mix(p.glow, pulse*0.6)
		return trBg.mix(c, vis)
	})
	in := rect{max(r.x0+1, w.x0+1), max(r.y0+1, w.y0+1), min(r.x1-1, w.x1-1), min(r.y1-1, w.y1-1)}
	return in, in.x1 >= in.x0 && in.y1 >= in.y0
}

// splice — вклеить кадр живого приложения в видимую часть окна.
func (tr Trailer) splice(lines []string, g *grid, in rect) {
	w := tr.winRect()
	ix, iy := w.x0+1, w.y0+1
	app := strings.Split(tr.app.View().Content, "\n")
	for y := in.y0; y <= in.y1; y++ {
		l := ""
		if ay := y - iy; ay < len(app) {
			l = app[ay]
		}
		a, b := in.x0-ix, in.x1-ix+1
		seg := ansi.Cut(l, a, b)
		if pad := b - a - ansi.StringWidth(seg); pad > 0 {
			seg += strings.Repeat(" ", pad)
		}
		lines[y] = g.span(y, 0, in.x0) + seg + "\x1b[0m" + g.span(y, in.x1+1, g.w)
	}
}

// --- подписи и кейкапы ---

func (tr Trailer) drawCaption(g *grid, d *dotBuf, p introPal, frame int) {
	for i, c := range trailerCaps {
		if tr.t < c.from || tr.t >= c.to {
			continue
		}
		k := tr.t - c.from
		out := clamp01((c.to - tr.t) / 0.2)
		y := g.h - 5
		m := textMask(c.word, capPx)
		reveal := easeOut(clamp01(k/0.35)) * float64(m.w+8)
		ox, oy := 6, y*4+(20-(m.bot-m.top+1))/2-m.top
		for _, pt := range m.pts {
			x := float64(pt[0])
			if x > reveal {
				continue
			}
			col := p.blue.mix(p.teal, x/float64(m.w)).mix(p.glow, max(0, 1-(reveal-x)/6))
			d.set(ox+pt[0], oy+pt[1], trBg.mix(col, out))
		}
		tx := 3 + (m.w+1)/2 + 3
		g.text(tx, y+1, fmt.Sprintf("%02d", i+1), trBg.mix(p.teal, out))
		g.text(tx+3, y+1, fmt.Sprintf("/ %02d", len(trailerCaps)), trBg.mix(p.border, out))
		decodeText(g, tx, y+2, c.desc, clamp01((k-0.1)/0.5), trBg.mix(p.fg, out), trBg.mix(p.blue, out), frame)
	}
}

func (tr Trailer) drawKeys(g *grid, p introPal) {
	x, y := g.w-3, g.h-4
	for i := len(tr.keys) - 1; i >= 0; i-- {
		k := tr.keys[i]
		age := tr.t - k.at
		w := len([]rune(k.label)) + 4
		if age > 1.8 || x-w < g.w/2+16 {
			break
		}
		x -= w
		vis := clamp01((1.8 - age) / 0.4)
		hot := clamp01(1 - age/0.3)
		if hot > 0 {
			g.fillBg(x+1, y+1, x+w-2, y+1, trBg.mix(p.teal, 0.3*hot))
		}
		border := trBg.mix(p.border.mix(p.teal, hot), vis)
		trBox(g, x, y, x+w-1, y+2, func(int, int) rgb { return border })
		g.text(x+2, y+1, k.label, trBg.mix(p.fg.mix(p.glow, hot), vis))
		x--
	}
}

// --- каскад фич и финал ---

func (tr Trailer) drawRoll(g *grid, d *dotBuf, p introPal, frame int) {
	k := tr.t - atRoll
	n := len(trailerRoll)
	i := min(int(k/rollStep), n-1)
	local := k - float64(i)*rollStep
	bandH := textMask(trailerRoll[i].word, rollPx).h
	top := (d.h-bandH)/2 - 8
	slide := easeInOut(clamp01(local / 0.2))
	exit := easeInOut(clamp01((tr.t - (atOutro - 0.3)) / 0.3))
	word := func(j int, off float64) {
		m := textMask(trailerRoll[j].word, rollPx)
		ox := (d.w - m.w) / 2
		for _, pt := range m.pts {
			y := pt[1] + int(math.Round(off))
			if y < 0 || y >= m.h {
				continue
			}
			c := p.blue.mix(p.teal, float64(pt[0])/float64(max(1, m.w))).mix(p.glow, (1-slide)*0.6)
			d.set(ox+pt[0], top+y, c)
		}
	}
	off := float64(bandH) * (1 - slide)
	if i > 0 && slide < 1 {
		word(i-1, off-float64(bandH))
	}
	word(i, off-exit*float64(bandH))

	fade := 1 - exit
	kick := fmt.Sprintf("и ещё  %02d / %02d", i+1, n)
	g.text((g.w-len([]rune(kick)))/2, top/4-2, kick, trBg.mix(p.teal, fade))
	desc := trailerRoll[i].desc
	decodeText(g, (g.w-len([]rune(desc)))/2, (top+bandH)/4+2, desc, clamp01((local-0.08)/0.3),
		trBg.mix(p.fg, fade), trBg.mix(p.blue, fade), frame)
}

func (tr Trailer) drawOutro(g *grid, p introPal) {
	f := int((tr.t - atOutro) / introDT.Seconds())
	word := Logo("omabitrix")
	lockW := ringW + introLockGap + len([]rune(word[0]))
	x0, y0 := (g.w-lockW)/2, (g.h-22)/2
	slide := easeInOut(clamp01(float64(f-22) / 12))
	ringX := int(math.Round(lerp(float64(g.w-ringW)/2, float64(x0), slide)))
	wx := x0 + ringW + introLockGap
	drawWordmark(g, wx, y0+3, word, f, p)
	drawRing(g, ringX, y0, f, p)
	drawTyped(g, wx+1, y0+10, f, p)

	url := "github.com/oglenyaboss/omabitrix"
	typeLine(g, (g.w-len(url))/2, y0+16, url, (f-50)*2, p.fg)
	cmd := "$ go run github.com/oglenyaboss/omabitrix/cmd/omabitrix@latest -demo"
	cx := (g.w - len(cmd)) / 2
	typeLine(g, cx, y0+18, cmd, (f-66)*3, p.dim)
	if f >= 66 {
		g.set(cx, y0+18, '$', p.teal)
	}
	if foot := "go · bubble tea · omarchy · hyprland · mit"; f >= 92 {
		g.text((g.w-len([]rune(foot)))/2, y0+21, foot, p.border)
	}
}

// --- примитивы: брайль-точки, маски текста, рамки ---

type dotBuf struct {
	w, h int
	on   []bool
	c    []rgb
}

func newDots(cols, rows int) *dotBuf {
	return &dotBuf{w: cols * 2, h: rows * 4, on: make([]bool, cols*rows*8), c: make([]rgb, cols*rows*8)}
}

func lum(c rgb) int { return int(c.r)*3 + int(c.g)*6 + int(c.b) }

func (d *dotBuf) set(x, y int, c rgb) {
	if x < 0 || y < 0 || x >= d.w || y >= d.h {
		return
	}
	i := y*d.w + x
	if !d.on[i] || lum(c) > lum(d.c[i]) {
		d.on[i], d.c[i] = true, c
	}
}

// flush — точки в клетки брайля; цвет клетки — самой яркой точки.
func (d *dotBuf) flush(g *grid) {
	for cy := 0; cy < g.h; cy++ {
		for cx := 0; cx < g.w; cx++ {
			bits, best := 0, rgb{}
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					if i := (cy*4+dy)*d.w + cx*2 + dx; d.on[i] {
						bits |= brailleBit[dy][dx]
						if lum(d.c[i]) > lum(best) {
							best = d.c[i]
						}
					}
				}
			}
			if bits != 0 {
				g.set(cx, cy, rune(0x2800+bits), best)
			}
		}
	}
}

// dotMask — строка, растеризованная Go Mono Bold: 1 пиксель = 1 точка брайля.
type dotMask struct {
	w, h, base int
	top, bot   int
	pts        [][2]int
}

var (
	maskMu    sync.Mutex
	maskFont  *opentype.Font
	maskCache = map[string]dotMask{}
)

func textMask(s string, px float64) dotMask {
	maskMu.Lock()
	defer maskMu.Unlock()
	key := fmt.Sprintf("%g|%s", px, s)
	if m, ok := maskCache[key]; ok {
		return m
	}
	if maskFont == nil {
		f, err := opentype.Parse(gomonobold.TTF)
		if err != nil {
			return dotMask{}
		}
		maskFont = f
	}
	face, err := opentype.NewFace(maskFont, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return dotMask{}
	}
	defer face.Close()
	dr := &font.Drawer{Face: face}
	mt := face.Metrics()
	m := dotMask{w: dr.MeasureString(s).Ceil(), h: (mt.Ascent + mt.Descent).Ceil(), base: mt.Ascent.Ceil(), top: math.MaxInt}
	img := image.NewAlpha(image.Rect(0, 0, m.w, m.h))
	dr.Dst, dr.Src, dr.Dot = img, image.Opaque, fixed.Point26_6{Y: mt.Ascent}
	dr.DrawString(s)
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			if img.AlphaAt(x, y).A >= 110 {
				m.pts = append(m.pts, [2]int{x, y})
				m.top, m.bot = min(m.top, y), max(m.bot, y)
			}
		}
	}
	maskCache[key] = m
	return m
}

func trBox(g *grid, x0, y0, x1, y1 int, col func(x, y int) rgb) {
	for x := x0; x <= x1; x++ {
		top, bot := '─', '─'
		switch x {
		case x0:
			top, bot = '╭', '╰'
		case x1:
			top, bot = '╮', '╯'
		}
		g.set(x, y0, top, col(x, y0))
		g.set(x, y1, bot, col(x, y1))
	}
	for y := y0 + 1; y < y1; y++ {
		g.set(x0, y, '│', col(x0, y))
		g.set(x1, y, '│', col(x1, y))
	}
}

func decodeText(g *grid, x, y int, s string, k float64, c, hot rgb, frame int) {
	rs := []rune(s)
	head := k * float64(len(rs)+6)
	for i, ch := range rs {
		switch fi := float64(i); {
		case fi < head-6:
			g.set(x+i, y, ch, c)
		case fi < head && ch != ' ':
			g.set(x+i, y, trNoise[int(hash01(i, frame)*float64(len(trNoise)))], hot)
		}
	}
}

func typeLine(g *grid, x, y int, s string, n int, c rgb) {
	for i, ch := range []rune(s) {
		if i >= n {
			break
		}
		g.set(x+i, y, ch, c)
	}
}

func hash01(a, b int) float64 {
	h := uint32(a)*0x9E3779B1 ^ uint32(b)*0x85EBCA77
	h ^= h >> 15
	h *= 0x2C1B3C6D
	h ^= h >> 12
	h *= 0x297A2D39
	h ^= h >> 15
	return float64(h) / (1 << 32)
}

func easeIn(t float64) float64 { return t * t * t }
