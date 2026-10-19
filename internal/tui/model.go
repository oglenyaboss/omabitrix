package tui

import (
	"context"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/wordwrap"
)

type tab int

const (
	tabChats tab = iota
	tabTasks
	tabNotify
	tabCount
)

type focus int

const (
	focusList focus = iota
	focusReply
	focusFilter
)

type overlay int

const (
	ovNone overlay = iota
	ovHelp
	ovStatus
	ovDeadline
)

// Model — корневая модель: одна Update, экраны — рендер-функции.
type Model struct {
	api *API
	ctx context.Context

	tab     tab
	overlay overlay

	chats   []Chat
	tasks   []Task
	buckets TaskBuckets
	notify  []NotifyItem

	cursor   int // список чатов/задач/уведов (по вкладке)
	filter   string
	filterIn textinput.Model
	focus    focus
	help     help.Model

	active         *Chat
	history        []Message
	noMore         bool
	historyLoading bool // первый запрос истории в полёте — в панели «загрузка…»
	vp             viewport.Model
	reply          textarea.Model
	sending        bool
	loadErr        string

	taskIdx     int // выбранная задача в плоском списке
	taskCard    Task
	taskOpen    bool
	cardReq     int64 // последний запрошенный id — защита от гонки ответов
	cardLoading bool  // карточка ещё грузится — в панели «загрузка…»
	followSeq   int   // тик «карточка за курсором»: живёт только последний
	statusIdx   int
	deadlineIdx int

	drafts map[string]string // черновики по диалогам

	listW, dialW, cardW, bodyH int

	width, height int
	spinner       spinner.Model
	ready         bool
	flash         string
	flashAt       time.Time

	pendingChat string
	pendingTask int64
}

// OpenChat — открыть диалог после загрузки (--chat).
func (m *Model) OpenChat(dialogID string) { m.pendingChat = dialogID }

// OpenTask — открыть карточку после загрузки (--task).
func (m *Model) OpenTask(id int64) { m.pendingTask = id }

func NewModel(api *API, ctx context.Context) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.Points))
	ta := textarea.New()
	ta.Placeholder = "ответить… enter — отправить, esc — отмена"
	ta.CharLimit = 2000
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false) // enter = отправка
	ti := textinput.New()
	ti.Placeholder = "фильтр чатов…"
	ti.CharLimit = 60
	h := help.New()
	h.ShortSeparator = " · "
	h.FullSeparator = "  "
	h.Styles = help.Styles{
		ShortKey:       stHelpKey,
		ShortDesc:      stHelpDesc,
		ShortSeparator: stDim,
		FullKey:        stHelpKey,
		FullDesc:       stHelpDesc,
		FullSeparator:  stDim,
		Ellipsis:       stDim,
	}
	return Model{
		api: api, ctx: ctx,
		spinner: sp, reply: ta, filterIn: ti, help: h,
		drafts: map[string]string{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.reloadAllCmd(), tick10s())
}

func tick10s() tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

type tickMsg struct{}

type tickReload struct{} // пришло из подгрузки

// loadCmd — одна ветка данных, приходят независимо: висящий запрос
// не держит остальные.
func (m Model) loadCmd(what string) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg {
		switch what {
		case "chats":
			_, err := api.Chats(ctx, true)
			return reloadMsg{chats: true, errs: errStr(err)}
		case "tasks":
			_, err := api.OpenTasks(ctx, true)
			return reloadMsg{tasks: true, errs: errStr(err)}
		default:
			_, err := api.Notify(ctx, true)
			return reloadMsg{notify: true, errs: errStr(err)}
		}
	}
}

// reloadAllCmd — перечитать все три списка; один кодовый путь для Init, R и тика.
func (m Model) reloadAllCmd() tea.Cmd {
	return tea.Batch(m.loadCmd("chats"), m.loadCmd("tasks"), m.loadCmd("notify"))
}

func errStr(err error) []string {
	if err == nil {
		return nil
	}
	return []string{err.Error()}
}

type reloadMsg struct {
	chats, tasks, notify bool
	errs                 []string
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tickMsg:
		// активность мимо TUI (ответ Анны, отправка из веба) тоже должна
		// обновлять списки — иначе они висят с момента старта/последней отправки
		if m.active != nil {
			return m, tea.Batch(m.refreshActiveCmd(), m.reloadAllCmd(), tick10s())
		}
		return m, tea.Batch(m.reloadAllCmd(), tick10s())

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case reloadMsg:
		m.ready = true
		key := m.cursorKey() // до замены списков: курсор вернём на тот же элемент
		cardKey := ""        // задача открытой карточки — taskIdx следует за курсором
		if m.tab == tabTasks {
			if rows := m.flatTasks(); m.taskIdx >= 0 && m.taskIdx < len(rows) {
				cardKey = strconv.FormatInt(rows[m.taskIdx].task.ID, 10)
			}
		}
		if msg.chats {
			m.chats = m.api.snapshotChats()
		}
		if msg.tasks {
			m.tasks = m.api.snapshotTasks()
			m.buckets = Buckets(m.tasks, m.api.C.ServerLoc(), m.api.now())
		}
		if msg.notify {
			m.notify = m.api.snapshotNotify()
		}
		if m.active != nil {
			for i := range m.chats {
				if m.chats[i].DialogID == m.active.DialogID {
					m.active = &m.chats[i]
					break
				}
			}
		}
		if len(msg.errs) > 0 {
			m.loadErr = strings.Join(msg.errs, "; ")
		} else {
			m.loadErr = ""
		}
		m.restoreCursor(key)
		if key != "" && key == cardKey {
			m.taskIdx = m.cursor
		}
		m.clampCursors()
		m.layout()
		if m.pendingChat != "" {
			id := m.pendingChat
			m.pendingChat = ""
			m.openChatByDialog(id)
			return m, m.historyCmd(id, 0)
		}
		if m.pendingTask != 0 {
			id := m.pendingTask
			m.pendingTask = 0
			// deep-link: встать курсором на задачу, если она в списке открытых;
			// закрытая всё равно откроется карточкой
			for i, r := range m.flatTasks() {
				if r.task.ID == id {
					m.cursor, m.taskIdx = i, i
					break
				}
			}
			return m, m.requestCard(id)
		}
		return m, nil

	case historyMsg:
		m.historyLoading = false
		if msg.err != nil {
			m.loadErr = msg.err.Error()
			return m, nil
		}
		m.noMore = msg.noMore
		var read tea.Cmd
		if m.active != nil && msg.dialogID == m.active.DialogID {
			if msg.prepend {
				// догрузка старых: держим позицию (YOffset += прирост)
				old := m.vp.TotalLineCount()
				atBottom := m.vp.AtBottom() || old == 0
				m.history = append(msg.rows, m.history...)
				m.renderHistory()
				if !atBottom {
					m.vp.SetYOffset(m.vp.TotalLineCount() - old)
				}
			} else {
				// тик-обновление не должно вырывать юзера из истории: к низу —
				// только если он и был у низа
				atBottom := m.vp.AtBottom() || len(m.history) == 0
				m.history = mergeHistory(m.history, msg.rows)
				m.renderHistory()
				if atBottom {
					m.vp.GotoBottom()
				}
				// открыл чат — прочитал: гасим счётчик локально и на портале
				if len(msg.rows) > 0 && m.active.Counter > 0 {
					m.active.Counter = 0
					for i := range m.chats {
						if m.chats[i].DialogID == msg.dialogID {
							m.chats[i].Counter = 0
							break
						}
					}
					read = m.readDialogCmd(msg.dialogID, msg.rows[len(msg.rows)-1].ID)
				}
			}
		}
		return m, read

	case sentMsg:
		m.sending = false
		same := m.active != nil && m.active.DialogID == msg.dialogID
		if msg.err != nil {
			m.loadErr = "не отправлено: " + msg.err.Error()
			if same {
				// текст вернули в поле; допечатанное за время отправки не трогаем
				if m.reply.Value() == "" {
					m.reply.SetValue(msg.text)
				} else {
					m.reply.SetValue(msg.text + " " + m.reply.Value())
				}
				m.replyActive()
			} else {
				// юзер уже в другом чате — неотправленное ждёт черновиком
				if d := m.drafts[msg.dialogID]; d == "" {
					m.drafts[msg.dialogID] = msg.text
				} else {
					m.drafts[msg.dialogID] = msg.text + " " + d
				}
			}
			return m, nil
		}
		if same {
			// поле чистое с момента отправки — черновик больше не нужен
			if m.reply.Value() == "" {
				delete(m.drafts, msg.dialogID)
			}
			now := m.api.now()
			key, label := DayInfo(now.Format(time.RFC3339), now)
			m.history = append(m.history, Message{
				ID: msg.id, Text: msg.text,
				Time: now.Local().Format("15:04"), DayKey: key, DayLabel: label,
				Mine: true,
			})
			m.renderHistory()
			m.vp.GotoBottom()
		}
		m.api.chatsAt = time.Time{} // превью списка обновится при следующем Chats()
		return m, m.loadCmd("chats")

	case statusMsg:
		if msg.err != nil {
			m.loadErr = "статус не изменён: " + msg.err.Error()
			return m, nil
		}
		m.overlay = ovNone
		m.flash = "✓ статус обновлён"
		m.flashAt = m.api.now()
		if m.taskOpen {
			return m, m.loadTaskCardCmd(m.taskCard.ID)
		}
		return m, m.loadCmd("tasks")

	case deadlineMsg:
		if msg.err != nil {
			m.loadErr = "срок не изменён: " + msg.err.Error()
			return m, nil
		}
		m.overlay = ovNone
		m.flash = "✓ срок обновлён"
		m.flashAt = m.api.now()
		if m.taskOpen {
			return m, tea.Batch(m.loadTaskCardCmd(m.taskCard.ID), m.loadCmd("tasks"))
		}
		return m, m.loadCmd("tasks")

	case taskCardMsg:
		if msg.requested != m.cardReq {
			return m, nil // устарел: пока грузили, выбрали другую задачу
		}
		m.cardLoading = false
		if msg.err != nil {
			m.loadErr = msg.err.Error()
			return m, nil
		}
		m.taskCard = msg.task
		m.taskOpen = true
		m.tab = tabTasks
		m.cursor = m.taskIdx
		return m, nil

	case cardFollowMsg:
		// карточка догоняет курсор: живёт только последний тик и только
		// если курсор за задержку не ушёл с задачи
		if msg.seq != m.followSeq || !m.taskOpen || m.tab != tabTasks {
			return m, nil
		}
		rows := m.flatTasks()
		if m.cursor >= len(rows) || rows[m.cursor].task.ID != msg.id {
			return m, nil
		}
		m.taskIdx = m.cursor
		return m, m.requestCard(msg.id)

	case tea.MouseClickMsg:
		return m.mouseClick(msg)

	case tea.MouseWheelMsg:
		return m.mouseWheel(msg)

	case readAllMsg:
		if msg.err != nil {
			m.loadErr = "не прочитано: " + msg.err.Error()
			return m, nil
		}
		m.notify = m.api.snapshotNotify()
		m.flash = "✓ всё прочитано"
		m.flashAt = m.api.now()
		return m, nil

	case readMsg:
		if msg.err != nil {
			m.loadErr = "не прочитано: " + msg.err.Error()
		}
		return m, nil

	case browserMsg:
		if msg.err != nil {
			m.loadErr = "браузер: " + msg.err.Error()
			return m, nil
		}
		m.flash = "открыто в браузере"
		m.flashAt = m.api.now()
		return m, nil

	case tea.KeyPressMsg:
		if !m.ready {
			// «любая клавиша — пропустить»: входим с тем, что загрузилось,
			// клавиша при этом применяется (не теряется)
			m.ready = true
			m.layout()
			return m.keys(msg)
		}
		return m.keys(msg)
	}
	return m, nil
}

// ---- служебное

// layout: сетка экрана. bodyH — высота контента под хедером/футером.
// Рамки добавляют по 2: width - 2 = контент.
func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.listW = m.width / 3
	if m.listW > 46 {
		m.listW = 46
	}
	if m.listW < 12 {
		m.listW = 12
	}
	m.dialW = m.width - (m.listW + 4) - 2 - 4
	if m.dialW < 20 {
		m.dialW = 20
	}
	m.cardW = m.width - (m.listW + 4) - 2 - 4
	if m.cardW < 20 {
		m.cardW = 20
	}
	m.bodyH = m.height - 1
	if m.bodyH < 6 {
		m.bodyH = 6
	}
	// единственная точка правды ширины истории: минус колонка скроллбара
	m.vp.SetWidth(m.dialW - 2)
	replyRows := 2 // пустая + подсказка «r — ответить»
	if m.focus == focusReply {
		replyRows = 3
	}
	m.vp.SetHeight(m.bodyH - 2 - replyRows)
	if m.vp.Height() < 2 {
		m.vp.SetHeight(2)
	}
	m.reply.SetWidth(m.dialW - 4)
	m.reply.SetHeight(3)
	if m.focus == focusFilter {
		m.filterIn.SetWidth(m.listW - 6)
	}
	m.renderHistory()
}

func (m *Model) clampCursors() {
	n := m.visibleCount()
	if n == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// cursorKey — стабильный ключ элемента под курсором: индексы плывут при
// пересортировке (непрочитанные вверх), ключ переживает перечитку.
func (m Model) cursorKey() string {
	switch m.tab {
	case tabChats:
		vc := m.visibleChats()
		if m.cursor < 0 || m.cursor >= len(vc) {
			return ""
		}
		return vc[m.cursor].DialogID
	case tabTasks:
		rows := m.flatTasks()
		if m.cursor < 0 || m.cursor >= len(rows) {
			return ""
		}
		return strconv.FormatInt(rows[m.cursor].task.ID, 10)
	default:
		vn := m.visibleNotify()
		if m.cursor < 0 || m.cursor >= len(vn) {
			return ""
		}
		return vn[m.cursor].ID
	}
}

// restoreCursor — курсор на тот же элемент в перечитанном списке; не нашли —
// ничего, дальше подрежет clampCursors. Заголовков корзин в flatTasks нет
// (они только в lineMap рендера), курсор всегда стоит на задаче.
func (m *Model) restoreCursor(key string) {
	if key == "" {
		return
	}
	switch m.tab {
	case tabChats:
		for i, c := range m.visibleChats() {
			if c.DialogID == key {
				m.cursor = i
				return
			}
		}
	case tabTasks:
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return
		}
		rows := m.flatTasks()
		for i, r := range rows {
			if r.task.ID == id {
				m.cursor = i
				return
			}
		}
	default:
		for i, n := range m.visibleNotify() {
			if n.ID == key {
				m.cursor = i
				return
			}
		}
	}
}

func (m *Model) replyActive() {
	m.focus = focusReply
	m.layout()
}

// ---- команды

type historyMsg struct {
	dialogID string
	rows     []Message
	noMore   bool
	prepend  bool
	err      error
}

func (m Model) historyCmd(dialogID string, lastID int64) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg {
		rows, noMore, err := api.History(ctx, dialogID, lastID)
		return historyMsg{dialogID: dialogID, rows: rows, noMore: noMore,
			prepend: lastID > 0, err: err}
	}
}

func (m Model) refreshActiveCmd() tea.Cmd {
	if m.active == nil {
		return nil
	}
	return m.historyCmd(m.active.DialogID, 0)
}

type sentMsg struct {
	dialogID string
	id       int64
	text     string
	err      error
}

// sendCmd — текст приходит параметром: поле уже очищено при отправке.
func (m Model) sendCmd(text string) tea.Cmd {
	api, ctx := m.api, m.ctx
	dialogID := m.active.DialogID
	return func() tea.Msg {
		id, err := api.Send(ctx, dialogID, text)
		return sentMsg{dialogID: dialogID, id: id, text: text, err: err}
	}
}

type statusMsg struct{ err error }

func (m Model) statusCmd(id int64, status string) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg {
		return statusMsg{err: api.SetStatus(ctx, id, status)}
	}
}

type deadlineMsg struct{ err error }

func (m Model) deadlineCmd(id int64, dl string) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg { return deadlineMsg{err: api.SetDeadline(ctx, id, dl)} }
}

// dlOpt — пункт меню сроков.
type dlOpt struct{ label, val string }

// deadlineOptions: сегодня/завтра — всегда 18:00 в локали юзера;
// сдвиги — от текущего дедлайна с его временем (или 18:00).
func deadlineOptions(now time.Time, cur string) []dlOpt {
	at1800 := func(day time.Time) string {
		d := time.Date(day.Year(), day.Month(), day.Day(), 18, 0, 0, 0, time.Local)
		return d.Format(time.RFC3339)
	}
	base, hasBase := ParseDeadline(cur, time.Local)
	if !hasBase {
		base = now
	}
	shift := func(days int) string {
		d := base.AddDate(0, 0, days)
		if hasBase {
			return d.Format(time.RFC3339)
		}
		return at1800(d)
	}
	return []dlOpt{
		{"сегодня 18:00", at1800(now)},
		{"завтра 18:00", at1800(now.AddDate(0, 0, 1))},
		{"+1 день", shift(1)},
		{"+1 неделя", shift(7)},
		{"снять дедлайн", ""},
	}
}

type taskCardMsg struct {
	requested int64
	task      Task
	err       error
}

// requestCard — запрос карточки: защита от гонки и индикатор загрузки вместе.
func (m *Model) requestCard(id int64) tea.Cmd {
	m.cardReq = id
	m.cardLoading = true
	return m.loadTaskCardCmd(id)
}

func (m Model) loadTaskCardCmd(id int64) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg {
		t, err := api.Task(ctx, id)
		return taskCardMsg{requested: id, task: t, err: err}
	}
}

type cardFollowMsg struct {
	seq int
	id  int64
}

// followCard — карточка следует за курсором с задержкой 250 мс: при быстром
// листании не копится очередь запросов (портал троттлит 500 мс).
func (m *Model) followCard() tea.Cmd {
	if m.tab != tabTasks || !m.taskOpen {
		return nil
	}
	rows := m.flatTasks()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return nil
	}
	id := rows[m.cursor].task.ID
	if id == m.taskCard.ID && !m.cardLoading {
		return nil
	}
	m.followSeq++
	seq := m.followSeq
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return cardFollowMsg{seq: seq, id: id}
	})
}

type readAllMsg struct{ err error }

func (m Model) readAllCmd() tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg { return readAllMsg{err: api.ReadAllNotify(ctx)} }
}

// readMsg — ответ ReadDialog/ReadNotify: при ошибке показываем loadErr.
type readMsg struct{ err error }

func (m Model) readDialogCmd(dialogID string, lastID int64) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg { return readMsg{err: api.ReadDialog(ctx, dialogID, lastID)} }
}

func (m Model) readNotifyCmd(id string) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg { return readMsg{err: api.ReadNotify(ctx, id)} }
}

// mouseClick — клик открывает элемент списка (колесо — отдельное событие).
func (m Model) mouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.focus != focusList || m.overlay != ovNone {
		return m, nil
	}
	if msg.Button == tea.MouseLeft {
		return m.mouseLeft(msg)
	}
	return m, nil
}

// wheel: над диалогом — скролл истории, над списком — курсор.
func (m Model) mouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	up := msg.Button == tea.MouseWheelUp
	if m.active != nil && msg.X > m.listW+4 {
		if up {
			m.vp.ScrollUp(3)
		} else {
			m.vp.ScrollDown(3)
		}
		return m, nil
	}
	if up {
		if m.cursor > 0 {
			m.cursor--
			return m, m.followCard()
		}
		return m, nil
	}
	if m.cursor < m.visibleCount()-1 {
		m.cursor++
		return m, m.followCard()
	}
	return m, nil
}

// mouseLeft: геометрия строк синхронна с chatWindow/taskWindow/notifyWindow.
func (m Model) mouseLeft(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	switch m.tab {
	case tabChats:
		if msg.X > m.listW+4 {
			return m, nil
		}
		vc := m.visibleChats()
		start, _, filterRows := m.chatWindow(vc)
		idx := (msg.Y - 1 - filterRows) / 2 // строка 0 — верхняя грань рамки
		if idx < 0 || start+idx >= len(vc) {
			return m, nil
		}
		m.cursor = start + idx
		if m.active == nil || m.active.DialogID != vc[m.cursor].DialogID {
			m.openDialog(m.cursor)
			if m.active != nil {
				return m, m.historyCmd(m.active.DialogID, 0)
			}
		}
	case tabTasks:
		// гейт как в чатах: клик по карточке — не список
		if m.taskOpen && msg.X > m.listW+4 {
			return m, nil
		}
		rows := m.flatTasks()
		_, _, lineMap := m.taskWindow(rows)
		idx := msg.Y - 1 - m.filterRows()
		if idx < 0 || idx >= len(lineMap) || lineMap[idx] < 0 {
			return m, nil
		}
		m.cursor = lineMap[idx]
		return m.enterKey()
	case tabNotify:
		vn := m.visibleNotify()
		start, _ := m.notifyWindow(len(vn))
		idx := (msg.Y - 1 - m.filterRows()) / 2
		if idx < 0 || start+idx >= len(vn) {
			return m, nil
		}
		m.cursor = start + idx
		return m.enterKey()
	}
	return m, nil
}

// ---- данные для рендера

func (m *Model) visibleChats() []Chat {
	if m.filter == "" {
		return m.chats
	}
	f := strings.ToLower(m.filter)
	out := make([]Chat, 0, len(m.chats))
	for _, c := range m.chats {
		if strings.Contains(strings.ToLower(c.Title), f) ||
			strings.Contains(strings.ToLower(c.Preview), f) {
			out = append(out, c)
		}
	}
	return out
}

func (m *Model) visibleCount() int {
	switch m.tab {
	case tabChats:
		return len(m.visibleChats())
	case tabTasks:
		return len(m.flatTasks())
	default:
		return len(m.visibleNotify())
	}
}

type taskRow struct {
	task Task
	kind string // overdue/today/later/nodate/help/watch
}

func (m *Model) flatTasks() []taskRow {
	out := make([]taskRow, 0, len(m.tasks))
	for _, t := range m.buckets.Overdue {
		out = append(out, taskRow{t, "overdue"})
	}
	for _, t := range m.buckets.Today {
		out = append(out, taskRow{t, "today"})
	}
	for _, t := range m.buckets.Later {
		out = append(out, taskRow{t, "later"})
	}
	for _, t := range m.buckets.NoDate {
		out = append(out, taskRow{t, "nodate"})
	}
	for _, t := range m.buckets.Help {
		out = append(out, taskRow{t, "help"})
	}
	for _, t := range m.buckets.Watch {
		out = append(out, taskRow{t, "watch"})
	}
	if m.tab == tabTasks && m.filter != "" {
		// фильтр по титулу как в чатах; пустые корзины просто исчезают —
		// заголовок секции рисуется только перед её задачами
		f := strings.ToLower(m.filter)
		kept := make([]taskRow, 0, len(out))
		for _, r := range out {
			if strings.Contains(strings.ToLower(r.task.Title), f) {
				kept = append(kept, r)
			}
		}
		out = kept
	}
	return out
}

// visibleNotify — видимый список уведов: фильтр по Title/Text без учёта
// регистра, только на вкладке уведов (как фильтр чатов).
func (m *Model) visibleNotify() []NotifyItem {
	if m.tab != tabNotify || m.filter == "" {
		return m.notify
	}
	f := strings.ToLower(m.filter)
	out := make([]NotifyItem, 0, len(m.notify))
	for _, n := range m.notify {
		if strings.Contains(strings.ToLower(n.Title), f) ||
			strings.Contains(strings.ToLower(n.Text), f) {
			out = append(out, n)
		}
	}
	return out
}

// clearNotifyUnread — погасить Unread в полном списке по ID: индекс
// отфильтрованного списка не совпадает с исходным.
func (m *Model) clearNotifyUnread(id string) {
	for i := range m.notify {
		if m.notify[i].ID == id {
			m.notify[i].Unread = false
			return
		}
	}
}

// cardMatchesCursor — s/d безопасны только над карточкой той задачи,
// что стоит под курсором (иначе гонка «курсор ушёл, карточка старая»).
func (m Model) cardMatchesCursor() bool {
	if m.taskCard.ID == 0 {
		return false
	}
	rows := m.flatTasks()
	return m.cursor < len(rows) && rows[m.cursor].task.ID == m.taskCard.ID
}

// openChatByDialog — вход из --chat (тосты/бар) и из карточки задачи («c»).
func (m *Model) openChatByDialog(id string) {
	for i := range m.chats {
		if m.chats[i].DialogID == id {
			m.tab = tabChats
			m.openDialog(i)
			return
		}
	}
	// чат задачи не в списке (отфильтрован) — synthetic
	m.tab = tabChats
	m.active = &Chat{DialogID: id, Title: "чат " + id}
	m.reply.SetValue(m.drafts[id])
	m.openHistory()
}

func (m *Model) openDialog(idx int) {
	vc := m.visibleChats()
	if idx < 0 || idx >= len(vc) {
		return
	}
	m.active = &vc[idx]
	m.cursor = idx
	m.reply.SetValue(m.drafts[m.active.DialogID])
	m.openHistory()
}

// closeDialog — выход из чата: с карточки задачи возвращаемся на неё.
func (m *Model) closeDialog() {
	if m.active != nil && m.reply.Value() != "" {
		m.drafts[m.active.DialogID] = m.reply.Value()
	}
	m.active = nil
	if m.taskOpen {
		m.tab = tabTasks
	}
}

func (m *Model) openHistory() {
	m.history = nil
	m.noMore = false
	m.historyLoading = true
	m.renderHistory()
}

// mergeHistory — тик-обновление не должно стирать догруженное по H старьё:
// всё старее первой строки свежей страницы берём из текущего списка.
func mergeHistory(old, fresh []Message) []Message {
	if len(fresh) == 0 {
		return old
	}
	first := fresh[0].ID
	var tail []Message
	for _, msg := range old {
		if msg.ID < first {
			tail = append(tail, msg)
		}
	}
	out := make([]Message, 0, len(fresh)+len(tail))
	out = append(out, tail...)
	out = append(out, fresh...)
	return out
}

// renderHistory — tele-стиль: подряд идущие сообщения одного автора
// группируются (имя один раз), mine — маркером без имени, даты по центру.
// потолок: prepend-догрузка может слить две серии одного автора на границе —
// сдвиг скролла на 1–2 строки, приемлемо.
func (m *Model) renderHistory() {
	if m.vp.Width() <= 0 {
		return
	}
	W := m.vp.Width() - 1
	if W < 10 {
		W = 10
	}
	if m.historyLoading && len(m.history) == 0 {
		m.vp.SetContent(centered(stDim.Render("загрузка…"), W))
		return
	}
	type hgroup struct {
		author       int64
		name         string
		mine, system bool
		msgs         []Message
	}
	var groups []hgroup
	for _, msg := range m.history {
		if msg.System {
			groups = append(groups, hgroup{system: true, msgs: []Message{msg}})
			continue
		}
		if n := len(groups); n > 0 && !groups[n-1].system &&
			groups[n-1].mine == msg.Mine && groups[n-1].author == msg.AuthorID {
			groups[n-1].msgs = append(groups[n-1].msgs, msg)
			continue
		}
		groups = append(groups, hgroup{author: msg.AuthorID, name: msg.Name,
			mine: msg.Mine, msgs: []Message{msg}})
	}
	var b strings.Builder
	var prevDay string
	for gi, g := range groups {
		first := g.msgs[0]
		if first.DayKey != prevDay && first.DayLabel != "" {
			if gi > 0 {
				b.WriteString("\n")
			}
			b.WriteString(dayDivider(stDim.Render(first.DayLabel), W) + "\n")
			prevDay = first.DayKey
		} else if gi > 0 {
			b.WriteString("\n")
		}
		switch {
		case g.system:
			for _, msg := range g.msgs {
				b.WriteString(centered(stDim.Render("· "+msg.Text+" ·"), W) + "\n")
			}
		case g.mine:
			for i, msg := range g.msgs {
				if i == 0 {
					line := stAccent.Render("›") + stDim.Render(" "+msg.Time+" ") +
						stBubbleMine.Render(msg.Text)
					b.WriteString(indentWrap(line, W, 2) + "\n")
					continue
				}
				b.WriteString(indentWrap(stBubbleMine.Render(msg.Text), W, 2) + "\n")
			}
		default:
			name := g.name
			if name == "" {
				name = "?"
			}
			b.WriteString(authorStyle(g.author).Render(name) +
				stDim.Render(" "+first.Time) + "\n")
			for _, msg := range g.msgs {
				b.WriteString(indentWrap(msg.Text, W, 2) + "\n")
			}
		}
	}
	out := b.String()
	if m.noMore && len(m.history) > 0 {
		// выше всех строк — граница истории
		out = centered(stDim.Render("— начало переписки —"), W) + "\n" + out
	}
	m.vp.SetContent(out)
}

func centered(s string, w int) string {
	pad := w - lipgloss.Width(s)
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad/2) + s
}

// dayDivider — «── сегодня ──» по центру; лейбл приходит стилизованным.
func dayDivider(label string, w int) string {
	fill := w - lipgloss.Width(label) - 2
	if fill < 2 {
		return label
	}
	left := fill / 2
	return stDim.Render(strings.Repeat("─", left)+" ") + label +
		stDim.Render(" "+strings.Repeat("─", fill-left))
}

func indentWrap(text string, width, indent int) string {
	wrapped := wordwrap.String(text, width-4)
	pad := strings.Repeat(" ", indent)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		if i > 0 {
			lines[i] = pad + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) flashLine() string {
	if m.flash == "" || m.api.now().Sub(m.flashAt) > 4*time.Second {
		return ""
	}
	return stAccent.Render(m.flash)
}

// ---- снапшоты из api (после reload)

func (a *API) snapshotChats() []Chat        { return a.allChats }
func (a *API) snapshotTasks() []Task        { return a.allTasks }
func (a *API) snapshotNotify() []NotifyItem { return a.allNotify }
