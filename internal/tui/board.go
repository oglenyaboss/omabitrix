package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/wordwrap"
)

// boardCol — колонка доски: одна стадия канбана проекта.
type boardCol struct {
	title, color string
	rows         []taskRow
}

// boardRole — текущая роль доски; пустая трактуется как «делаю».
func (m Model) boardRole() string {
	if m.taskRole == "" {
		return "do"
	}
	return m.taskRole
}

// boardColumns — задачи роли, сгруппированные по стадиям: колонки по
// минимальному StageSort (пустая стадия — «—» последней), внутри — по дедлайну
// (без дедлайна — в конце, при равенстве по ID). kind — корзина дедлайна,
// как в Buckets, для help/watch тоже.
func (m Model) boardColumns() []boardCol {
	role := m.boardRole()
	var tasks []Task
	for _, t := range m.tasks {
		if t.Role != role {
			continue
		}
		if m.taskProject != 0 && t.GroupID != m.taskProject {
			continue
		}
		if m.filter != "" && !strings.Contains(strings.ToLower(t.Title), strings.ToLower(m.filter)) {
			continue
		}
		tasks = append(tasks, t)
	}
	byStage := map[string][]Task{}
	for _, t := range tasks {
		byStage[t.Stage] = append(byStage[t.Stage], t)
	}
	loc := m.srvLoc()
	today := m.api.now().In(loc).Format("2006-01-02")
	kindOf := func(t Task) string {
		d, ok := ParseDeadline(t.Deadline, loc)
		if !ok {
			return "nodate"
		}
		day := d.In(loc).Format("2006-01-02")
		switch {
		case day < today:
			return "overdue"
		case day == today:
			return "today"
		default:
			return "later"
		}
	}
	cols := make([]boardCol, 0, len(byStage))
	for title, group := range byStage {
		sort.Slice(group, func(i, j int) bool {
			di, oki := ParseDeadline(group[i].Deadline, loc)
			dj, okj := ParseDeadline(group[j].Deadline, loc)
			switch {
			case oki != okj:
				return oki
			case oki:
				return di.Before(dj)
			default:
				return group[i].ID < group[j].ID
			}
		})
		c := boardCol{title: title, color: group[0].StageColor}
		if title == "" {
			c.title = "—" // без стадии — последняя колонка
		}
		for _, t := range group {
			c.rows = append(c.rows, taskRow{task: t, kind: kindOf(t)})
		}
		cols = append(cols, c)
	}
	minSort := func(c boardCol) int {
		min := 1 << 30
		for _, r := range c.rows {
			if r.task.StageSort < min {
				min = r.task.StageSort
			}
		}
		return min
	}
	sort.Slice(cols, func(i, j int) bool {
		// пустая стадия всегда последней (заголовок «—»)
		empty := func(c boardCol) bool { return c.rows[0].task.Stage == "" }
		if empty(cols[i]) != empty(cols[j]) {
			return empty(cols[j])
		}
		si, sj := minSort(cols[i]), minSort(cols[j])
		if si != sj {
			return si < sj
		}
		return cols[i].title < cols[j].title
	})
	for i := range cols {
		for j := range cols[i].rows {
			cols[i].rows[j].col = i
		}
	}
	return cols
}

// moveCol — курсор на тот же номер строки в соседней колонке, кламп к её длине.
func (m *Model) moveCol(dir int) {
	rows := m.flatTasks()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return
	}
	cols := m.boardColumns()
	from := rows[m.cursor].col
	to := from + dir
	if to < 0 || to >= len(cols) {
		return
	}
	row := 0
	for i := 0; i < m.cursor; i++ {
		if rows[i].col == from {
			row++
		}
	}
	base := 0
	for i := 0; i < to; i++ {
		base += len(cols[i].rows)
	}
	m.cursor = base + min(row, len(cols[to].rows)-1)
}

// moveStage — оптимистичный перенос задачи в соседнюю стадию её проекта:
// поля стадии меняются сразу, сервер подтверждает (или откатит перечиткой).
func (m Model) moveStage(dir int) (tea.Model, tea.Cmd) {
	rows := m.flatTasks()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return m, nil
	}
	t := rows[m.cursor].task
	stages := m.api.StagesOf(t.GroupID)
	if len(stages) == 0 {
		m.flash = "у задачи нет канбана проекта"
		m.flashAt = m.api.now()
		return m, nil
	}
	idx := 0
	for i, s := range stages {
		if s.ID == t.StageID {
			idx = i
			break
		}
	}
	dst := idx + dir
	if dst < 0 || dst >= len(stages) {
		return m, nil // край канбана
	}
	key := strconv.FormatInt(t.ID, 10)
	for i := range m.tasks {
		if m.tasks[i].ID == t.ID {
			m.tasks[i].StageID = stages[dst].ID
			m.tasks[i].Stage = stages[dst].Title
			m.tasks[i].StageColor = stages[dst].Color
			m.tasks[i].StageSort = dst
			break
		}
	}
	m.restoreCursor(key) // курсор остаётся на той же задаче
	m.flash = "→ " + stages[dst].Title
	m.flashAt = m.api.now()
	return m, m.moveStageCmd(t.ID, stages[dst].ID)
}

type stageMsg struct{ err error }

func (m Model) moveStageCmd(taskID, stageID int64) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg { return stageMsg{err: api.MoveStage(ctx, taskID, stageID)} }
}

// nextProject — следующий проект роли: 0 → GroupID проектов роли по имени
// по алфавиту → 0.
func (m *Model) nextProject() int64 {
	role := m.boardRole()
	projects := map[int64]string{}
	for _, t := range m.tasks {
		if t.Role == role && t.GroupID != 0 {
			projects[t.GroupID] = t.Project
		}
	}
	if len(projects) == 0 {
		return 0
	}
	ids := make([]int64, 0, len(projects))
	for id := range projects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if projects[ids[i]] != projects[ids[j]] {
			return projects[ids[i]] < projects[ids[j]]
		}
		return ids[i] < ids[j]
	})
	if m.taskProject == 0 {
		return ids[0]
	}
	for i, id := range ids {
		if id == m.taskProject {
			if i+1 < len(ids) {
				return ids[i+1]
			}
			return 0
		}
	}
	return 0 // фильтр остался от другой роли — сбрасываем
}

// viewBoard — канбан-доска вкладки задач: строка чипов ролей/проекта,
// под ней колонки стадий. Высота — не больше m.bodyH.
func (m Model) viewBoard() string {
	cols := m.boardColumns()
	rows := m.flatTasks()
	hiddenL, hiddenR := 0, 0
	var board string
	if len(cols) == 0 {
		// пустая рамка — той же высоты, что и колонки: футер прижат к низу
		inner := make([]string, max(m.bodyH-4, 1))
		inner[0] = stDim.Render(" — задач нет — ")
		board = box("", inner, max(m.width-4, 1), true)
	} else {
		n := len(cols)
		colW := (m.width - (n - 1)) / n
		if colW < 28 {
			colW = 28
		}
		vis := n
		if n*colW+(n-1) > m.width {
			// все колонки не влезают — окно из влезающих, курсор в кадре
			vis = (m.width + 1) / (colW + 1)
			if vis < 1 {
				vis = 1
			}
		}
		selCol, selRow := -1, -1
		if m.cursor >= 0 && m.cursor < len(rows) {
			selCol = rows[m.cursor].col
			for i := 0; i < m.cursor; i++ {
				if rows[i].col == selCol {
					selRow++
				}
			}
		}
		start := 0
		if selCol >= vis {
			start = selCol - vis + 1
		}
		hiddenL, hiddenR = start, n-start-vis
		colH := max(m.bodyH-2, 3)
		blocks := make([]string, 0, vis*2-1)
		for i := start; i < start+vis && i < n; i++ {
			if i > start {
				blocks = append(blocks, " ")
			}
			sel := -1
			if i == selCol {
				sel = selRow
			}
			blocks = append(blocks, m.boardColView(cols[i], sel, colW, colH))
		}
		board = lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
	}
	return m.boardChipsRow(hiddenL, hiddenR) + "\n\n" + board
}

// boardChipsRow — строка 1 доски: роли, фильтр проекта, текстовый фильтр;
// справа — счётчик скрытых колонок слева/справа от окна.
func (m Model) boardChipsRow(hiddenL, hiddenR int) string {
	role := m.boardRole()
	count := func(r string) int {
		n := 0
		for _, t := range m.tasks {
			if t.Role == r && (m.taskProject == 0 || t.GroupID == m.taskProject) {
				n++
			}
		}
		return n
	}
	var left string
	for i, r := range []struct{ key, label string }{
		{"do", "делаю"}, {"help", "помогаю"}, {"watch", "наблюдаю"},
	} {
		label := fmt.Sprintf("%s %d", r.label, count(r.key))
		if r.key == role {
			left += pill(label, cInk, cAccent)
		} else {
			left += chip(label)
		}
		if i < 2 {
			left += " "
		}
	}
	left += "   " + stDim.Render("проект") + " "
	if m.taskProject == 0 {
		left += chip("все")
	} else {
		name := fmt.Sprint(m.taskProject)
		for _, t := range m.tasks {
			if t.GroupID == m.taskProject {
				name = t.Project
				break
			}
		}
		left += chip(name)
	}
	if m.filter != "" {
		left += " " + stDim.Render("фильтр: "+m.filter)
	}
	var right string
	if hiddenL > 0 {
		right += stDim.Render(fmt.Sprintf("‹ %d", hiddenL))
	}
	if hiddenR > 0 {
		if right != "" {
			right += "  "
		}
		right += stDim.Render(fmt.Sprintf("%d ›", hiddenR))
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		return truncate(left, m.width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// boardColView — колонка доски: рамка со заголовком стадии, полоса цвета
// стадии, карточки с прокруткой окна (курсор держится в кадре).
func (m Model) boardColView(c boardCol, sel, colW, colH int) string {
	innerW := max(colW-4, 1)
	bar := lipgloss.NewStyle().Foreground(stageColor(c.color)).
		Render(strings.Repeat("▔", innerW))
	lines := []string{bar}
	avail := colH - 3 // после полосы стадии
	// окно карточек: сколько влезает, курсор в кадре, индикаторы «ещё»
	cap := avail / 5
	off := 0
	for cap > 0 {
		off = 0
		if sel >= cap {
			off = sel - cap + 1
		}
		if off+cap > len(c.rows) {
			off = len(c.rows) - cap
		}
		if off < 0 {
			off = 0
		}
		used := cap * 5
		if off > 0 {
			used++
		}
		if off+cap < len(c.rows) {
			used++
		}
		if used <= avail {
			break
		}
		cap--
	}
	if cap > 0 {
		if off > 0 {
			lines = append(lines, stDim.Render(fmt.Sprintf("↑ ещё %d", off)))
		}
		for i := off; i < off+cap && i < len(c.rows); i++ {
			lines = append(lines, m.boardCard(c.rows[i], i == sel, innerW)...)
		}
		if off+cap < len(c.rows) {
			lines = append(lines, stDim.Render(fmt.Sprintf("ещё %d", len(c.rows)-off-cap)))
		}
	}
	// колонка тянется на всю высоту доски: недостающее — пустые строки
	for len(lines) < colH-2 {
		lines = append(lines, "")
	}
	if len(lines) > colH-2 {
		lines = lines[:colH-2]
	}
	title := lipgloss.NewStyle().Foreground(stageColor(c.color)).Render("●") + " " +
		stTitle.Render(c.title) + " " + stDim.Render(fmt.Sprint(len(c.rows)))
	return box(title, lines, innerW, sel >= 0)
}

// boardCard — карточка задачи: 5 строк, проект врезан в верхнюю грань,
// название в две строки, снизу плашка срока и бейдж новых комментариев.
func (m Model) boardCard(r taskRow, sel bool, innerW int) []string {
	cardW := max(innerW-6, 4)
	st := stTrack
	if sel {
		st = stCursor
	}
	out := make([]string, 0, 5)
	out = append(out, " "+boxTop(stDim.Render(r.task.Project), cardW, st))
	// название: перенос по словам на две строки, остаток обрезан
	wrap := strings.Split(wordwrap.String(r.task.Title, cardW), "\n")
	render := func(s string) string {
		if pad := cardW - lipgloss.Width(s); pad > 0 {
			s += strings.Repeat(" ", pad)
		}
		return st.Render("│ ") + s + st.Render(" │")
	}
	line2 := ""
	if len(wrap) > 1 {
		line2 = stDim.Render(truncate(strings.Join(wrap[1:], " "), cardW))
	}
	out = append(out, " "+render(stTitle.Render(truncate(wrap[0], cardW))), " "+render(line2))
	var due string
	if d, ok := ParseDeadline(r.task.Deadline, m.srvLoc()); ok {
		now := m.api.now().In(m.srvLoc())
		switch {
		case r.kind == "overdue":
			due = pill("просрочено "+dueLabel(d, now), cInk, cUrgent)
		case r.kind == "today":
			due = pill(dueLabel(d, now), cInk, pulseColor(m.frame))
		case dayDiff(d, now) < 7:
			due = chip(dueLabel(d, now))
		default:
			due = stDim.Render(dueLabel(d, now))
		}
	} else {
		due = stFaint.Render("без срока")
	}
	if r.task.NewComments > 0 {
		badge := stBadge.Render("✎ " + fmt.Sprint(r.task.NewComments))
		gap := cardW - lipgloss.Width(due) - lipgloss.Width(badge)
		if gap < 1 {
			gap = 1
		}
		due += strings.Repeat(" ", gap) + badge
	}
	out = append(out, " "+render(due), " "+boxBottom(cardW, st))
	return out
}

// dayDiff — полных календарных дней между датами в зоне dl.
func dayDiff(dl, now time.Time) int {
	dd := time.Date(dl.Year(), dl.Month(), dl.Day(), 0, 0, 0, 0, dl.Location())
	nd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, dl.Location())
	return int(dd.Sub(nd).Hours() / 24)
}
