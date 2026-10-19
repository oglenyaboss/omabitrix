package tui

import (
	"context"
	"sort"
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
	tabHome tab = iota
	tabTasks
	tabChats
	tabNotify
	tabCount
)

type focus int

const (
	focusList focus = iota
	focusReply
	focusFilter
	focusSearch
)

type overlay int

const (
	ovNone overlay = iota
	ovHelp
	ovStatus
	ovDeadline
	ovConfirm // «удалить сообщение? y/n»
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

	searchIn  textinput.Model // поиск по истории открытого диалога
	searchQ   string          // запрос последнего поиска — заголовок и подсветка
	searchRes []Message       // результаты поиска (свежие первыми)
	searching bool            // запрос поиска в полёте
	searchOn  bool            // панель диалога показывает результаты, не историю
	searchOff int             // строковый офсет окна результатов

	sel      int              // выделенное сообщение (-1 — режим выключен)
	replyTo  Message          // ответ-цитата для следующей отправки
	editID   int64            // правка своего сообщения
	delID    int64            // сообщение под подтверждением удаления
	mentions map[string]int64 // имена, вставленные по «@»: имя → id автора
	msgLine  map[int64]int    // стартовая строка сообщения в рендере истории

	taskIdx     int    // выбранная задача в плоском списке
	taskRole    string // роль доски: do | help | watch (пусто — do)
	taskProject int64  // фильтр доски по проекту; 0 — все
	taskCard    Task
	taskOpen    bool
	cardReq     int64 // последний запрошенный id — защита от гонки ответов
	cardLoading bool  // карточка ещё грузится — в панели «загрузка…»
	followSeq   int   // тик «карточка за курсором»: живёт только последний
	statusIdx   int
	deadlineIdx int
	descOff     int        // окно прокрутки описания в карточке
	drag        *boardDrag // перетаскивание карточки мышью на доске

	drafts map[string]string // черновики по диалогам

	listW, dialW, cardW, bodyH int

	width, height int
	frame         int // кадр анимаций (animTick): пульс сроков, мигание часов
	spinner       spinner.Model
	ready         bool
	flash         string
	flashAt       time.Time
	toast         *toastInfo // всплывашка «новое сообщение из другого чата»

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
	si := textinput.New()
	si.Placeholder = "поиск по истории…"
	si.CharLimit = 100
	h := help.New()
	h.ShortSeparator = "  "
	h.FullSeparator = "  "
	h.Styles = help.Styles{
		ShortKey:       stKey,
		ShortDesc:      stHelpDesc,
		ShortSeparator: stDim,
		FullKey:        stHelpKey,
		FullDesc:       stHelpDesc,
		FullSeparator:  stDim,
		Ellipsis:       stDim,
	}
	return Model{
		api: api, ctx: ctx,
		spinner: sp, reply: ta, filterIn: ti, searchIn: si, help: h,
		sel: -1, drafts: map[string]string{}, mentions: map[string]int64{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.reloadAllCmd(), tick10s(), animTick())
}

type frameMsg struct{}

// animTick — кадр анимаций, 150 мс: цикл пульса 16 кадров ≈ 2.4 с.
// ponytail: тикает всегда; при простое можно останавливать, когда в кадре
// нечему двигаться.
func animTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return frameMsg{} })
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

	case frameMsg:
		m.frame++
		return m, animTick()

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
			// тост «новое сообщение»: счётчик чужого чата вырос при перечитке
			old := map[string]int{}
			for _, c := range m.chats {
				old[c.DialogID] = c.Counter
			}
			fresh := m.api.snapshotChats()
			if len(m.chats) > 0 {
				for _, c := range fresh {
					if c.Counter <= old[c.DialogID] {
						continue
					}
					if m.active != nil && c.DialogID == m.active.DialogID {
						continue
					}
					m.toast = &toastInfo{chat: c, at: m.api.now(), frame: m.frame}
					break
				}
			}
			m.chats = fresh
		}
		if msg.tasks {
			m.tasks = m.api.snapshotTasks()
			m.buckets = Buckets(m.tasks, m.srvLoc(), m.api.now())
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
			selID := int64(0)
			if m.sel >= 0 && m.sel < len(m.history) {
				selID = m.history[m.sel].ID
			}
			if msg.prepend {
				// догрузка старых: держим позицию (YOffset += прирост)
				old := m.vp.TotalLineCount()
				atBottom := m.vp.AtBottom() || old == 0
				m.history = append(msg.rows, m.history...)
				m.keepSel(selID)
				m.renderHistory()
				if !atBottom {
					m.vp.SetYOffset(m.vp.TotalLineCount() - old)
				}
			} else {
				// тик-обновление не должно вырывать юзера из истории: к низу —
				// только если он и был у низа
				atBottom := m.vp.AtBottom() || len(m.history) == 0
				m.history = mergeHistory(m.history, msg.rows)
				m.keepSel(selID)
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

	case searchMsg:
		if m.active == nil || m.active.DialogID != msg.dialogID {
			return m, nil // активный диалог сменился — ответ устарел
		}
		m.searching = false
		if msg.err != nil {
			m.searchOn = false
			m.flash = msg.err.Error()
			m.flashAt = m.api.now()
			return m, nil
		}
		m.searchRes = msg.rows
		m.searchQ = msg.q
		m.searchOff = 0
		return m, nil

	case sentMsg:
		m.sending = false
		// BB-коды упоминаний → «@Имя»: и в истории, и в поле при ошибке
		msg.text, _ = applyMentions(msg.text, m.api.My)
		m.replyTo = Message{} // цитата ушла вместе с сообщением (или чат сменился);
		// при ошибке ответ на цитату не восстанавливаем — текст вернулся в поле
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

	case stageMsg:
		// перенос стадии: при ошибке перечитка вернёт задачу в прежнюю стадию
		if msg.err != nil {
			m.loadErr = "стадия не изменена: " + msg.err.Error()
		}
		if m.taskOpen {
			return m, tea.Batch(m.loadCmd("tasks"), m.loadTaskCardCmd(m.taskCard.ID))
		}
		return m, m.loadCmd("tasks")

	case actionMsg:
		if msg.what == "правка" {
			m.sending = false // флаг ставит только отправка правки
		}
		if msg.err != nil {
			m.loadErr = msg.what + ": " + msg.err.Error()
			return m, m.refreshActiveCmd()
		}
		if msg.what == "удаление" {
			// успех: убираем сообщение из истории локально
			for i := range m.history {
				if m.history[i].ID == msg.id {
					m.history = append(m.history[:i], m.history[i+1:]...)
					break
				}
			}
			if m.sel >= len(m.history) {
				m.sel = -1
			}
			m.renderHistory()
		}
		return m, nil

	case taskCardMsg:
		if msg.requested != m.cardReq {
			return m, nil // устарел: пока грузили, выбрали другую задачу
		}
		m.cardLoading = false
		if msg.err != nil {
			m.loadErr = msg.err.Error()
			return m, nil
		}
		if msg.task.ID != m.taskCard.ID {
			m.descOff = 0 // новая задача — описание с начала
		}
		m.taskCard = msg.task
		// tasks.task.get поля канбана не отдаёт — переносим из списка
		found := false
		for i := range m.tasks {
			if m.tasks[i].ID == msg.task.ID {
				found = true
				m.taskCard.Role = m.tasks[i].Role
				m.taskCard.GroupID = m.tasks[i].GroupID
				m.taskCard.Project = m.tasks[i].Project
				m.taskCard.StageID = m.tasks[i].StageID
				m.taskCard.Stage = m.tasks[i].Stage
				m.taskCard.StageColor = m.tasks[i].StageColor
				m.taskCard.StageSort = m.tasks[i].StageSort
				break
			}
		}
		if !found && m.taskCard.GroupID > 0 {
			// карточка не с доски (из уведов): проект и стадия из кэша меты
			m.api.Annotate(&m.taskCard)
		}
		m.taskOpen = true
		m.tab = tabTasks
		m.cursor = m.taskIdx
		m.layout()
		// двухпанельный экран: чат задачи открывается сразу, без смены вкладки
		var cmd tea.Cmd
		if id := taskChatDialog(m.taskCard.ChatID); m.taskCard.ChatID > 0 && m.twoPane() &&
			(m.active == nil || m.active.DialogID != id) {
			m.openTaskChatInline(id)
			m.active.Title = "чат задачи"
			cmd = m.historyCmd(id, 0)
		}
		return m, cmd

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

	case tea.MouseMotionMsg:
		return m.mouseMotion(msg)

	case tea.MouseReleaseMsg:
		return m.mouseRelease(msg)

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
	if m.taskOpen && m.width >= 100 {
		// экран задачи: слева карточка, справа чат; между ними 1 пробел
		leftW := (m.width - 1) * 11 / 20
		m.cardW = leftW - 4
		m.dialW = m.width - leftW - 1 - 4
	} else {
		m.dialW = m.width - (m.listW + 4) - 2 - 4
		m.cardW = m.width - 4 // вкладка задач: карточка на всю ширину доски
	}
	if m.dialW < 20 {
		m.dialW = 20
	}
	if m.cardW < 20 {
		m.cardW = 20
	}
	m.bodyH = m.height - 2 // строка вкладок + футер
	if m.bodyH < 6 {
		m.bodyH = 6
	}
	// единственная точка правды ширины истории: минус колонка скроллбара
	m.vp.SetWidth(m.dialW - 2)
	replyRows := 2 // пустая + подсказка «r — ответить»
	if m.focus == focusReply {
		replyRows = 4 // разделитель «── ответ» + поле
		if m.replyTo.ID > 0 || m.editID > 0 {
			replyRows = 5 // + строка цитаты
		}
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
	if m.focus == focusSearch {
		m.searchIn.SetWidth(m.dialW - 6)
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
	case tabHome:
		return ""
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

type searchMsg struct {
	dialogID string
	q        string
	rows     []Message
	err      error
}

// searchCmd — поиск по истории активного диалога; dialogID фиксируем для
// гейта гонки (сменился диалог — ответ устарел).
func (m Model) searchCmd(chatID int64, q string) tea.Cmd {
	api, ctx := m.api, m.ctx
	dialogID := m.active.DialogID
	return func() tea.Msg {
		rows, err := api.SearchMessages(ctx, chatID, q)
		return searchMsg{dialogID: dialogID, q: q, rows: rows, err: err}
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
// replyID > 0 — ответ-цитата.
func (m Model) sendCmd(text string, replyID int64) tea.Cmd {
	api, ctx := m.api, m.ctx
	dialogID := m.active.DialogID
	return func() tea.Msg {
		id, err := api.Send(ctx, dialogID, text, replyID)
		return sentMsg{dialogID: dialogID, id: id, text: text, err: err}
	}
}

// actionMsg — ответ правки/удаления/лайка; при ошибке история перечитывается.
type actionMsg struct {
	err  error
	what string // «правка» / «удаление» / «лайк» — префикс ошибки
	id   int64  // сообщение; для удаления — что убрать из истории
}

func (m Model) updateCmd(id int64, text string) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg {
		return actionMsg{err: api.UpdateMessage(ctx, id, text), what: "правка", id: id}
	}
}

func (m Model) deleteCmd(id int64) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg {
		return actionMsg{err: api.DeleteMessage(ctx, id), what: "удаление", id: id}
	}
}

func (m Model) likeCmd(id int64, like bool) tea.Cmd {
	api, ctx := m.api, m.ctx
	return func() tea.Msg {
		return actionMsg{err: api.LikeMessage(ctx, id, like), what: "лайк", id: id}
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

// mouseMotion — движение с зажатой кнопкой: запоминаем колонку-цель переноса.
func (m Model) mouseMotion(msg tea.MouseMotionMsg) (tea.Model, tea.Cmd) {
	if m.drag == nil || m.taskOpen || m.tab != tabTasks || m.overlay != ovNone {
		return m, nil
	}
	if col, _ := m.boardHit(msg.X, msg.Y); col >= 0 {
		m.drag.toCol = col
	}
	return m, nil
}

// mouseRelease — бросили карточку: перенос в стадию колонки-цели.
// Отпускание над той же колонкой — обычный клик (повторный по выбранной
// карточке открывает её).
func (m Model) mouseRelease(msg tea.MouseReleaseMsg) (tea.Model, tea.Cmd) {
	d := m.drag
	m.drag = nil
	if d == nil || m.taskOpen || m.tab != tabTasks || m.overlay != ovNone {
		return m, nil
	}
	col, row := m.boardHit(msg.X, msg.Y)
	if col < 0 {
		return m, nil
	}
	if col == d.fromCol {
		if d.wasSel && row >= 0 {
			cols := m.boardColumns()
			if col < len(cols) && row < len(cols[col].rows) &&
				cols[col].rows[row].task.ID == d.taskID {
				return m.enterKey()
			}
		}
		return m, nil
	}
	cols := m.boardColumns()
	if col >= len(cols) {
		return m, nil
	}
	var t Task
	for _, x := range m.tasks {
		if x.ID == d.taskID {
			t = x
			break
		}
	}
	if t.ID == 0 {
		return m, nil
	}
	for i, s := range m.api.StagesOf(t.GroupID) {
		if s.Title == cols[col].title {
			return m.setStage(t, i)
		}
	}
	m.flash = "в проекте нет стадии " + cols[col].title
	m.flashAt = m.api.now()
	return m, nil
}

// mouseLeft: геометрия строк синхронна с chatWindow/notifyWindow.
func (m Model) mouseLeft(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	switch m.tab {
	case tabChats:
		if msg.X > m.listW+4 {
			return m, nil
		}
		vc := m.visibleChats()
		start, _, filterRows := m.chatWindow(vc)
		idx := (msg.Y - 2 - filterRows) / 2 // строка 0 — вкладки, 1 — верхняя грань рамки
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
		if m.taskOpen {
			return m, nil
		}
		// доска: клик по карточке — курсор, повторный по выбранной — открыть;
		// зажатие начинает перетаскивание
		col, row := m.boardHit(msg.X, msg.Y)
		if col < 0 || row < 0 {
			return m, nil
		}
		cols := m.boardColumns()
		rows := m.flatTasks()
		idx := boardIndex(cols, col, row)
		if idx < 0 || idx >= len(rows) {
			return m, nil
		}
		wasSel := m.cursor == idx
		m.cursor = idx
		m.drag = &boardDrag{taskID: rows[idx].task.ID, fromCol: col, toCol: col, wasSel: wasSel}
		return m, nil
	case tabNotify:
		vn := m.visibleNotify()
		start, _ := m.notifyWindow(len(vn))
		idx := (msg.Y - 2 - m.filterRows()) / 2
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
	case tabHome:
		return 0
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
	kind string // overdue/today/later/nodate
	col  int    // индекс колонки доски
}

// flatTasks — строки доски колонка за колонкой (порядок boardColumns).
func (m *Model) flatTasks() []taskRow {
	cols := m.boardColumns()
	out := make([]taskRow, 0, len(m.tasks))
	for _, c := range cols {
		out = append(out, c.rows...)
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
// twoPane — экран задачи с чатом справа: места достаточно.
func (m Model) twoPane() bool { return m.taskOpen && m.width >= 100 }

// taskChatDialog — DialogID чата задачи по ChatID.
func taskChatDialog(chatID int64) string {
	return "chat" + strconv.FormatInt(chatID, 10)
}

// openTaskChatInline — чат задачи без смены вкладки: панель справа на
// экране задачи. Вкладку и курсор списка не трогает.
func (m *Model) openTaskChatInline(id string) {
	for i := range m.chats {
		if m.chats[i].DialogID == id {
			m.active = &m.chats[i]
			m.reply.SetValue(m.drafts[id])
			m.resetSearch()
			m.openHistory()
			return
		}
	}
	// чат задачи не в списке (отфильтрован) — synthetic
	m.active = &Chat{DialogID: id, Title: "чат " + id}
	m.reply.SetValue(m.drafts[id])
	m.openHistory()
}

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
	m.resetSearch() // результаты чужого диалога показывать нельзя
	m.openHistory()
}

// closeDialog — выход из чата: с карточки задачи возвращаемся на неё.
func (m *Model) closeDialog() {
	if m.active != nil && m.reply.Value() != "" {
		m.drafts[m.active.DialogID] = m.reply.Value()
	}
	m.active = nil
	m.resetSearch()
	m.sel = -1
	m.mentions = map[string]int64{}
	if m.taskOpen {
		m.tab = tabTasks
	}
}

// resetSearch — выход из режима поиска: панель возвращается к истории.
func (m *Model) resetSearch() {
	m.searchOn = false
	m.searching = false
	m.searchRes = nil
	m.searchQ = ""
	m.searchOff = 0
	m.searchIn.SetValue("")
}

func (m *Model) openHistory() {
	m.history = nil
	m.noMore = false
	m.historyLoading = true
	m.sel = -1
	m.editID = 0
	m.replyTo = Message{}
	m.mentions = map[string]int64{}
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
// группируются (имя один раз), свои — прижаты вправо, даты по центру.
// Выделенному (sel) — маркер в колонке отступа (у своих — в колонке 0),
// сообщениям — цитата ответа и сердечки лайков. потолок: prepend-догрузка
// может слить две серии одного автора на границе — сдвиг скролла на 1–2
// строки, приемлемо.
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
	type hmsg struct {
		msg Message
		idx int // позиция в m.history — сравнение с sel
	}
	type hgroup struct {
		author       int64
		name         string
		mine, system bool
		msgs         []hmsg
	}
	var groups []hgroup
	for i, msg := range m.history {
		if msg.System {
			groups = append(groups, hgroup{system: true, msgs: []hmsg{{msg, i}}})
			continue
		}
		if n := len(groups); n > 0 && !groups[n-1].system &&
			groups[n-1].mine == msg.Mine && groups[n-1].author == msg.AuthorID {
			groups[n-1].msgs = append(groups[n-1].msgs, hmsg{msg, i})
			continue
		}
		groups = append(groups, hgroup{author: msg.AuthorID, name: msg.Name,
			mine: msg.Mine, msgs: []hmsg{{msg, i}}})
	}
	var b strings.Builder
	var prevDay string
	line := 0
	m.msgLine = map[int64]int{} // стартовые строки сообщений — скролл к выделенному

	// writeMsg — блок одного сообщения: цитата, затем текст; у выделенного
	// каждая строка блока получает маркер в колонке отступа.
	writeMsg := func(hm hmsg, head, text string) {
		m.msgLine[hm.msg.ID] = line
		var lines []string
		if q := m.quoteLine(hm.msg, W-6); q != "" {
			lines = append(lines, q)
		}
		lines = append(lines, strings.Split(indentWrap(head+text, W, 2), "\n")...)
		if hm.idx == m.sel {
			for i := range lines {
				lines[i] = stCursor.Render("▌") + " " + lines[i]
			}
		}
		for _, l := range lines {
			b.WriteString(l + "\n")
			line++
		}
	}
	for gi, g := range groups {
		first := g.msgs[0].msg
		if first.DayKey != prevDay && first.DayLabel != "" {
			if gi > 0 {
				b.WriteString("\n")
				line++
			}
			b.WriteString(dayDivider(stDim.Render(first.DayLabel), W) + "\n")
			line++
			prevDay = first.DayKey
		} else if gi > 0 {
			b.WriteString("\n")
			line++
		}
		switch {
		case g.system:
			for _, hm := range g.msgs {
				m.msgLine[hm.msg.ID] = line
				b.WriteString(centered(stDim.Render("· "+hm.msg.Text+" ·"), W) + "\n")
				line++
			}
		case g.mine:
			// свои сообщения прижаты вправо: перенос по словам на 2/3 ширины,
			// правый край строки на колонке W; время и «›» — в конце последней
			// строки первого сообщения группы; у выделенного маркер в колонке 0
			tw := W * 2 / 3
			if lim := W - 4; tw > lim {
				tw = lim
			}
			if tw < 1 {
				tw = 1
			}
			for i, hm := range g.msgs {
				m.msgLine[hm.msg.ID] = line
				edge := W
				mark := ""
				if hm.idx == m.sel {
					edge -= 2 // место под «▌ » в колонке 0
					mark = stCursor.Render("▌") + " "
				}
				tail, wrapW := "", tw
				if i == 0 {
					tail = stDim.Render("  "+hm.msg.Time) + " " + stAccent.Render("›")
					// хвост должен остаться в пределах края — учитываем при переносе
					if lim := edge - lipgloss.Width(tail); wrapW > lim {
						wrapW = lim
					}
					if wrapW < 1 {
						wrapW = 1
					}
				}
				var lines []string
				if q := m.quoteLine(hm.msg, wrapW); q != "" {
					lines = append(lines, q)
				}
				wrapped := strings.Split(wordwrap.String(hm.msg.Text, wrapW), "\n")
				for j, bl := range wrapped {
					l := stBubbleMine.Render(bl)
					if j == len(wrapped)-1 {
						l += tail
					}
					lines = append(lines, l)
				}
				if h := hearts(hm.msg); h != "" {
					lines = append(lines, h) // сердечки — справа под сообщением
				}
				for _, l := range lines {
					pad := edge - lipgloss.Width(l)
					if pad < 0 {
						pad = 0
					}
					b.WriteString(mark + strings.Repeat(" ", pad) + l + "\n")
					line++
				}
			}
		default:
			name := g.name
			if name == "" {
				name = "?"
			}
			b.WriteString(authorStyle(g.author).Render(name) +
				stDim.Render(" "+first.Time) + "\n")
			line++
			for _, hm := range g.msgs {
				text := hm.msg.Text + hearts(hm.msg)
				if hm.msg.MentionsMe {
					text = stBold.Render(text) // упомянули меня — жирным
				}
				writeMsg(hm, "", text)
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

// keepSel — выделение переживает пересборку истории: ищем то же сообщение.
func (m *Model) keepSel(id int64) {
	m.sel = -1
	if id <= 0 {
		return
	}
	for i := range m.history {
		if m.history[i].ID == id {
			m.sel = i
			return
		}
	}
}

// enterSel — режим выделения: встаём на последнее не-системное сообщение.
func (m *Model) enterSel() {
	for i := len(m.history) - 1; i >= 0; i-- {
		if !m.history[i].System {
			m.selTo(i)
			return
		}
	}
	m.sel = -1
}

// selTo — сменить выделение: маркер рисуется в renderHistory,
// поэтому любая смена требует перерисовки; выделенное держим в кадре.
func (m *Model) selTo(idx int) {
	m.sel = idx
	m.renderHistory()
	m.scrollToSel()
}

// selStep — шаг выделения, системные перескакиваем.
func (m *Model) selStep(dir int) {
	for i := m.sel + dir; i >= 0 && i < len(m.history); i += dir {
		if !m.history[i].System {
			m.sel = i
			return
		}
	}
}

// topExtra — строка «начало переписки» над контентом (msgLine её не считает).
func (m *Model) topExtra() int {
	if m.noMore && len(m.history) > 0 {
		return 1
	}
	return 0
}

// scrollToSel — держать выделенное сообщение в кадре.
func (m *Model) scrollToSel() {
	if m.sel < 0 || m.sel >= len(m.history) {
		return
	}
	start, ok := m.msgLine[m.history[m.sel].ID]
	if !ok {
		return
	}
	start += m.topExtra()
	if start < m.vp.YOffset() || start >= m.vp.YOffset()+m.vp.Height() {
		m.vp.SetYOffset(start)
	}
}

// quoteLine — строка-цитата «↳ Имя: текст», если сообщение-ответ найдено
// в истории; w — ширина для резки.
func (m *Model) quoteLine(msg Message, w int) string {
	if msg.ReplyID <= 0 {
		return ""
	}
	for _, h := range m.history {
		if h.ID == msg.ReplyID {
			name := h.Name
			if name == "" {
				name = "?"
			}
			return stDim.Render(truncate("↳ "+name+": "+h.Text, w))
		}
	}
	return ""
}

// hearts — хвостик с лайками « ♥ 2»; мой лайк — акцентом.
func hearts(msg Message) string {
	if msg.Likes <= 0 {
		return ""
	}
	st := stDim
	if msg.LikedByMe {
		st = stAccent
	}
	return " " + st.Render("♥ "+strconv.Itoa(msg.Likes))
}

// applyEdit — оптимистичная правка: текст в истории сразу, ответ сервера
// подтвердит или откатит перечиткой.
func (m *Model) applyEdit(id int64, text string) {
	for i := range m.history {
		if m.history[i].ID == id {
			m.history[i].Text = text
			break
		}
	}
	m.renderHistory()
}

// expandMentions — «@Имя» из подсказок → BB-код портала; длинные имена
// первыми, чтобы «Анна» не съела «@Анна Петрова».
func (m Model) expandMentions(text string) string {
	names := make([]string, 0, len(m.mentions))
	for n := range m.mentions {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		tag := "[USER=" + strconv.FormatInt(m.mentions[n], 10) + "]" + n + "[/USER]"
		text = strings.ReplaceAll(text, "@"+n, tag)
	}
	return text
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

// srvLoc — зона портала для дедлайнов; без клиента (тесты) — UTC.
func (m Model) srvLoc() *time.Location {
	if m.api == nil || m.api.C == nil {
		return time.UTC
	}
	return m.api.C.ServerLoc()
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
