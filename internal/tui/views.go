package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/wordwrap"
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
		// тост нового сообщения поверх экрана на любой вкладке
		if m.toastVisible() && m.width >= 50 && m.height >= 8 {
			s = m.toastOverlay(s)
		}
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
		{tabNotify, "уведы", m.notifyBadge()},
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

// chatRows — две строки на чат: аватар + имя + время; превью + онлайн/счётчик.
// Выбранный чат — целиком на фоне, строки добиты до listW.
func (m Model) chatRows(c Chat, selected bool) []string {
	inner := m.listW
	marker := " "
	if selected {
		marker = stCursor.Render("▌")
	}
	nameSt := stDim
	if c.Counter > 0 {
		nameSt = stChatUnread
	}
	if selected {
		nameSt = stTitle
	}
	// маркер + аватар + зазор + имя + минимум 1 до времени
	name := nameSt.Render(truncate(c.Title, max(inner-11, 1)))
	timeS := stDim.Render(c.Time)
	line1 := marker + avatar(c.Title, strings.HasPrefix(c.DialogID, "chat"), seedOf(c.DialogID)) +
		" " + name
	if gap := inner - lipgloss.Width(line1) - lipgloss.Width(timeS); gap > 0 {
		line1 += strings.Repeat(" ", gap)
	}
	line1 += timeS

	right2 := ""
	if c.Online {
		right2 += stAccent.Render("●")
	}
	if c.Counter > 0 {
		if right2 != "" {
			right2 += " "
		}
		right2 += stBadge.Render(fmt.Sprint(c.Counter))
	}
	// 5 пробелов — под маркер, аватар и зазор первой строки
	preview := stDim.Render(truncate(c.Preview, max(inner-6-lipgloss.Width(right2), 1)))
	line2 := "     " + preview
	if gap := inner - 5 - lipgloss.Width(preview) - lipgloss.Width(right2); gap > 0 {
		line2 += strings.Repeat(" ", gap)
	}
	line2 += right2

	bg := lipgloss.NewStyle().Background(cSelBg)
	if selected {
		line1 = bg.Render(padTo(line1, inner))
		line2 = bg.Render(padTo(line2, inner))
	}
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

// replySep — разделитель «── ответ/правка» во всю ширину панели.
func (m Model) replySep() string {
	s := "── ответ "
	if m.editID > 0 {
		s = "── правка "
	}
	if fill := m.dialW - lipgloss.Width(s); fill > 0 {
		s += strings.Repeat("─", fill)
	}
	return s
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
		// разделитель отделяет поле от истории: «── ответ» или «── правка»
		lines = append(lines, stAccent.Render(m.replySep()))
		if m.editID == 0 && m.replyTo.ID > 0 {
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
		if m.taskOpen {
			// на экране задачи поле ответа видно всегда: без фокуса — приглушённый
			// разделитель и плейсхолдер (или черновик)
			lines = append(lines, stDim.Render(m.replySep()))
			if v := m.reply.Value(); v != "" {
				lines = append(lines, stFaint.Render(truncate(v, m.dialW-4)))
			} else {
				lines = append(lines, stFaint.Render("enter или r — написать"))
			}
		} else {
			lines = append(lines, "", stDim.Render(" r — ответить · колесо — скролл"))
		}
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

// ---- задачи: канбан-доска или экран задачи (карточка + чат)

func (m Model) viewTasks() string {
	if !m.taskOpen {
		return m.viewBoard()
	}
	if m.twoPane() {
		return lipgloss.JoinHorizontal(lipgloss.Top, m.taskCardPane(), " ", m.taskChatPane())
	}
	return m.taskCardPane()
}

// ---- карточка задачи (левая панель экрана задачи)

// cardDescMax — сколько строк описания влезает в карточку при текущем меню.
// Общая точка для рендера и пейджинга.
func (m Model) cardDescMax() int {
	cardInner := max(m.bodyH-2, 4)
	menuH := 0
	switch m.overlay {
	case ovStatus:
		menuH = m.menuLen() + 3
	case ovDeadline:
		menuH = len(deadlineOptions(m.api.now(), m.taskCard.Deadline)) + 3
	}
	chatRows := 0
	if !m.twoPane() && m.taskCard.ChatID > 0 {
		chatRows = 2
	}
	d := cardInner - 6 - chatRows - menuH - 1 // шапка 6 + пустая после описания
	if d < 3 {
		d = 3
	}
	return d
}

func (m Model) taskCardPane() string {
	t := m.taskCard
	inner := m.cardW
	cardInner := max(m.bodyH-2, 4)
	if m.cardLoading {
		// карточка ещё грузится: тело — заглушка, id берём из запроса,
		// а не от старой карточки
		title := paneTitle("", "задача · #"+fmt.Sprint(m.cardReq), "", m.tab == tabTasks)
		return box(title, []string{stDim.Render("загрузка…")}, inner, m.tab == tabTasks)
	}
	descMax := m.cardDescMax()
	var lines []string
	// титул: перенос по словам до 2 строк
	wrap := strings.Split(wordwrap.String(t.Title, max(inner-2, 1)), "\n")
	lines = append(lines, stTitle.Render(truncate(wrap[0], inner-2)))
	if len(wrap) > 1 {
		lines = append(lines, stTitle.Render(truncate(strings.Join(wrap[1:], " "), inner-2)))
	}
	lines = append(lines, "", "  "+m.stageTrackLine(inner-2),
		"  "+m.cardMetaLine(),
		stFaint.Render("  "+strings.Repeat("─", max(inner-4, 1))))
	if t.Description != "" {
		dl := strings.Split(wordwrapWrap(t.Description, inner-4, 2), "\n")
		off := min(m.descOff, max(len(dl)-1, 0))
		end := min(off+descMax, len(dl))
		win := dl[off:end]
		if end < len(dl) {
			// индикатор вместо последней строки окна
			win = append(win[:max(len(win)-1, 0)], stFaint.Render("  ещё ↓"))
		}
		lines = append(lines, win...)
		lines = append(lines, "")
	}
	if !m.twoPane() && t.ChatID > 0 {
		lines = append(lines, stDim.Render("  󰭹 чат задачи · c"), "")
	}
	// карточка — полной высоты панели: пустоту добиваем перед меню (меню
	// прижато к низу), иначе рамки панелей экрана разъезжаются
	padTo := cardInner
	switch m.overlay {
	case ovStatus:
		padTo = cardInner - (m.menuLen() + 3)
	case ovDeadline:
		padTo = cardInner - (len(deadlineOptions(m.api.now(), t.Deadline)) + 3)
	}
	for len(lines) < padTo {
		lines = append(lines, "")
	}
	switch m.overlay {
	case ovStatus:
		lines = append(lines, "")
		lines = append(lines, strings.Split(m.statusMenu(), "\n")...)
	case ovDeadline:
		lines = append(lines, "")
		lines = append(lines, strings.Split(m.deadlineMenu(), "\n")...)
	}
	if len(lines) > cardInner {
		lines = lines[:cardInner]
	}
	title := paneTitle("", fmt.Sprintf("#%d · %s", t.ID, t.Project), "", m.tab == tabTasks)
	return box(title, lines, inner, m.tab == tabTasks)
}

// stageTrackLine — трек стадий проекта: пройдено «●» dim, текущая — плашка
// цвета стадии, будущие «○» faint. Не влезает — окно вокруг текущей с «…».
// Без стадий — плашка статуса.
func (m Model) stageTrackLine(w int) string {
	t := m.taskCard
	st := m.api.StagesOf(t.GroupID)
	if len(st) == 0 {
		return pill(statusName(t.Status), cInk, cAccent)
	}
	cur := 0
	for i, s := range st {
		if s.ID == t.StageID {
			cur = i
			break
		}
	}
	seg := func(lo, hi int) string {
		var parts []string
		if lo > 0 {
			parts = append(parts, stFaint.Render("…"))
		}
		for i := lo; i <= hi; i++ {
			if i > lo {
				parts = append(parts, stFaint.Render("─"))
			}
			switch {
			case i < cur:
				parts = append(parts, stDim.Render("● "+st[i].Title))
			case i == cur:
				parts = append(parts, pill(st[i].Title, cInk, stageColor(st[i].Color)))
			default:
				parts = append(parts, stFaint.Render("○ "+st[i].Title))
			}
		}
		if hi < len(st)-1 {
			parts = append(parts, stFaint.Render("…"))
		}
		return strings.Join(parts, " ")
	}
	lo, hi := 0, len(st)-1
	out := seg(lo, hi)
	for lipgloss.Width(out) > w && hi > lo {
		if cur-lo >= hi-cur {
			lo++
		} else {
			hi--
		}
		out = seg(lo, hi)
	}
	return truncate(out, w)
}

// cardMetaLine — срок (как на доске), автор, бейдж новых комментариев.
func (m Model) cardMetaLine() string {
	t := m.taskCard
	var parts []string
	if d, ok := ParseDeadline(t.Deadline, m.srvLoc()); ok {
		now := m.api.now().In(m.srvLoc())
		day := d.In(m.srvLoc()).Format("2006-01-02")
		switch {
		case d.Before(now): // сегодняшний срок, прошедший по времени, — тоже просрочка
			label := dueLabel(d, now)
			if day == now.Format("2006-01-02") {
				label = d.Format("15:04")
			}
			parts = append(parts, pill("просрочено "+label, cInk, cUrgent))
		case day == now.Format("2006-01-02"):
			parts = append(parts, pill(dueLabel(d, now), cInk, pulseColor(m.frame)))
		default:
			parts = append(parts, chip(dueLabel(d, now)))
		}
	} else {
		parts = append(parts, stFaint.Render("без срока"))
	}
	if t.CreatorName != "" {
		parts = append(parts, stDim.Render("от "+t.CreatorName))
	}
	if t.NewComments > 0 {
		parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(pulseColor(m.frame)).
			Render("✎ "+fmt.Sprint(t.NewComments)))
	}
	return strings.Join(parts, "  ")
}

// taskChatPane — правая панель экрана задачи: чат задачи или заглушка.
// Чужой активный диалог (открыли на вкладке чатов) не рендерим — только
// подсказка, как вернуть чат задачи.
func (m Model) taskChatPane() string {
	if m.taskCard.ChatID <= 0 {
		return box(paneTitle("", "чат задачи", "", false),
			[]string{"", stDim.Render("у задачи нет чата")}, m.dialW, false)
	}
	if m.active == nil || m.active.DialogID != taskChatDialog(m.taskCard.ChatID) {
		return box(paneTitle("", "чат задачи", "", false),
			[]string{"", stDim.Render("c — открыть чат задачи")}, m.dialW, false)
	}
	return m.dialogPane()
}

func (m Model) statusMenu() string {
	var rows []string
	if st := m.api.StagesOf(m.taskCard.GroupID); len(st) > 0 {
		// меню стадий проекта: текущая помечена «●»
		for i, s := range st {
			mark := "○ "
			if s.ID == m.taskCard.StageID {
				mark = "● "
			}
			line := " " + mark + s.Title
			if i == m.statusIdx {
				line = stCursor.Render("▌") + " " + stTitle.Render(mark+s.Title)
			}
			rows = append(rows, line)
		}
	} else {
		opts := statusOptions(m.taskCard.Status)
		for i, o := range opts {
			line := " " + o.name
			if i == m.statusIdx {
				line = stCursor.Render("▌") + " " + stTitle.Render(o.name)
			}
			rows = append(rows, line)
		}
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

// notifyBadge — счётчик вкладки уведов: серверный (total_unread_count), когда
// данные уже приходили с портала; иначе локальный по списку (тесты).
func (m Model) notifyBadge() int {
	if !m.api.notifyAt.IsZero() {
		return m.api.NotifyUnread()
	}
	return countUnread(m.notify)
}

// notifyWide — широкий экран уведов: слева список, справа превью.
func (m Model) notifyWide() bool { return m.width >= 100 }

// notifyListW — внутренняя ширина списка уведов (широкий экран — левая панель).
func (m Model) notifyListW() int {
	if m.notifyWide() {
		return (m.width-1)*11/20 - 4
	}
	return m.width - 4
}

// notifyPreviewW — внутренняя ширина панели превью (широкий экран).
func (m Model) notifyPreviewW() int {
	left := (m.width - 1) * 11 / 20
	return m.width - left - 1 - 4
}

// notifyGlyph — значок модуля уведа.
func notifyGlyph(n NotifyItem) string {
	switch n.Module {
	case "tasks":
		return "󰄬"
	case "calendar":
		return "󰃭"
	case "disk":
		return "󰋊"
	}
	return "󰅎"
}

// notifyName — кто в строке списка: автор, затем заголовок, затем текст.
func notifyName(n NotifyItem) string {
	switch {
	case n.Author != "":
		return n.Author
	case n.Title != "":
		return n.Title
	default:
		return n.Text
	}
}

func (m Model) viewNotify() string {
	rows := m.visibleNotify()
	start, end := m.notifyWindow(len(rows))
	out := make([]string, 0, (end-start)*2+3)
	if m.filterRows() > 0 {
		out = append(out, m.filterLine()...)
	}
	if len(rows) == 0 {
		out = append(out, stDim.Render(" — уведомлений нет — (R — обновить) "))
	}
	for i := start; i < end; i++ {
		out = append(out, m.notifyRows(rows[i], i == m.cursor)...)
	}
	if m.notifyLoading {
		out = append(out, stDim.Render("  загрузка…"))
	}
	if m.notifyWide() {
		// панель полной высоты, как карточка на экране задачи — иначе рамки
		// превью и списка разъезжаются; на узком экране — как раньше
		for len(out) < max(m.bodyH-2, 4) {
			out = append(out, "")
		}
	}
	title := paneTitle("", "уведы", unreadExtra(m.notifyBadge()), m.tab == tabNotify)
	list := box(title, out, m.notifyListW(), m.tab == tabNotify)
	if !m.notifyWide() {
		return list
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", m.notifyPreview(m.notifyPreviewW()))
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

// notifyRows — две строки на увед: автор (или заголовок/текст) + время;
// вторая строка — текст (или заголовок, если он не ушёл в первую). Выбранный
// увед — имя акцентом, непрочитанный — цветом и точкой перед временем.
func (m Model) notifyRows(n NotifyItem, selected bool) []string {
	inner := m.notifyListW()
	name := notifyName(n)
	nameSt := lipgloss.NewStyle()
	if n.Unread {
		nameSt = stChatUnread
	}
	if selected {
		nameSt = stTitle
	}
	t := nameSt.Render(truncate(name, max(inner-12, 1)))
	marker := " "
	if selected {
		marker = stCursor.Render("▌")
	}
	right := ""
	if n.Unread {
		right += stBadge.Render("•") + " "
	}
	right += stDim.Render(n.Time)
	gap := inner - 3 - lipgloss.Width(t) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line1 := marker + notifyGlyph(n) + " " + t + strings.Repeat(" ", gap) + right
	body := n.Text
	if n.Title != "" && n.Title != name {
		body = n.Title
	}
	line2 := ""
	if body != "" && body != name {
		// одна строка: переводы строк текста — в пробел (высота ряда = 2)
		line2 = stDim.Render("   " + truncate(strings.ReplaceAll(body, "\n", " "), max(inner-4, 1)))
	}
	return []string{line1, line2}
}

// notifyPreview — правая панель уведов на широком экране: полный текст без
// обрезки, полная дата и подсказки клавиш. Высота — как у списка (тот же
// window math), короткий контент добивается пустыми строками.
func (m Model) notifyPreview(w int) string {
	rows := m.visibleNotify()
	contentH := max(m.bodyH-2, 4)
	pad := func(lines []string) []string {
		for len(lines) < contentH {
			lines = append(lines, "")
		}
		if len(lines) > contentH {
			lines = lines[:contentH]
		}
		return lines
	}
	title := paneTitle("", "увед", "", false)
	if m.cursor < 0 || m.cursor >= len(rows) {
		return box(title, pad([]string{"", stDim.Render("—")}), w, false)
	}
	n := rows[m.cursor]
	label := n.Module
	switch n.Module {
	case "tasks":
		label = "задача"
	case "calendar":
		label = "календарь"
	}
	lines := []string{
		stTitle.Render(notifyGlyph(n) + " " + truncate(notifyName(n), max(w-2, 1))),
		stDim.Render(truncate(n.Date+" · "+label, max(w-2, 1))),
		"",
	}
	if n.Title != "" {
		lines = append(lines, stTitle.Render(truncate(n.Title, max(w-2, 1))))
	}
	hints := []string{stDim.Render("u — " + hintNotifyRead(n.Unread))}
	if n.TaskID > 0 {
		hints = append([]string{stDim.Render("enter — открыть задачу")}, hints...)
	} else if n.URL != "" {
		hints = append([]string{stDim.Render("enter — открыть в браузере")}, hints...)
	}
	// текст: перенос по словам, окно по высоте; обрезано — «…» в конце
	if avail := contentH - len(lines) - len(hints) - 1; avail > 0 && n.Text != "" {
		wrapped := strings.Split(wordwrap.String(n.Text, max(w-2, 1)), "\n")
		if len(wrapped) > avail {
			wrapped = wrapped[:avail]
			last := strings.TrimRight(wrapped[len(wrapped)-1], " ")
			wrapped[len(wrapped)-1] = truncate(last+" …", max(w-2, 1))
		}
		lines = append(lines, wrapped...)
	}
	lines = append(lines, "")
	lines = append(lines, hints...)
	return box(title, pad(lines), w, false)
}

// hintNotifyRead — подпись клавиши u для текущего состояния.
func hintNotifyRead(unread bool) string {
	if unread {
		return "отметить прочитанным"
	}
	return "отметить непрочитанным"
}

// ---- help

func (m Model) viewHelp() string {
	h := m.help
	// оверлей не в коробке: поля по 1 с боков (колонка уведов с «u» впритык)
	h.SetWidth(m.width - 2) // узкий терминал: колонки обрежутся с «…»
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
