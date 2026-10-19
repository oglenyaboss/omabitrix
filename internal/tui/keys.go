package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// keys — единая точка хоткеев. q/esc идут «наружу» по стеку режимов.
func (m Model) keys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// оверлеи перехватывают всё
	switch m.overlay {
	case ovHelp:
		m.overlay = ovNone // любая клавиша закрывает
		return m, nil
	case ovStatus:
		switch msg.Code {
		case tea.KeyEsc:
			m.overlay = ovNone
			return m, nil
		case tea.KeyEnter:
			return m.statusConfirm()
		case tea.KeyUp, tea.KeyDown:
			return m.statusMove(msg.String() == "up")
		}
		if msg.String() == "j" {
			return m.statusMove(false)
		}
		if msg.String() == "k" {
			return m.statusMove(true)
		}
		return m, nil

	case ovDeadline:
		opts := deadlineOptions(m.api.now(), m.taskCard.Deadline)
		switch msg.Code {
		case tea.KeyEsc:
			m.overlay = ovNone
			return m, nil
		case tea.KeyEnter:
			m.overlay = ovNone
			return m, m.deadlineCmd(m.taskCard.ID, opts[m.deadlineIdx].val)
		case tea.KeyUp:
			if m.deadlineIdx > 0 {
				m.deadlineIdx--
			}
			return m, nil
		case tea.KeyDown:
			if m.deadlineIdx < len(opts)-1 {
				m.deadlineIdx++
			}
			return m, nil
		}
		switch msg.String() {
		case "j":
			if m.deadlineIdx < len(opts)-1 {
				m.deadlineIdx++
			}
		case "k":
			if m.deadlineIdx > 0 {
				m.deadlineIdx--
			}
		case "1", "2", "3", "4", "5":
			if n := int(msg.String()[0] - '1'); n < len(opts) {
				m.deadlineIdx = n
			}
		}
		return m, nil
	}

	// вода ответа: всё уходит в textarea, кроме esc/ctrl+c (оба берут черновик)
	if m.focus == focusReply {
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.active != nil && m.reply.Value() != "" {
				m.drafts[m.active.DialogID] = m.reply.Value()
			}
			m.focus = focusList
			m.layout()
			return m, nil
		}
		var cmd tea.Cmd
		m.reply, cmd = m.reply.Update(msg)
		if msg.Code == tea.KeyEnter {
			// оптимистичная отправка: текст забираем и чистим поле сразу,
			// фокус остаётся — допечатанное переживёт ответ
			text := strings.TrimSpace(m.reply.Value())
			if text != "" && m.active != nil && !m.sending {
				m.reply.Reset()
				m.sending = true
				return m, m.sendCmd(text)
			}
		}
		return m, cmd
	}

	// фильтр: всё в input
	if m.focus == focusFilter {
		switch msg.Code {
		case tea.KeyEsc:
			m.focus = focusList
			m.filterIn.Blur()
			m.filter = ""
			m.cursor = 0
			return m, nil
		case tea.KeyEnter:
			m.focus = focusList
			m.filterIn.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.filterIn, cmd = m.filterIn.Update(msg)
		m.filter = m.filterIn.Value()
		m.clampCursors()
		return m, cmd
	}

	// глобальные
	switch {
	case key.Matches(msg, km.Quit), key.Matches(msg, km.Back):
		// назад по текущей вкладке: диалог → карточка; q в конце — выход
		if m.tab == tabChats && m.active != nil {
			m.closeDialog()
			return m, nil
		}
		if m.tab == tabTasks && m.taskOpen {
			m.taskOpen = false
			m.cardReq = 0
			return m, nil
		}
		if key.Matches(msg, km.Quit) {
			return m, tea.Quit
		}
	case key.Matches(msg, km.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, km.Tab1):
		m.switchTab(tabChats)
		return m, nil
	case key.Matches(msg, km.Tab2):
		m.switchTab(tabTasks)
		return m, nil
	case key.Matches(msg, km.Tab3):
		m.switchTab(tabNotify)
		return m, nil
	case key.Matches(msg, km.NextTab):
		m.switchTab((m.tab + 1) % tabCount)
		return m, nil
	case key.Matches(msg, km.PrevTab):
		m.switchTab((m.tab + tabCount - 1) % tabCount)
		return m, nil
	case key.Matches(msg, km.Help):
		m.overlay = ovHelp
		return m, nil
	case key.Matches(msg, km.Refresh):
		return m, m.reloadAllCmd()
	case key.Matches(msg, km.Reply):
		if m.tab == tabChats && m.active != nil {
			m.replyActive()
			return m, m.reply.Focus()
		}
	case key.Matches(msg, km.Filter):
		m.focus = focusFilter
		return m, m.filterIn.Focus()
	case key.Matches(msg, km.ReadAll):
		if m.tab == tabNotify {
			return m, m.readAllCmd()
		}
	case key.Matches(msg, km.Browser):
		return m.openBrowser()
	case key.Matches(msg, km.Status):
		if m.tab == tabTasks && m.taskOpen {
			if !m.cardMatchesCursor() {
				m.flash = "карточка другой задачи — enter, чтобы открыть эту"
				m.flashAt = m.api.now()
				return m, nil
			}
			m.overlay = ovStatus
			m.statusIdx = 0
			return m, nil
		}
	case key.Matches(msg, km.Deadline):
		if m.tab == tabTasks && m.taskOpen {
			if !m.cardMatchesCursor() {
				m.flash = "карточка другой задачи — enter, чтобы открыть эту"
				m.flashAt = m.api.now()
				return m, nil
			}
			m.overlay = ovDeadline
			m.deadlineIdx = 0
			return m, nil
		}
	case key.Matches(msg, km.TaskChat):
		if m.tab == tabTasks && m.taskOpen {
			if m.taskCard.ChatID <= 0 {
				m.flash = "у задачи нет чата"
				m.flashAt = m.api.now()
				return m, nil
			}
			id := "chat" + fmt.Sprint(m.taskCard.ChatID)
			m.openChatByDialog(id)
			m.active.Title = m.taskCard.Title
			return m, m.historyCmd(id, 0)
		}
	}

	// навигация по спискам
	switch {
	case key.Matches(msg, km.Down):
		if m.visibleCount() > 0 && m.cursor < m.visibleCount()-1 {
			m.cursor++
		}
		return m, m.followCard()
	case key.Matches(msg, km.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, m.followCard()
	case key.Matches(msg, km.Top):
		m.cursor = 0
		return m, m.followCard()
	case key.Matches(msg, km.Bottom):
		m.clampCursors()
		m.cursor = m.visibleCount() - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, m.followCard()
	case key.Matches(msg, km.Open):
		return m.enterKey()
	}

	// скролл истории, если диалог открыт и нет фокуса ввода
	if m.active != nil {
		switch {
		case key.Matches(msg, km.PageDown):
			m.vp.PageDown()
		case key.Matches(msg, km.PageUp):
			m.vp.PageUp()
		case key.Matches(msg, km.Older):
			if !m.noMore && len(m.history) > 0 {
				first := m.history[0].ID
				return m, m.historyCmd(m.active.DialogID, first)
			}
			if m.noMore {
				m.flash = "это начало переписки"
				m.flashAt = m.api.now()
			}
		}
	}
	return m, nil
}

// switchTab — смена вкладки: фильтр вкладко-специфичен, сбрасываем;
// курсор клампится под новый список.
func (m *Model) switchTab(t tab) {
	m.tab = t
	m.filter = ""
	m.filterIn.SetValue("")
	m.clampCursors()
}

func (m Model) enterKey() (tea.Model, tea.Cmd) {
	switch m.tab {
	case tabChats:
		if m.active != nil {
			return m, nil // уже в диалоге
		}
		m.openDialog(m.cursor)
		if m.active != nil {
			return m, m.historyCmd(m.active.DialogID, 0)
		}
	case tabTasks:
		rows := m.flatTasks()
		if m.cursor < len(rows) {
			id := rows[m.cursor].task.ID
			m.taskIdx = m.cursor
			return m, m.requestCard(id)
		}
	case tabNotify:
		vn := m.visibleNotify()
		if m.cursor < len(vn) {
			n := vn[m.cursor]
			if n.TaskID > 0 {
				m.tab = tabTasks
				cmd := m.requestCard(n.TaskID)
				if n.Unread {
					m.clearNotifyUnread(n.ID)
					return m, tea.Batch(cmd, m.readNotifyCmd(n.ID))
				}
				return m, cmd
			}
			m.flash = "у уведомления нет перехода"
			m.flashAt = m.api.now()
			if n.Unread {
				m.clearNotifyUnread(n.ID)
				return m, m.readNotifyCmd(n.ID)
			}
		}
	}
	return m, nil
}

type statusOption struct {
	code string
	name string
}

var allStatuses = []statusOption{
	{"2", "ждёт выполнения"},
	{"3", "в работе"},
	{"4", "на проверке"},
	{"6", "отложена"},
	{"5", "завершена"},
}

// statusOptions — доступные переходы (текущий + остальные; закрытая → все).
func statusOptions(current string) []statusOption {
	if current == "5" || current == "6" || current == "7" {
		return append([]statusOption{{"2", "переоткрыть"}}, allStatuses[:4]...)
	}
	out := make([]statusOption, 0, len(allStatuses))
	for _, s := range allStatuses {
		if s.code != current {
			out = append(out, s)
		}
	}
	return out
}

func (m Model) statusMove(up bool) (tea.Model, tea.Cmd) {
	opts := statusOptions(m.taskCard.Status)
	if m.statusIdx <= 0 && up || m.statusIdx >= len(opts)-1 && !up {
		return m, nil
	}
	if up {
		m.statusIdx--
	} else {
		m.statusIdx++
	}
	return m, nil
}

func (m Model) statusConfirm() (tea.Model, tea.Cmd) {
	opts := statusOptions(m.taskCard.Status)
	if m.statusIdx < 0 || m.statusIdx >= len(opts) {
		return m, nil
	}
	return m, m.statusCmd(m.taskCard.ID, opts[m.statusIdx].code)
}
