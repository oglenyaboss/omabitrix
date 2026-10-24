package tui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Пульт дня: часы и прогресс рабочего дня, ближайший срок, календарь месяца,
// непрочитанные чаты, канбан по стадиям, неделя. Виджеты собирают строки
// контента для box(), viewHome раскладывает их по колонкам и высоте экрана.

var (
	// месяц в именительном (заголовок календаря) и родительном (дата)
	monthNom = [...]string{"январь", "февраль", "март", "апрель", "май", "июнь",
		"июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"}
	monthGen = [...]string{"января", "февраля", "марта", "апреля", "мая", "июня",
		"июля", "августа", "сентября", "октября", "ноября", "декабря"}
)

// plural — русское склонение: 1 срок / 2 срока / 5 сроков (11–14 — как «5»).
func plural(n int, f1, f2, f5 string) string {
	switch {
	case n%100 >= 11 && n%100 <= 14:
		return f5
	case n%10 == 1:
		return f1
	case n%10 >= 2 && n%10 <= 4:
		return f2
	default:
		return f5
	}
}

// boxTitle — заголовок рамки с прижатой вправо частью: box() рисует заголовок
// одной строкой, зазор добираем пробелами; не влезло — порежет box().
func boxTitle(name, right string, innerW int) string {
	t := stTitle.Render(name)
	if right == "" {
		return t
	}
	gap := max(innerW-1-lipgloss.Width(t)-lipgloss.Width(right), 1)
	return t + strings.Repeat(" ", gap) + right
}

// nextDue — задача для «следующего срока»: первая просрочка, иначе ближайшая
// сегодняшняя, иначе ближайшая из «позже».
func (m Model) nextDue() (Task, bool) {
	if len(m.buckets.Overdue) > 0 {
		return m.buckets.Overdue[0], true
	}
	nearest := func(ts []Task) (Task, bool) {
		best, bestDl, found := Task{}, time.Time{}, false
		for _, t := range ts {
			dl, ok := ParseDeadline(t.Deadline, m.srvLoc())
			if !ok {
				continue
			}
			if !found || dl.Before(bestDl) {
				best, bestDl, found = t, dl, true
			}
		}
		return best, found
	}
	if t, ok := nearest(m.buckets.Today); ok {
		return t, true
	}
	return nearest(m.buckets.Later)
}

// homeDueCounts — дедлайны моих задач (Role «do») по датам серверной зоны.
func (m Model) homeDueCounts() map[string]int {
	loc := m.srvLoc()
	counts := map[string]int{}
	for _, t := range m.tasks {
		if t.Role != "do" {
			continue
		}
		if dl, ok := ParseDeadline(t.Deadline, loc); ok {
			counts[dl.In(loc).Format("2006-01-02")]++
		}
	}
	return counts
}

// homeToday — часы, приветствие, счётчики дня и полоса рабочего дня.
// ponytail: рабочий день 9–18 захардкожен; вынести в config при запросе
func (m Model) homeToday(innerW int) []string {
	now := m.api.now().In(m.srvLoc())
	clock := bigClock(now.Format("15:04"), now.Second()%2 == 0, cAccent, cBlue)
	cw := 0
	for _, l := range clock {
		cw = max(cw, lipgloss.Width(l))
	}
	greet := "доброй ночи"
	switch h := now.Hour(); {
	case h >= 5 && h < 12:
		greet = "доброе утро"
	case h < 18:
		greet = "добрый день"
	case h < 23:
		greet = "добрый вечер"
	}
	right := []string{
		stDim.Render(greet),
		stDim.Render(weekdayShort[now.Weekday()] + " · " + fmt.Sprint(now.Day()) + " " + monthGen[now.Month()-1]),
	}
	if n := len(m.buckets.Today); n > 0 {
		right = append(right, stAmber.Render(fmt.Sprintf("%d %s сегодня", n, plural(n, "срок", "срока", "сроков"))))
	} else {
		right = append(right, stDim.Render("сегодня сроков нет"))
	}
	if n := len(m.buckets.Overdue); n > 0 {
		right = append(right, stUrgent.Render("просрочено "+fmt.Sprint(n)))
	}
	rw := 0
	for _, r := range right {
		rw = max(rw, lipgloss.Width(r))
	}
	lines := make([]string, 8)
	for i, l := range clock {
		if 1+cw+1+rw <= innerW && i < len(right) {
			l += strings.Repeat(" ", cw-lipgloss.Width(l)) + " " + right[i]
		}
		lines[i+1] = " " + l
	}
	d0 := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())
	d1 := time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, now.Location())
	frac := min(max(float64(now.Sub(d0))/float64(d1.Sub(d0)), 0), 1)
	pct := fmt.Sprintf("%d%%", int(frac*100+0.5))
	mw := max(innerW-5-1-len(pct)-6, 1)
	lines[7] = stDim.Render("день ") + meter(mw, frac, cAccent, cAmber) +
		" " + stBold.Render(pct) + stDim.Render(" 18:00")
	return lines
}

// homeNextDue — ближайший срок: стадия, проект, обратный отсчёт и остальные
// сегодняшние задачи.
func (m Model) homeNextDue(innerW int) []string {
	t, ok := m.nextDue()
	if !ok {
		return []string{stDim.Render(" сроков нет — можно выдохнуть ")}
	}
	now := m.api.now().In(m.srvLoc())
	dl, _ := ParseDeadline(t.Deadline, m.srvLoc())
	head := ""
	if t.Stage != "" {
		head = pill(t.Stage, cInk, stageColor(t.StageColor)) + " "
	}
	head = truncate(head+stTitle.Render(t.Title), innerW)
	meta := ""
	if t.Project != "" {
		meta = stDim.Render(t.Project)
	}
	if t.NewComments > 0 {
		sep := " · "
		if meta == "" {
			sep = ""
		}
		meta += sep + lipgloss.NewStyle().Bold(true).Foreground(pulseColor(m.frame)).
			Render(fmt.Sprintf("✎ %d %s",
				t.NewComments, plural(t.NewComments, "новый", "новых", "новых")))
	}
	d := dl.Sub(now)
	label := untilLabel(d)
	switch {
	case d < 0:
		label = stUrgent.Bold(true).Render(label)
	case d < 3*time.Hour:
		label = lipgloss.NewStyle().Bold(true).Foreground(pulseColor(m.frame)).Render(label)
	default:
		label = stAmber.Render(label)
	}
	if mw := min(26, innerW-lipgloss.Width(label)-1); mw >= 1 {
		frac := 1 - float64(d)/float64(24*time.Hour)
		label += " " + meter(mw, frac, cAccent, cAmber)
	}
	lines := []string{head, meta, "", label, ""}
	var rest []Task
	for _, x := range m.buckets.Today {
		if x.ID != t.ID {
			rest = append(rest, x)
		}
	}
	if len(rest) > 3 {
		rest = rest[:3]
	}
	if len(rest) > 0 {
		lines = append(lines, stDim.Render("ещё сегодня"))
		for _, x := range rest {
			lines = append(lines, m.homeTaskRow(x, innerW))
		}
	}
	return lines
}

// homeTaskRow — строка «ещё сегодня»: точка стадии, титул, проект справа.
func (m Model) homeTaskRow(t Task, innerW int) string {
	dot := lipgloss.NewStyle().Foreground(stageColor(t.StageColor)).Render("●")
	proj := stDim.Render(t.Project)
	row := dot + " " + truncate(t.Title, max(innerW-lipgloss.Width(proj)-3, 1))
	gap := max(innerW-lipgloss.Width(row)-lipgloss.Width(proj), 1)
	return row + strings.Repeat(" ", gap) + proj
}

// homeChats — непрочитанные чаты по строке; сколько влезает в колонку рядом
// с канбаном и неделей.
func (m Model) homeChats(innerW int) []string {
	var unread []Chat
	for _, c := range m.chats {
		if c.Counter > 0 {
			unread = append(unread, c)
		}
	}
	n := len(unread)
	if lim := m.bodyH - 2 - len(m.homeKanban(innerW)) - 2 - len(m.homeWeek(innerW)) - 2; n > max(lim, 1) {
		n = max(lim, 1)
	}
	if n == 0 {
		return []string{stDim.Render(" всё прочитано ✓")}
	}
	rem := max(innerW-13, 2) // аватар, зазоры, время и точка справа
	nameW := rem / 2
	prevW := rem - nameW
	rows := make([]string, n)
	for i, c := range unread[:n] {
		line := avatar(c.Title, strings.HasPrefix(c.DialogID, "chat"), seedOf(c.DialogID)) +
			" " + stTitle.Render(truncate(c.Title, nameW)) + " " +
			stDim.Render(truncate(c.Preview, prevW))
		right := stDim.Render(c.Time) + " " + stAccent.Render("●")
		gap := max(innerW-lipgloss.Width(line)-lipgloss.Width(right), 1)
		rows[i] = line + strings.Repeat(" ", gap) + right
	}
	return rows
}

// homeKanban — сплошная полоса задач по стадиям и легенда со счётчиками.
func (m Model) homeKanban(innerW int) []string {
	var do []Task
	for _, t := range m.tasks {
		if t.Role == "do" {
			do = append(do, t)
		}
	}
	if len(do) == 0 {
		return []string{stDim.Render(" задач нет ")}
	}
	type seg struct {
		name, color string
		sort, n     int
	}
	idx := map[string]int{}
	var segs []seg
	for _, t := range do {
		i, ok := idx[t.Stage]
		if !ok {
			idx[t.Stage] = len(segs)
			segs = append(segs, seg{name: t.Stage, color: t.StageColor, sort: t.StageSort})
			i = len(segs) - 1
		}
		if t.StageSort < segs[i].sort {
			segs[i].sort = t.StageSort
			segs[i].color = t.StageColor
		}
		segs[i].n++
	}
	sort.SliceStable(segs, func(a, b int) bool { return segs[a].sort < segs[b].sort })
	// ширины пропорциональны числу задач, минимум 1 ячейка; остаток — крупным
	widths := make([]int, len(segs))
	sum := 0
	for i, s := range segs {
		widths[i] = max(s.n*innerW/len(do), 1)
		sum += widths[i]
	}
	byN := make([]int, len(segs))
	for i := range segs {
		byN[i] = i
	}
	sort.SliceStable(byN, func(a, b int) bool { return segs[byN[a]].n > segs[byN[b]].n })
	for i := 0; sum < innerW; i++ {
		widths[byN[i%len(byN)]]++
		sum++
	}
	for i := 0; sum > innerW && i < len(segs); i++ { // перебор снимаем с мелких
		for sum > innerW && widths[byN[len(byN)-1-i]] > 1 {
			widths[byN[len(byN)-1-i]]--
			sum--
		}
	}
	var bar string
	used := 0
	for i, s := range segs {
		w := min(widths[i], innerW-used)
		if w <= 0 {
			break
		}
		col := stageColor(s.color)
		if s.name == "" {
			col = cFaint // без стадии — нейтральный сегмент
		}
		bar += lipgloss.NewStyle().Foreground(col).Render(strings.Repeat("█", w))
		used += w
	}
	lines := []string{bar, ""}
	var cur string
	for _, s := range segs {
		name, col := s.name, stageColor(s.color)
		if name == "" {
			name, col = "—", cFaint
		}
		e := lipgloss.NewStyle().Foreground(col).Render("●") + " " +
			name + " " + stBold.Render(fmt.Sprint(s.n))
		add := e
		if cur != "" {
			add = "   " + e
		}
		if cur != "" && lipgloss.Width(cur)+lipgloss.Width(add) > innerW {
			lines = append(lines, cur)
			cur = e
			continue
		}
		cur += add
	}
	return append(lines, cur)
}

// homeCalendar — месяц пн-первый: сегодня плашкой, маркеры — сроки моих задач.
func (m Model) homeCalendar(innerW int) []string {
	now := m.api.now().In(m.srvLoc())
	counts := m.homeDueCounts()
	todayKey := now.Format("2006-01-02")
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	lead := (int(first.Weekday()) + 6) % 7
	last := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
	weeks := (lead + last + 6) / 7
	head := ""
	for i, d := range []string{"пн", "вт", "ср", "чт", "пт", "сб", "вс"} {
		cell := fmt.Sprintf("%3s", d)
		if i >= 5 {
			cell = stFaint.Render(cell)
		}
		head += cell + "   "
	}
	lines := []string{head}
	for w := 0; w < weeks; w++ {
		var b strings.Builder
		for i := 0; i < 7; i++ {
			day := w*7 + i - lead + 1
			if day < 1 || day > last {
				b.WriteString("      ") // день чужого месяца
				continue
			}
			key := fmt.Sprintf("%04d-%02d-%02d", now.Year(), now.Month(), day)
			var num string
			if key == todayKey {
				num = pill(fmt.Sprint(day), cInk, cAccent)
			} else if i >= 5 {
				num = stFaint.Render(fmt.Sprintf("%3d", day))
			} else {
				num = fmt.Sprintf("%3d", day)
			}
			marker := " "
			if cnt := counts[key]; cnt == 1 {
				marker = "•"
			} else if cnt > 1 {
				marker = "●"
			}
			if marker != " " {
				var col color.Color = cAccent
				switch {
				case key < todayKey:
					col = cUrgent
				case key == todayKey:
					col = pulseColor(m.frame)
				}
				marker = lipgloss.NewStyle().Foreground(col).Render(marker)
			}
			b.WriteString(num + marker + strings.Repeat(" ", max(6-lipgloss.Width(num)-1, 1)))
		}
		lines = append(lines, b.String())
	}
	// легенда: сроки на сегодня, текущую неделю и месяц
	monOff := (int(now.Weekday()) + 6) % 7
	weekN, monthN := 0, 0
	for k, c := range counts {
		if strings.HasPrefix(k, fmt.Sprintf("%04d-%02d", now.Year(), now.Month())) {
			monthN += c
		}
	}
	for i := 0; i < 7; i++ {
		weekN += counts[now.AddDate(0, 0, i-monOff).Format("2006-01-02")]
	}
	lines = append(lines, stDim.Render(fmt.Sprintf("● сегодня %d · неделя %d · месяц %d",
		counts[todayKey], weekN, monthN)))
	return lines
}

// homeWeek — столбики дедлайнов пн..вс, максимум 4 ступени.
func (m Model) homeWeek(innerW int) []string {
	now := m.api.now().In(m.srvLoc())
	counts := m.homeDueCounts()
	todayKey := now.Format("2006-01-02")
	mon := now.AddDate(0, 0, -(int(now.Weekday())+6)%7)
	type dline struct {
		label string
		n     int
		past  bool
		today bool
	}
	days := make([]dline, 7)
	maxC := 0
	for i := range days {
		d := mon.AddDate(0, 0, i)
		key := d.Format("2006-01-02")
		days[i] = dline{weekdayShort[d.Weekday()], counts[key], key < todayKey, key == todayKey}
		maxC = max(maxC, min(days[i].n, 4))
	}
	colOf := func(d dline) color.Color {
		switch {
		case d.today:
			return pulseColor(m.frame)
		case d.past:
			return cUrgent
		default:
			return cAccent
		}
	}
	rows := maxC + 1 // столбики плюс строка чисел над самым высоким
	// клетка дня сжимается под ширину, чтобы строки не резались рамкой
	cellW := max(min(8, innerW/7), 2)
	barW := max(cellW-2, 2)
	baseW := max(cellW-2, 3)
	var lines []string
	for r := 0; r < rows; r++ {
		var b strings.Builder
		for _, d := range days {
			c := min(d.n, 4)
			switch {
			case c > 0 && r == rows-c-1: // число над столбиком
				num := fmt.Sprint(d.n)
				b.WriteString(" " + lipgloss.NewStyle().Foreground(colOf(d)).Render(num) +
					strings.Repeat(" ", max(cellW-1-len(num), 0)))
			case c > 0 && r > rows-c-1:
				b.WriteString(lipgloss.NewStyle().Foreground(colOf(d)).Render(strings.Repeat("█", barW)) +
					strings.Repeat(" ", cellW-barW))
			case r == rows-1:
				b.WriteString(" " + stFaint.Render("·") + strings.Repeat(" ", max(cellW-2, 0))) // пустой день
			default:
				b.WriteString(strings.Repeat(" ", cellW))
			}
		}
		lines = append(lines, b.String())
	}
	base, labels := "", ""
	for i, d := range days {
		base += stFaint.Render(strings.Repeat("─", baseW)) + strings.Repeat(" ", max(cellW-baseW, 0))
		if d.today {
			labels += pill(d.label, cInk, cAccent) + strings.Repeat(" ", max(cellW-4, 0))
		} else if i >= 5 {
			labels += stFaint.Render(d.label) + strings.Repeat(" ", max(cellW-2, 0))
		} else {
			labels += stDim.Render(d.label) + strings.Repeat(" ", max(cellW-2, 0))
		}
	}
	return append(lines, base, labels)
}

// homeFurther — сколько дедлайнов моих задач позже воскресенья.
func (m Model) homeFurther() int {
	now := m.api.now().In(m.srvLoc())
	nextMon := now.AddDate(0, 0, 7-(int(now.Weekday())+6)%7).Format("2006-01-02")
	n := 0
	for k, c := range m.homeDueCounts() {
		if k >= nextMon {
			n += c
		}
	}
	return n
}

// homeWidget — виджет пульта: заголовок рамки, акцентная рамка, контент.
type homeWidget struct {
	title  string
	active bool
	lines  []string
}

// homeColLeft — левая колонка: «сегодня», «следующий срок», календарь.
func (m Model) homeColLeft(innerW int) []homeWidget {
	now := m.api.now().In(m.srvLoc())
	return []homeWidget{
		{boxTitle("сегодня", "", innerW), false, m.homeToday(innerW)},
		{boxTitle("следующий срок", "", innerW), true, m.homeNextDue(innerW)},
		{boxTitle(monthNom[now.Month()-1], "", innerW), false, m.homeCalendar(innerW)},
	}
}

// homeColRight — правая колонка: «чаты», «канбан · проекты», «неделя».
func (m Model) homeColRight(innerW int) []homeWidget {
	chatsRight := ""
	if n := m.unreadChats(); n > 0 {
		chatsRight = stAccent.Render(fmt.Sprintf("%d ждут", n))
	}
	return []homeWidget{
		{boxTitle("чаты", chatsRight, innerW), false, m.homeChats(innerW)},
		{boxTitle("канбан · проекты", stDim.Render(fmt.Sprint(m.myOpenCount())), innerW), false, m.homeKanban(innerW)},
		{boxTitle("неделя", stDim.Render(fmt.Sprintf("дальше: %d", m.homeFurther())), innerW), false, m.homeWeek(innerW)},
	}
}

// homeColumn — рамки колонки: не влезающие по высоте отбрасываются снизу,
// последний влезающий виджет растягивается пустыми строками до низа.
func homeColumn(ws []homeWidget, innerW, h int) []string {
	var kept []homeWidget
	total := 0
	for _, w := range ws {
		if total+len(w.lines)+2 > h {
			break
		}
		kept = append(kept, w)
		total += len(w.lines) + 2
	}
	if len(kept) == 0 {
		return make([]string, h)
	}
	if fill := h - total; fill > 0 {
		last := &kept[len(kept)-1]
		last.lines = append(last.lines, make([]string, fill)...)
	}
	var out []string
	for _, w := range kept {
		out = append(out, strings.Split(box(w.title, w.lines, innerW, w.active), "\n")...)
	}
	return out
}

// viewHome — пульт дня: на широком экране две колонки, на узком — одна.
func (m Model) viewHome() string {
	if m.width < 100 {
		inner := m.width - 4
		l, r := m.homeColLeft(inner), m.homeColRight(inner)
		// сегодня, следующий срок, чаты, канбан, календарь, неделя
		ws := []homeWidget{l[0], l[1], r[0], r[1], l[2], r[2]}
		return strings.Join(homeColumn(ws, inner, m.bodyH), "\n")
	}
	leftW := (m.width - 1) / 2
	rightW := m.width - leftW - 1
	left := homeColumn(m.homeColLeft(leftW-4), leftW-4, m.bodyH)
	right := homeColumn(m.homeColRight(rightW-4), rightW-4, m.bodyH)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		strings.Join(left, "\n"), " ", strings.Join(right, "\n"))
}
