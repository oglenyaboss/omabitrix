package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// toastInfo — всплывашка «новое сообщение из другого чата»: чат, момент
// появления и кадр старта выезда.
type toastInfo struct {
	chat  Chat
	at    time.Time
	frame int
}

// toastTTL — сколько тост живёт на экране.
const toastTTL = 5 * time.Second

// toastOuter — внешняя ширина тоста: 30 контента + 4 рамка.
const toastOuter = 34

// toastVisible — тост показан, пока не истёк TTL; новый заменяет старый.
func (m Model) toastVisible() bool {
	return m.toast != nil && m.api.now().Sub(m.toast.at) < toastTTL
}

// toastView — рамка 34×4: аватар + имя + время, под ними превью.
func (m Model) toastView() string {
	c := m.toast.chat
	inner := toastOuter - 4
	// аватар + зазор + имя + минимум 1 до времени
	name := stTitle.Render(truncate(c.Title, max(inner-10, 1)))
	line1 := avatar(c.Title, strings.HasPrefix(c.DialogID, "chat"), seedOf(c.DialogID)) +
		" " + name
	timeS := stDim.Render(c.Time)
	if gap := inner - lipgloss.Width(line1) - lipgloss.Width(timeS); gap > 0 {
		line1 += strings.Repeat(" ", gap)
	}
	line1 += timeS
	// 4 пробела — под аватар и зазор первой строки
	line2 := stDim.Render("    " + truncate(c.Preview, max(inner-4, 1)))
	return box("", []string{line1, line2}, inner, false)
}

// toastOverlay — наложить тост на собранный экран: выезжает из-за правой
// границы за 6 кадров (по 150 мс), живёт 5 с; правый край режет канвас.
// Слои — через Compositor: Canvas.Compose(NewLayer) стирал бы нижний слой
// (StyledString.Draw чистит весь area).
func (m Model) toastOverlay(s string) string {
	k := m.frame - m.toast.frame
	shift := 0
	if k >= 0 && k < 6 {
		shift = toastOuter * (6 - k) / 6
	}
	x := m.width - toastOuter - 1 + shift
	if x < 0 {
		x = 0
	}
	comp := lipgloss.NewCompositor(
		lipgloss.NewLayer(s),
		lipgloss.NewLayer(m.toastView()).X(x).Y(1),
	)
	canvas := lipgloss.NewCanvas(m.width, lipgloss.Height(s))
	canvas.Compose(comp)
	return canvas.Render()
}
