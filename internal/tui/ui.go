package tui

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Примитивы редизайна: пилюли, аватары, градиенты, крупные цифры, пульс.
// Цвета — роли, как в theme.go; стадии канбана красятся hex'ом портала.
var (
	cAmber = compat.AdaptiveColor{Light: lipgloss.Color("#a05a00"), Dark: lipgloss.Color("#e0af68")}
	cBlue  = compat.AdaptiveColor{Light: lipgloss.Color("#2a5db0"), Dark: lipgloss.Color("#7aa2f7")}
	cInk   = compat.AdaptiveColor{Light: lipgloss.Color("#ffffff"), Dark: lipgloss.Color("#0d1215")}
	cChip  = compat.AdaptiveColor{Light: lipgloss.Color("#dfe6e3"), Dark: lipgloss.Color("#34433f")}
	cFaint = compat.AdaptiveColor{Light: lipgloss.Color("#c9d3d0"), Dark: lipgloss.Color("#2a3735")}
	cSelBg = compat.AdaptiveColor{Light: lipgloss.Color("#e3efec"), Dark: lipgloss.Color("#18272a")}

	stFaint = lipgloss.NewStyle().Foreground(cFaint)
	stAmber = lipgloss.NewStyle().Foreground(cAmber)
	stKey   = lipgloss.NewStyle().Bold(true).Foreground(cFg).Background(cChip).Padding(0, 1)
)

// pill — плашка « текст » на заливке: вкладка, срок, стадия.
func pill(text string, fg, bg color.Color) string {
	return lipgloss.NewStyle().Bold(true).Foreground(fg).Background(bg).Render(" " + text + " ")
}

// chip — тихая плашка без жирного: неактивные фильтры, дальние сроки.
func chip(text string) string {
	return lipgloss.NewStyle().Foreground(cFg).Background(cChip).Render(" " + text + " ")
}

// stageColor — цвет стадии из hex портала; пустой — нейтральный.
func stageColor(hex string) color.Color {
	if hex == "" {
		return cDim
	}
	return lipgloss.Color("#" + strings.TrimPrefix(hex, "#"))
}

// seedOf — стабильное число из строки (DialogID): цвет аватара не прыгает.
func seedOf(s string) int64 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int64(h.Sum32())
}

// avatar — буква на цветной заливке « К »; групповой чат — « # ».
func avatar(title string, group bool, seed int64) string {
	letter := "#"
	if !group {
		letter = "?"
		for _, r := range title {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				letter = string(unicode.ToUpper(r))
				break
			}
		}
	}
	bg := authorPalette[int(uint64(seed)%uint64(len(authorPalette)))]
	return pill(letter, cInk, bg)
}

// pulseColor — янтарь, дышащий к dim: кадр анимации frame (тик 150 мс,
// цикл 16 кадров ≈ 2.4 с).
func pulseColor(frame int) color.Color {
	steps := lipgloss.Blend1D(9, cAmber, cDim)
	p := frame % 16
	if p < 0 {
		p += 16
	}
	if p > 8 {
		p = 16 - p
	}
	return steps[p]
}

// gradient — текст с градиентом по рунам; пробелы не красятся.
func gradient(s string, stops ...color.Color) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	cols := lipgloss.Blend1D(max(len(r), len(stops)), stops...)
	var b strings.Builder
	for i, ch := range r {
		if ch == ' ' {
			b.WriteRune(ch)
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(cols[i]).Render(string(ch)))
	}
	return b.String()
}

// bigFont — цифры 3×5 «пикселей»; пиксель рисуется «██».
var bigFont = map[rune][5]string{
	'0': {"###", "# #", "# #", "# #", "###"},
	'1': {" # ", "## ", " # ", " # ", "###"},
	'2': {"###", "  #", "###", "#  ", "###"},
	'3': {"###", "  #", "###", "  #", "###"},
	'4': {"# #", "# #", "###", "  #", "  #"},
	'5': {"###", "#  ", "###", "  #", "###"},
	'6': {"###", "#  ", "###", "# #", "###"},
	'7': {"###", "  #", "  #", "  #", "  #"},
	'8': {"###", "# #", "###", "# #", "###"},
	'9': {"###", "# #", "###", "  #", "###"},
	':': {" ", "#", " ", "#", " "},
}

// bigText — 5 строк крупного текста (без цвета); между глифами 2 колонки.
// colon=false гасит двоеточие (мигание часов). Неизвестные руны пропускаются.
func bigText(s string, colon bool) []string {
	lines := make([]string, 5)
	first := true
	for _, ch := range s {
		g, ok := bigFont[ch]
		if !ok {
			continue
		}
		for j := range lines {
			if !first {
				lines[j] += "  "
			}
			row := g[j]
			if ch == ':' && !colon {
				row = " "
			}
			lines[j] += strings.NewReplacer("#", "██", " ", "  ").Replace(row)
		}
		first = false
	}
	return lines
}

// bigClock — крупный текст с горизонтальным градиентом from→to.
func bigClock(s string, colon bool, from, to color.Color) []string {
	plain := bigText(s, colon)
	w := 0
	for _, l := range plain {
		w = max(w, len([]rune(l)))
	}
	cols := lipgloss.Blend1D(max(w, 2), from, to)
	out := make([]string, len(plain))
	for j, l := range plain {
		var b strings.Builder
		for i, ch := range []rune(l) {
			if ch == ' ' {
				b.WriteRune(ch)
				continue
			}
			b.WriteString(lipgloss.NewStyle().Foreground(cols[i]).Render(string(ch)))
		}
		out[j] = b.String()
	}
	return out
}

// meter — шкала ширины w: заполнено frac (0..1) градиентом «▰», остаток «▱».
func meter(w int, frac float64, from, to color.Color) string {
	if w <= 0 {
		return ""
	}
	frac = min(max(frac, 0), 1)
	n := int(frac*float64(w) + 0.5)
	cols := lipgloss.Blend1D(max(w, 2), from, to)
	var b strings.Builder
	for i := 0; i < w; i++ {
		if i < n {
			b.WriteString(lipgloss.NewStyle().Foreground(cols[i]).Render("▰"))
		} else {
			b.WriteString(stFaint.Render("▱"))
		}
	}
	return b.String()
}

// untilLabel — «через 1 ч 05 мин» / «через 25 мин» / «через 2 дн»;
// прошедшее — «просрочено на …».
func untilLabel(d time.Duration) string {
	past := d < 0
	if past {
		d = -d
	}
	var s string
	switch {
	case d < time.Minute:
		s = "меньше минуты"
	case d < time.Hour:
		s = fmt.Sprintf("%d мин", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		if mm := int(d.Minutes()) % 60; mm > 0 {
			s = fmt.Sprintf("%d ч %02d мин", h, mm)
		} else {
			s = fmt.Sprintf("%d ч", h)
		}
	default:
		s = fmt.Sprintf("%d дн", int(d.Hours()/24))
	}
	if past {
		return "просрочено на " + s
	}
	return "через " + s
}

var weekdayShort = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

var monthShort = [...]string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}

// clockLabel — «пн 19 окт  16:55» для строки вкладок.
func clockLabel(now time.Time) string {
	return weekdayShort[now.Weekday()] + " " + now.Format("2") + " " +
		monthShort[now.Month()-1] + "  " + now.Format("15:04")
}

// dueLabel — подпись срока для плашки: «сегодня 18:00», «завтра 18:00»,
// «ср 21.10»; прошедший день — «вчера» / «18.10». now и dl — в одной зоне.
func dueLabel(dl, now time.Time) string {
	dd := time.Date(dl.Year(), dl.Month(), dl.Day(), 0, 0, 0, 0, dl.Location())
	nd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, dl.Location())
	days := int(dd.Sub(nd).Hours() / 24)
	switch {
	case days == 0:
		return "сегодня " + dl.Format("15:04")
	case days == 1:
		return "завтра " + dl.Format("15:04")
	case days == -1:
		return "вчера"
	case days > 1 && days < 7:
		return weekdayShort[dl.Weekday()] + " " + dl.Format("02.01")
	default:
		return dl.Format("02.01")
	}
}
