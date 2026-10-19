package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() tea.View {
	var s string
	if !m.ready || m.width == 0 {
		if m.loadErr != "" {
			s = SplashErr(m.spinner.View(), m.loadErr)
		} else {
			s = Splash(m.spinner.View())
		}
	} else if m.overlay == ovHelp {
		s = m.viewHelp()
	} else {
		s = m.tabBar() + "\n"
		switch m.tab {
		case tabHome:
			s += m.viewHome()
		case tabChats:
			s += m.viewChats()
		case tabTasks:
			s += m.viewTasks()
		case tabNotify:
			s += m.viewNotify()
		}
		s += "\n" + m.footer()
	}
	v := tea.NewView(s)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// tabBar — строка вкладок: активная — акцентной плашкой с хоткеем,
// у остальных счётчики; справа дата/время и связь с порталом.
func (m Model) tabBar() string {
	left := stAccent.Bold(true).Render("◆ ") + stTitle.Render("omabitrix") + "   "
	tabs := []struct {
		t    tab
		name string
		n    int
	}{
		{tabHome, "пульт", 0},
		{tabTasks, "задачи", m.myOpenCount()},
		{tabChats, "чаты", m.unreadChats()},
		{tabNotify, "уведы", countUnread(m.notify)},
	}
	for i, t := range tabs {
		if t.t == m.tab {
			label := fmt.Sprintf("%d %s", i+1, t.name)
			if t.n > 0 {
				label += " " + fmt.Sprint(t.n)
			}
			left += pill(label, cInk, cAccent) + " "
			continue
		}
		left += " " + stDim.Render(fmt.Sprint(i+1)) + " " + t.name
		if t.n > 0 {
			left += " " + stBadge.Render(fmt.Sprint(t.n))
		}
		left += "  "
	}
	dot := stAccent.Render("●")
	if m.loadErr != "" {
		dot = stUrgent.Render("●")
	}
	right := stDim.Render(clockLabel(m.api.now())) + "  " + dot + stDim.Render(" портал")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		return truncate(left, m.width)
	}
	return left + strings.Repeat(" ", gap) + right
}

// myOpenCount — открытые задачи, где я исполнитель (счётчик вкладки).
func (m Model) myOpenCount() int {
	n := 0
	for _, t := range m.tasks {
		if t.Role == "do" {
			n++
		}
	}
	return n
}

// unreadChats — чаты с непрочитанным (счётчик вкладки).
func (m Model) unreadChats() int {
	n := 0
	for _, c := range m.chats {
		if c.Counter > 0 {
			n++
		}
	}
	return n
}

// Номер панели больше не врезан в рамку — он в строке вкладок;
// активная панель — акцентной рамкой.

func paneTitle(num, name string, extra string, active bool) string {
	idx := ""
	if num != "" {
		idx = stBadgeDm.Render("[" + num + "]")
		if active {
			idx = stCursor.Render("[" + num + "]")
		}
		idx += " "
	}
	nameS := stTitle.Render(name)
	if !active {
		nameS = stDim.Render(name)
	}
	return idx + nameS + extra
}

// ---- чаты: слева список, справа диалог

func (m Model) viewChats() string {
	return lipgloss.JoinHorizontal(lipgloss.Top, m.chatListPane(), "  ", m.dialogPane())
}

func (m Model) unreadTotal() int {
	n := 0
	for _, c := range m.chats {
		n += c.Counter
	}
	return n
}

func (m Model) chatListPane() string {
	vc := m.visibleChats()
	start, end, filterRows := m.chatWindow(vc)
	rows := make([]string, 0, (end-start)*2+3)
	if filterRows > 0 {
		if m.focus == focusFilter {
			rows = append(rows, m.filterIn.View(), "")
		} else {
			rows = append(rows, stDim.Render("фильтр: "+m.filter), "")
		}
	}
	if len(vc) == 0 {
		rows = append(rows, stDim.Render("— пусто —"))
	}
	for i := start; i < end; i++ {
		rows = append(rows, m.chatRows(vc[i], i == m.cursor)...)
	}
	active := m.focus != focusReply
	title := paneTitle("", "чаты", unreadExtra(m.unreadTotal()), active)
	return box(title, rows, m.listW, active)
}

// chatRows — две строки на чат: имя + счётчик справа; превью + время справа.
func (m Model) chatRows(c Chat, selected bool) []string {
	inner := m.listW
	marker := " "
	if selected {
		marker = stCursor.Render("▌")
	}
	dot := " "
	if c.Online {
		dot = stOnline.String()
	}
	name := c.Title
	nameSt := lipgloss.NewStyle()
	if c.Counter > 0 {
		nameSt = stChatUnread
	}
	if selected {
		nameSt = stTitle
	}
	name = nameSt.Render(truncate(name, inner-9))
	var badgeStr string
	if c.Counter > 0 {
		badgeStr = stBadge.Render(fmt.Sprint(c.Counter))
	}
	gap := inner - 3 - lipgloss.Width(name) - lipgloss.Width(badgeStr)
	if gap < 1 {
		gap = 1
	}
	line1 := marker + dot + " " + name + strings.Repeat(" ", gap) + badgeStr

	preview := stDim.Render("   " + truncate(c.Preview, inner-10))
	timeGap := inner - 4 - lipgloss.Width(preview) - lipgloss.Width(c.Time)
	if timeGap < 1 {
		timeGap = 1
	}
	line2 := preview + strings.Repeat(" ", timeGap) + stDim.Render(c.Time)
	return []string{line1, line2}
}

func unreadExtra(n int) string {
	if n <= 0 {
		return ""
	}
	return " " + stBadge.Render("·"+fmt.Sprint(n))
}

// chatWindow — окно видимых чатов: единая точка истины для рендера и мыши.
func (m Model) chatWindow(vc []Chat) (start, end, filterRows int) {
	filterRows = 0
	if m.focus == focusFilter || m.filter != "" {
		filterRows = 2
	}
	capacity := (m.bodyH - 2 - filterRows) / 2
	if capacity < 1 {
		capacity = 1
	}
	start = 0
	if m.cursor > capacity-1 {
		start = m.cursor - capacity + 1
	}
	end = start + capacity
	if end > len(vc) {
		end = len(vc)
	}
	return start, end, filterRows
}

// filterRows — высота строки фильтра в панели списка: 2 (строка + пустая),
// как в панели чатов; 0 — когда фильтра нет.
func (m Model) filterRows() int {
	if m.focus == focusFilter || m.filter != "" {
		return 2
	}
	return 0
}

// filterLine — строка фильтра для панели списка: инпут под фокусом, иначе эхо.
func (m Model) filterLine() []string {
	if m.focus == focusFilter {
		return []string{m.filterIn.View(), ""}
	}
	return []string{stDim.Render("фильтр: " + m.filter), ""}
}

func (m Model) dialogPane() string {
	if m.active == nil {
		return box(paneTitle("", "диалог", "", false),
			[]string{"", stDim.Render(" enter — открыть чат")}, m.dialW, false)
	}
	if m.searchOn {
		return m.searchPane()
	}
	hist := joinScrollbar(m.vp.View(),
		scrollbarColumn(m.vp.TotalLineCount(), m.vp.Height(), m.vp.YOffset()), m.vp.Height())
	lines := strings.Split(hist, "\n")
	switch {
	case m.overlay == ovConfirm:
		lines = append(lines, "", stErr.Render("удалить сообщение? y/n"))
	case m.focus == focusReply:
		if m.editID > 0 {
			lines = append(lines, stDim.Render("✎ редактирование"))
		} else if m.replyTo.ID > 0 {
			q := m.replyTo
			name := q.Name
			if name == "" {
				name = "?"
			}
			lines = append(lines, stDim.Render(truncate("↳ ответ "+name+": "+q.Text, m.dialW-4)))
		}
		lines = append(lines, strings.Split(m.reply.View(), "\n")...)
	case m.focus == focusSearch:
		lines = append(lines, "", m.searchIn.View())
	default:
		lines = append(lines, "", stDim.Render(" r — ответить · колесо — скролл"))
	}
	title := paneTitle("", truncate(m.active.Title, m.dialW-8), unreadExtra(m.active.Counter), m.focus == focusReply)
	return box(title, lines, m.dialW, m.focus == focusReply)
}

// searchPane — результаты поиска вместо истории: окно строк по высоте vp
// со своим скроллбаром, счёт/статус — в заголовке.
func (m Model) searchPane() string {
	all := m.searchView()
	total := len(all)
	h := m.vp.Height()
	off := m.searchOff
	if max := total - h; off > max {
		off = max
	}
	if off < 0 {
		off = 0
	}
	end := off + h
	if end > total {
		end = total
	}
	// строки добиваются до ширины окна: колонка скроллбара не липнет
	// к тексту, а стоит у правого края (как viewport у истории);
	// пустые филлеры до высоты — тоже, у них та же колонка
	win := make([]string, 0, h)
	for _, l := range all[off:end] {
		win = append(win, padTo(l, m.vp.Width()))
	}
	for len(win) < h {
		win = append(win, padTo("", m.vp.Width()))
	}
	lines := strings.Split(joinScrollbar(strings.Join(win, "\n"),
		scrollbarColumn(total, h, off), h), "\n")
	lines = append(lines, "", stDim.Render(" esc — назад к истории"))
	sub := fmt.Sprint(len(m.searchRes))
	switch {
	case m.searching:
		sub = "ищу…"
	case len(m.searchRes) == 0:
		sub = "ничего не найдено"
	}
	title := paneTitle("", "поиск: "+m.searchQ+" · "+sub, "", m.focus == focusReply)
	return box(title, lines, m.dialW, m.focus == focusReply)
}

// searchView — все строки результатов: «Имя · день время» и текст с
// подсветкой первого вхождения запроса (свои/чужие одинаково).
func (m Model) searchView() []string {
	W := m.vp.Width() - 1
	if W < 10 {
		W = 10
	}
	out := make([]string, 0, len(m.searchRes)*2)
	for _, msg := range m.searchRes {
		name := msg.Name
		if name == "" {
			name = "?"
		}
		head := authorStyle(msg.AuthorID).Render(name) +
			stDim.Render(" · "+msg.DayLabel+" "+msg.Time)
		out = append(out, head, indentWrap(searchHighlight(msg.Text, m.searchQ), W, 2))
	}
	return out
}

// padTo — строка ровно w ячеек: короткая добивается пробелом, длинная режется.
func padTo(s string, w int) string {
	if pad := w - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return ansi.Truncate(s, w, "")
}

// searchMaxOff — максимальный строковый офсет окна результатов (0 — всё влезло).
func (m Model) searchMaxOff() int {
	max := len(m.searchView()) - m.vp.Height()
	if max < 0 {
		return 0
	}
	return max
}

// searchHighlight — первое вхождение запроса акцентом, без учёта регистра.
func searchHighlight(text, q string) string {
	if q == "" {
		return text
	}
	low, lq := strings.ToLower(text), strings.ToLower(q)
	i := strings.Index(low, lq)
	if i < 0 {
		return text
	}
	ri := len([]rune(low[:i])) // байтовый индекс → руны (кириллица)
	rq := len([]rune(lq))
	r := []rune(text)
	if ri+rq > len(r) {
		return text
	}
	return string(r[:ri]) + stAccent.Render(string(r[ri:ri+rq])) + string(r[ri+rq:])
}

// ---- задачи: канбан-доска (или карточка на всю ширину)

func (m Model) viewTasks() string {
	if !m.taskOpen {
		return m.viewBoard()
	}
	return m.taskCardPane()
}

func deadlineShort(dl string) string {
	if len(dl) >= 10 {
		d := dl[8:10] + "." + dl[5:7]
		if len(dl) >= 16 {
			d += " " + dl[11:16]
		}
		return d
	}
	return dl
}

// ---- карточка задачи (правая панель вкладки задач)

func (m Model) taskCardPane() string {
	t := m.taskCard
	inner := m.cardW
	cardInner := m.bodyH - 2
	if cardInner < 4 {
		cardInner = 4
	}
	if m.cardLoading {
		// карточка ещё грузится: тело — заглушка, id берём из запроса,
		// а не от старой карточки
		title := paneTitle("", "задача · #"+fmt.Sprint(m.cardReq), "", m.tab == tabTasks)
		return box(title, []string{stDim.Render("загрузка…")}, inner, m.tab == tabTasks)
	}
	menuH := 0
	switch m.overlay {
	case ovStatus:
		menuH = len(statusOptions(t.Status)) + 3
	case ovDeadline:
		menuH = len(deadlineOptions(m.api.now(), t.Deadline)) + 3
	}
	chatRows := 0
	if t.ChatID > 0 {
		chatRows = 2
	}
	descMax := cardInner - 5 - chatRows - menuH // шапка(4) + пустая после desc
	if descMax < 3 {
		descMax = 3
	}
	lines := make([]string, 0, 16)
	lines = append(lines, stTitle.Render(truncate(t.Title, inner-2)), "")
	meta := []string{}
	if t.CreatorName != "" {
		meta = append(meta, "от "+t.CreatorName)
	}
	if t.Deadline != "" {
		meta = append(meta, "󰔞 "+deadlineShort(t.Deadline))
	}
	if t.Project != "" {
		meta = append(meta, t.Project)
	}
	if t.Stage != "" {
		meta = append(meta, t.Stage)
	}
	meta = append(meta, "статус: "+statusName(t.Status))
	lines = append(lines, stDim.Render("  "+strings.Join(meta, "  ·  ")), "")
	if t.Description != "" {
		desc := wordwrapWrap(t.Description, inner-4, 2)
		l := strings.Split(desc, "\n")
		if len(l) > descMax {
			// descMax строк ВКЛЮЧАЯ строку-эллипсис
			l = append(l[:descMax-1], stDim.Render("  …"))
		}
		lines = append(lines, l...)
		lines = append(lines, "")
	}
	if t.ChatID > 0 {
		lines = append(lines, stDim.Render("  󰭹 чат задачи · c"), "")
	}
	switch m.overlay {
	case ovStatus:
		lines = append(lines, "")
		lines = append(lines, strings.Split(m.statusMenu(), "\n")...)
	case ovDeadline:
		lines = append(lines, "")
		lines = append(lines, strings.Split(m.deadlineMenu(), "\n")...)
	}
	title := paneTitle("", "задача · #"+fmt.Sprint(t.ID), "", m.tab == tabTasks)
	return box(title, lines, m.cardW, m.tab == tabTasks)
}

func (m Model) statusMenu() string {
	opts := statusOptions(m.taskCard.Status)
	rows := make([]string, 0, len(opts))
	for i, o := range opts {
		line := " " + o.name
		if i == m.statusIdx {
			line = stCursor.Render("▌") + " " + stTitle.Render(o.name)
		}
		rows = append(rows, line)
	}
	// меню вложено в панель: по 1 колонке на паддинг панели с каждой стороны
	return box("", rows, m.cardW-4, true)
}

func (m Model) deadlineMenu() string {
	opts := deadlineOptions(m.api.now(), m.taskCard.Deadline)
	rows := make([]string, 0, len(opts))
	for i, o := range opts {
		line := fmt.Sprintf(" %d %s", i+1, o.label)
		if i == m.deadlineIdx {
			line = stCursor.Render("▌") + stTitle.Render(fmt.Sprintf(" %d %s", i+1, o.label))
		}
		rows = append(rows, line)
	}
	return box("", rows, m.cardW-4, true)
}

func statusName(code string) string {
	names := map[string]string{
		"1": "новая", "2": "ждёт выполнения", "3": "в работе",
		"4": "на проверке", "5": "завершена", "6": "отложена", "7": "отклонена",
	}
	if n, ok := names[code]; ok {
		return n
	}
	return code
}

// ---- уведы

func (m Model) viewNotify() string {
	rows := m.visibleNotify()
	start, end := m.notifyWindow(len(rows))
	out := make([]string, 0, (end-start)*2+2)
	if m.filterRows() > 0 {
		out = append(out, m.filterLine()...)
	}
	if len(rows) == 0 {
		out = append(out, stDim.Render(" — уведомлений нет — (R — обновить) "))
	}
	for i := start; i < end; i++ {
		out = append(out, m.notifyRows(rows[i], i == m.cursor)...)
	}
	title := paneTitle("", "уведы", unreadExtra(countUnread(m.notify)), m.tab == tabNotify)
	return box(title, out, m.width-4, m.tab == tabNotify)
}
func countUnread(rows []NotifyItem) int {
	n := 0
	for _, r := range rows {
		if r.Unread {
			n++
		}
	}
	return n
}

func (m Model) notifyWindow(n int) (start, end int) {
	capacity := (m.bodyH - 2 - m.filterRows()) / 2
	if capacity < 1 {
		capacity = 1
	}
	start = 0
	if m.cursor > capacity-1 {
		start = m.cursor - capacity + 1
	}
	end = start + capacity
	if end > n {
		end = n
	}
	return start, end
}

// notifyRows — две строки на увед: глиф+титул+время; текст dim.
func (m Model) notifyRows(n NotifyItem, selected bool) []string {
	inner := m.width - 4
	glyph := "󰅎"
	switch n.Module {
	case "tasks":
		glyph = "󰄬"
	case "calendar":
		glyph = "󰃭"
	case "disk":
		glyph = "󰋊"
	}
	title := n.Title
	if title == "" {
		title = n.Text
	}
	titleSt := lipgloss.NewStyle()
	if n.Unread {
		titleSt = stChatUnread
	}
	if selected {
		titleSt = stTitle
	}
	t := titleSt.Render(truncate(title, inner-12))
	marker := " "
	if selected {
		marker = stCursor.Render("▌")
	}
	gap := inner - 3 - lipgloss.Width(t) - lipgloss.Width(n.Time)
	if gap < 1 {
		gap = 1
	}
	line1 := marker + glyph + " " + t + strings.Repeat(" ", gap) + stDim.Render(n.Time)
	body := ""
	if n.Text != "" && n.Text != title {
		body = stDim.Render("   " + truncate(n.Text, inner-4))
	}
	return []string{line1, body}
}

// ---- help

func (m Model) viewHelp() string {
	h := m.help
	h.SetWidth(m.width - 4) // узкий терминал: колонки обрежутся с «…»
	return "\n" + stTitle.Render("  хоткеи") + "\n\n" +
		h.FullHelpView(m.fullHelp()) + "\n\n" + stDim.Render("  любая клавиша — назад")
}

// footer: контекстная справка (shortHelp) + ошибки + флеш.
func (m Model) footer() string {
	var right string
	switch {
	case m.loadErr != "":
		right = stErr.Render("⚠ " + truncate(m.loadErr, m.width-6))
	case m.flashLine() != "":
		right = m.flashLine()
	case m.sending:
		right = stDim.Render("отправка…")
	}
	left := m.help.ShortHelpView(m.shortHelp())
	if max := m.width - lipgloss.Width(right) - 2; max > 0 && lipgloss.Width(left) > max {
		left = truncate(left, max) // узкий терминал: подсказки уступают ошибкам
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	return stFooter.Render(left + strings.Repeat(" ", gap) + right)
}

func truncate(s string, n int) string {
	if n <= 0 {
		return s
	}
	// ширина в ячейках: эмодзи (2 ячейки) не должны вылезать за n
	return ansi.Truncate(s, n, "…")
}

func wordwrapWrap(text string, width, indent int) string {
	return indentWrap(text, width, indent)
}
