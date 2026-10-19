package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Рамка в стиле tele: rounded, заголовок врезан в верхнюю грань.
// Собирается вручную (в lipgloss нет title-in-border), стиль границы —
// отдельно от контента, чтобы не красить внутренний текст.
//
// Семантика: innerW — ширина строк контента (как их строят callers),
// внешняя ширина панели = innerW + 4 (грани + паддинг).

func boxTop(title string, innerW int, st lipgloss.Style) string {
	outer := innerW + 4
	if title != "" {
		title = " " + title + " "
		if lipgloss.Width(title) > outer-3 {
			title = ansi.Truncate(title, outer-3, "…")
		}
	}
	fill := outer - 3 - lipgloss.Width(title)
	return st.Render("╭─" + title + strings.Repeat("─", max(fill, 0)) + "╮")
}

func boxBottom(innerW int, st lipgloss.Style) string {
	return st.Render("╰" + strings.Repeat("─", max(innerW+2, 0)) + "╯")
}

// box — рамка вокруг готовых строк контента шириной ровно innerW
// (паддинг/клип ANSI-aware).
func box(title string, lines []string, innerW int, active bool) string {
	if innerW < 1 {
		innerW = 1
	}
	st := stTrack
	if active {
		st = stCursor
	}
	out := make([]string, 0, len(lines)+2)
	out = append(out, boxTop(title, innerW, st))
	for _, l := range lines {
		if pad := innerW - lipgloss.Width(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		} else if pad < 0 {
			l = ansi.Truncate(l, innerW, "")
		}
		out = append(out, st.Render("│ ")+l+st.Render(" │"))
	}
	out = append(out, boxBottom(innerW, st))
	return strings.Join(out, "\n")
}

// scrollbarColumn — трек-колонка той же высоты, что контент диалога.
// thumb по позиции viewport; при total<=height трек без ползунка.
func scrollbarColumn(total, height, offset int) string {
	if height < 1 {
		height = 1
	}
	thumbSize := height * height / total
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > height {
		thumbSize = height
	}
	start := 0
	if total > height {
		start = offset * (height - thumbSize) / (total - height)
		if start < 0 {
			start = 0
		}
		if start > height-thumbSize {
			start = height - thumbSize
		}
	}
	col := make([]string, height)
	for i := range col {
		if total > height && i >= start && i < start+thumbSize {
			col[i] = stThumb.Render("▐")
		} else {
			col[i] = stTrack.Render("│")
		}
	}
	return strings.Join(col, "\n")
}

// joinScrollbar — приклеить колонку справа от контента построчно.
func joinScrollbar(content, col string, height int) string {
	l := strings.Split(content, "\n")
	r := strings.Split(col, "\n")
	for len(l) < height {
		l = append(l, "")
	}
	out := make([]string, height)
	for i := 0; i < height; i++ {
		out[i] = l[i] + " " + r[i]
	}
	return strings.Join(out, "\n")
}
