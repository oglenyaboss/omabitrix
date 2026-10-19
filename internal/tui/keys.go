package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// keys — единая точка хоткеев. q/esc идут «наружу» по стеку режимов.
func (m Model) keys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// оверлеи перехватывают всё
	switch m.overlay {
	case ovHelp:
		if msg.Code == tea.KeyEsc || msg.Code == tea.KeyEnter || msg.String() == "?" {
			m.overlay = ovNone
		}
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

	// вода ответа: всё уходит в textarea, кроме esc
	if m.focus == focusReply {
		switch msg.String() {
		case "esc":
			if m.active != nil && m.reply.Value() != "" {
				m.drafts[m.active.DialogID] = m.reply.Value()
			}
			m.focus = focusList
			m.layout()
			return m, nil
		case "ctrl+c":
			m.focus = focusList
			return m, nil
		}
		var cmd tea.Cmd
		m.reply, cmd = m.reply.Update(msg)
		if msg.Code == tea.KeyEnter {
			text := m.reply.Value()
			if trimNonEmpty(text) && m.active != nil && !m.sending {
				m.sending = true
				return m, m.sendCmd()
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
	switch msg.String() {
	case "q":
		if m.active != nil {
			m.closeDialog()
			return m, nil
		}
		if m.taskOpen {
			m.taskOpen = false
			m.cardReq = 0
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		if m.active != nil {
			m.closeDialog()
			return m, nil
		}
		if m.taskOpen {
			m.taskOpen = false
			m.cardReq = 0
			return m, nil
		}
	case "ctrl+c":
		return m, tea.Quit
	case "1":
		m.tab = tabChats
		m.clampCursors()
		return m, nil
	case "2":
		m.tab = tabTasks
		m.clampCursors()
		return m, nil
	case "3":
		m.tab = tabNotify
		m.clampCursors()
		return m, nil
	case "?":
		m.overlay = ovHelp
		return m, nil
	case "R":
		return m, m.reloadAllCmd()
	case "r":
		if m.tab == tabChats && m.active != nil {
			m.replyActive()
			return m, m.reply.Focus()
		}
	case "/":
		if m.tab == tabChats {
			m.focus = focusFilter
			return m, m.filterIn.Focus()
		}
	case "A":
		if m.tab == tabNotify {
			return m, m.readAllCmd()
		}
	case "s":
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
	case "d":
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
	case "c":
		if m.tab == tabTasks && m.taskOpen {
			if m.taskCard.ChatID <= 0 {
				m.flash = "у задачи нет чата"
				m.flashAt = m.api.now()
				return m, nil
			}
			id := "chat" + fmt.Sprint(m.taskCard.ChatID)
			m.openChatByDialog(id)
			return m, m.historyCmd(id, 0)
		}
	}

	// навигация по спискам
	switch msg.String() {
	case "j", "down":
		if m.visibleCount() > 0 && m.cursor < m.visibleCount()-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g":
		m.cursor = 0
	case "G":
		m.clampCursors()
		m.cursor = m.visibleCount() - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
	case "enter":
		return m.enterKey()
	}

	// скролл истории, если диалог открыт и нет фокуса ввода
	if m.active != nil {
		switch msg.String() {
		case "pgdown", "f":
			m.vp.PageDown()
		case "pgup", "b":
			m.vp.PageUp()
		case "h":
			// догрузка старых — позже: "H"
		case "H":
			if m.active != nil && !m.noMore && len(m.history) > 0 {
				first := m.history[0].ID
				return m, m.historyCmd(m.active.DialogID, first)
			}
		}
	}
	return m, nil
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
			m.cardReq = id
			return m, m.loadTaskCardCmd(id)
		}
	case tabNotify:
		if m.cursor < len(m.notify) {
			n := m.notify[m.cursor]
			if n.TaskID > 0 {
				m.tab = tabTasks
				return m, m.loadTaskCardCmd(n.TaskID)
			}
			m.flash = "у уведомления нет перехода"
			m.flashAt = m.api.now()
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

func trimNonEmpty(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\n' && r != '\t' && r != '\r' {
			return true
		}
	}
	return false
}
