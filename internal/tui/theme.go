package tui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Палитра: монохром Omarchy + акцент + приглушённая per-author палитра.
// Ревизия решения v0 «один акцент»: автор-цвета переняты у tele (референс),
// юзером санкционировано — не сворачивать обратно в монохром.
// Роль, не hex из темы: терминал подхватывает ANSI 256 (foot сам красиво).
var (
	cFg     = compat.AdaptiveColor{Light: lipgloss.Color("#1c2321"), Dark: lipgloss.Color("#e6e6e6")}
	cDim    = compat.AdaptiveColor{Light: lipgloss.Color("#5a6a66"), Dark: lipgloss.Color("#7a8a86")}
	cBorder = compat.AdaptiveColor{Light: lipgloss.Color("#b8c4c0"), Dark: lipgloss.Color("#3a4844")}
	cAccent = compat.AdaptiveColor{Light: lipgloss.Color("#0f7b6c"), Dark: lipgloss.Color("#39d3b6")}
	cUrgent = compat.AdaptiveColor{Light: lipgloss.Color("#b02a37"), Dark: lipgloss.Color("#f26d7d")}
	cMine   = compat.AdaptiveColor{Light: lipgloss.Color("#0f7b6c"), Dark: lipgloss.Color("#1f5f56")}

	stTitle   = lipgloss.NewStyle().Bold(true).Foreground(cFg)
	stDim     = lipgloss.NewStyle().Foreground(cDim)
	stAccent  = lipgloss.NewStyle().Foreground(cAccent)
	stUrgent  = lipgloss.NewStyle().Foreground(cUrgent)
	stBold    = lipgloss.NewStyle().Bold(true)
	stBoldDim = lipgloss.NewStyle().Bold(true).Foreground(cDim)

	stBadge   = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	stBadgeDm = lipgloss.NewStyle().Foreground(cDim)

	// Выделение без заливки — как в tele: маркер акцентом, не плашка.
	stCursor = lipgloss.NewStyle().Bold(true).Foreground(cAccent)

	stFooter = lipgloss.NewStyle().Foreground(cDim)

	stChatUnread = lipgloss.NewStyle().Bold(true).Foreground(cFg)
	stOnline     = lipgloss.NewStyle().Foreground(cAccent).SetString("●")
	stBubbleMine = lipgloss.NewStyle().Foreground(cMine)
	stHelpKey    = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	stHelpDesc   = lipgloss.NewStyle().Foreground(cDim)
	stErr        = lipgloss.NewStyle().Foreground(cUrgent).Bold(true)

	stTrack = lipgloss.NewStyle().Foreground(cBorder)
	stThumb = lipgloss.NewStyle().Foreground(cAccent)
)

// authorPalette — приглушённые adaptive-пары для имён авторов.
// cAccent и cUrgent в палитру не входят: акцент — наш маркер, красный — просрочка.
var authorPalette = []compat.AdaptiveColor{
	{Light: lipgloss.Color("#2a5db0"), Dark: lipgloss.Color("#7aa2f7")}, // синий
	{Light: lipgloss.Color("#7a4fb5"), Dark: lipgloss.Color("#bb9af7")}, // фиолетовый
	{Light: lipgloss.Color("#a05a00"), Dark: lipgloss.Color("#e0af68")}, // янтарный
	{Light: lipgloss.Color("#b02a6a"), Dark: lipgloss.Color("#f7768e")}, // розовый
	{Light: lipgloss.Color("#1a7ea5"), Dark: lipgloss.Color("#7dcfff")}, // голубой
	{Light: lipgloss.Color("#5a6a16"), Dark: lipgloss.Color("#b8c96a")}, // оливковый
}

// authorStyle — стабильный цвет по author_id: id стабилен при переименовании,
// два одинаковых имени не склеиваются в одну группу.
func authorStyle(id int64) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(authorPalette[int(id)%len(authorPalette)])
}

// helpKey — «j/k» акцентом + описание тускло.
func help(key, desc string) string {
	return stHelpKey.Render(key) + stHelpDesc.Render(" "+desc)
}
