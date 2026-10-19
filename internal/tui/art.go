package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ANSI Shadow (neofetch-классика). 6 строк на букву, «omabitrix» — 71 колонка.
var glyphs = map[rune][]string{
	'o': {
		" ██████╗ ",
		"██╔═══██╗",
		"██║   ██║",
		"██║   ██║",
		"╚██████╔╝",
		" ╚═════╝ ",
	},
	'm': {
		"███╗   ███╗",
		"████╗ ████║",
		"██╔████╔██║",
		"██║╚██╔╝██║",
		"██║ ╚═╝ ██║",
		"╚═╝     ╚═╝",
	},
	'a': {
		" █████╗ ",
		"██╔══██╗",
		"███████║",
		"██╔══██║",
		"██║  ██║",
		"╚═╝  ╚═╝",
	},
	'b': {
		"██████╗ ",
		"██╔══██╗",
		"██████╔╝",
		"██╔══██╗",
		"██████╔╝",
		"╚═════╝ ",
	},
	'i': {
		"██╗",
		"██║",
		"██║",
		"██║",
		"██║",
		"╚═╝",
	},
	't': {
		"████████╗",
		"╚══██╔══╝",
		"   ██║   ",
		"   ██║   ",
		"   ██║   ",
		"   ╚═╝   ",
	},
	'r': {
		"██████╗ ",
		"██╔══██╗",
		"██████╔╝",
		"██╔══██╗",
		"██║  ██║",
		"╚═╝  ╚═╝",
	},
	'x': {
		"██╗  ██╗",
		"╚██╗██╔╝",
		" ╚███╔╝ ",
		" ██╔██╗ ",
		"██╔╝ ██╗",
		"╚═╝  ╚═╝",
	},
}

const logoSub = "bitrix24 в терминале"

var logoStyle = lipgloss.NewStyle().Foreground(cAccent)
var logoSubStyle = lipgloss.NewStyle().Foreground(cDim).Italic(true)

// Logo — арт построчно (одна строка = строка букв).
func Logo(word string) []string {
	rows := 6
	letters := make([][]string, 0, len(word))
	maxW := 0
	for _, ch := range word {
		g := glyphs[ch]
		if g == nil {
			continue
		}
		letters = append(letters, g)
		if w := lipgloss.Width(g[0]); w > maxW {
			maxW = w
		}
	}
	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		parts := make([]string, len(letters))
		for i, g := range letters {
			parts[i] = g[r]
		}
		out[r] = strings.Join(parts, " ")
	}
	return out
}

// Splash — лого + подзаголовок + спиннер загрузки.
func Splash(spinner string) string {
	var b strings.Builder
	b.WriteString("\n\n")
	for _, line := range Logo("omabitrix") {
		b.WriteString("  ")
		b.WriteString(logoStyle.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n    ")
	b.WriteString(logoSubStyle.Render(logoSub))
	b.WriteString("\n\n    ")
	b.WriteString(stDim.Render(spinner + " грузим чаты, задачи, уведы…"))
	b.WriteString("\n\n   ")
	b.WriteString(stDim.Render("любая клавиша — пропустить"))
	b.WriteString("\n")
	return b.String()
}

// SplashErr — сплэш с ошибкой загрузки.
func SplashErr(spinner, err string) string {
	return Splash(spinner) + stErr.Render("    ⚠ "+err) + "\n"
}
