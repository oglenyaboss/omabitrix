package tui

import (
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// expandPath — «~» → домашний каталог в пути прикрепления.
func expandPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// keys — единая точка хоткеев. q/esc идут «наружу» по стеку режимов.
func (m Model) keys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// оверлеи перехватывают всё
	switch m.overlay {
	case ovImage:
		// просмотр вложения: прокрутка, enter — системный вьювер
		switch msg.String() {
		case "esc", "q":
			m.overlay = ovNone
			m.imgMsg = Message{}
		case "enter":
			return m.openImageExternal()
		case "j", "down":
			m.imgOff++
		case "k", "up":
			if m.imgOff > 0 {
				m.imgOff--
			}
		case "pgdown", "f", " ":
			m.imgOff += 10
		case "pgup", "b":
			m.imgOff = max(m.imgOff-10, 0)
		}
		return m, nil
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

	case ovConfirm:
		// подтверждение удаления: y — удаляем, любая другая клавиша — закрыть
		id := m.delID
		m.overlay = ovNone
		m.delID = 0
		if msg.String() == "y" {
			return m, m.deleteCmd(id)
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
			if m.editID > 0 {
				// правка отменяется целиком: черновик не сохранять
				m.editID = 0
				m.reply.Reset()
			} else if m.active != nil && m.reply.Value() != "" {
				m.drafts[m.active.DialogID] = m.reply.Value()
			}
			m.replyTo = Message{}
			m.focus = focusList
			m.layout()
			return m, nil
		case "pgup":
			if m.taskOpen {
				m.vp.PageUp() // скролл истории, пока фокус в чате задачи
				return m, nil
			}
		case "pgdown":
			if m.taskOpen {
				m.vp.PageDown()
				return m, nil
			}
		case "a":
			// приложить файл к следующей отправке; правку не смешиваем
			if m.active != nil && m.editID == 0 {
				m.focus = focusAttach
				return m, m.attachIn.Focus()
			}
		case "ctrl+v":
			// картинка из буфера обмена (Wayland, wl-paste)
			if m.active != nil && m.editID == 0 && !m.sending {
				m.sending = true
				return m, m.sendClipboardCmd()
			}
		}
		var cmd tea.Cmd
		m.reply, cmd = m.reply.Update(msg)
		if msg.Code == tea.KeyEnter {
			// оптимистичная отправка: текст забираем и чистим поле сразу,
			// фокус остаётся — допечатанное переживёт ответ
			text := strings.TrimSpace(m.reply.Value())
			if text != "" && m.active != nil && !m.sending {
				wire := m.expandMentions(text)
				m.reply.Reset()
				m.sending = true
				if m.editID > 0 {
					// правка: в истории — читаемый текст, на сервер — с BB-кодами
					id := m.editID
					m.editID = 0
					m.applyEdit(id, text)
					return m, m.updateCmd(id, wire)
				}
				return m, m.sendCmd(wire, m.replyTo.ID)
			}
			if text == "" && m.attach != "" && m.active != nil && !m.sending {
				// файл без подписи
				path := m.attach
				m.attach = ""
				m.sending = true
				return m, m.sendFileCmd(path, "")
			}
		}
		return m, cmd
	}

	// ввод пути к файлу: enter — приложить, esc — назад в поле ответа
	if m.focus == focusAttach {
		switch msg.Code {
		case tea.KeyEsc:
			m.focus = focusReply
			m.attachIn.Blur()
			return m, m.reply.Focus()
		case tea.KeyEnter:
			path := expandPath(strings.TrimSpace(m.attachIn.Value()))
			m.attachIn.Blur()
			m.attachIn.SetValue("")
			m.focus = focusReply
			if st, err := os.Stat(path); err != nil || st.IsDir() {
				m.flash = "не нашёл файл: " + path
				m.flashAt = m.api.now()
				return m, m.reply.Focus()
			}
			m.attach = path
			return m, m.reply.Focus()
		}
		var cmd tea.Cmd
		m.attachIn, cmd = m.attachIn.Update(msg)
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

	// поиск по истории: всё в input
	if m.focus == focusSearch {
		switch msg.Code {
		case tea.KeyEsc:
			m.focus = focusList
			m.searchIn.Blur()
			return m, nil
		case tea.KeyEnter:
			m.focus = focusList
			m.searchIn.Blur()
			if m.active != nil {
				q := strings.TrimSpace(m.searchIn.Value())
				m.searchOn = true
				m.searching = true
				return m, m.searchCmd(m.active.ChatID, q)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.searchIn, cmd = m.searchIn.Update(msg)
		return m, cmd
	}

	// результаты поиска в панели диалога: esc/q — назад к истории,
	// навигация — прокрутка окна результатов; остальное — как обычно
	if m.searchOn {
		handled := true
		switch {
		case key.Matches(msg, km.Back), key.Matches(msg, km.Quit):
			m.searchOn = false
		case key.Matches(msg, km.PageDown):
			m.searchOff += m.vp.Height()
		case key.Matches(msg, km.PageUp):
			m.searchOff -= m.vp.Height()
		case key.Matches(msg, km.Down):
			m.searchOff += 2 // один результат — две строки
		case key.Matches(msg, km.Up):
			m.searchOff -= 2
		case key.Matches(msg, km.Reply), key.Matches(msg, km.Older), key.Matches(msg, km.Select):
			// ответ и догрузка истории поверх результатов не действуют
		default:
			handled = false
		}
		if handled {
			if max := m.searchMaxOff(); m.searchOff > max {
				m.searchOff = max
			}
			if m.searchOff < 0 {
				m.searchOff = 0
			}
			return m, nil
		}
	}

	// режим выделения сообщения: перехватывает всё, кроме «?» и ctrl+c
	if m.sel >= 0 && m.active != nil && m.tab == tabChats {
		if m.sel >= len(m.history) {
			m.sel = -1
		} else {
			switch msg.String() {
			case "?", "ctrl+c":
			default:
				return m.selKey(msg)
			}
		}
	}

	// пульт: n/enter — открыть ближайшую задачу
	if m.tab == tabHome && m.focus == focusList {
		if key.Matches(msg, km.NextDue) || key.Matches(msg, km.Open) {
			if t, ok := m.nextDue(); ok {
				id := t.ID
				tabCmd := m.switchTab(tabTasks)
				for i, r := range m.flatTasks() {
					if r.task.ID == id {
						m.cursor, m.taskIdx = i, i
						break
					}
				}
				return m, tea.Batch(tabCmd, m.requestCard(id))
			}
			m.flash = "сроков нет"
			m.flashAt = m.api.now()
			return m, nil
		}
	}

	// канбан-доска задач: колонки, перенос стадии, роль/проект; до глобальных —
	// чтобы свои h/l/H/L/t/p не столкнулись с чатами (H — «раньше», l — лайк)
	if m.tab == tabTasks && m.focus == focusList && m.overlay == ovNone {
		if m.taskOpen {
			// экран задачи: j/k/pgup/pgdn скроллят описание, i/tab — фокус в чат
			if key.Matches(msg, km.FocusChat) && m.twoPane() {
				if m.active != nil {
					m.replyActive()
					return m, m.reply.Focus()
				}
				return m, nil
			}
			switch {
			case key.Matches(msg, km.Open) && m.twoPane():
				// enter на карточке — в поле ответа (узкий экран — как раньше)
				if m.active == nil {
					m.flash = "у задачи нет чата"
					m.flashAt = m.api.now()
					return m, nil
				}
				m.replyActive()
				return m, m.reply.Focus()
			case key.Matches(msg, km.Down):
				m.descOff++
				return m, nil
			case key.Matches(msg, km.Up):
				if m.descOff > 0 {
					m.descOff--
				}
				return m, nil
			case key.Matches(msg, km.PageDown), key.Matches(msg, km.PageUp):
				step := m.cardPage()
				if msg.String() == "pgup" || msg.String() == "b" {
					step = -step
				}
				m.descOff = max(m.descOff+step, 0)
				return m, nil
			}
		}
		switch {
		case key.Matches(msg, km.ColLeft):
			m.moveCol(-1)
			return m, m.followCard()
		case key.Matches(msg, km.ColRight):
			m.moveCol(1)
			return m, m.followCard()
		case key.Matches(msg, km.Down):
			// j/k не выходят за пределы текущей колонки
			rows := m.flatTasks()
			if m.cursor >= 0 && m.cursor < len(rows)-1 &&
				rows[m.cursor+1].col == rows[m.cursor].col {
				m.cursor++
			}
			return m, m.followCard()
		case key.Matches(msg, km.Up):
			rows := m.flatTasks()
			if m.cursor > 0 && rows[m.cursor-1].col == rows[m.cursor].col {
				m.cursor--
			}
			return m, m.followCard()
		case key.Matches(msg, km.StageMove):
			dir := 1
			if msg.String() == "H" {
				dir = -1
			}
			return m.moveStage(dir)
		case key.Matches(msg, km.Role):
			switch m.boardRole() {
			case "do":
				m.taskRole = "help"
			case "help":
				m.taskRole = "watch"
			default:
				m.taskRole = "do"
			}
			m.cursor, m.taskIdx = 0, 0
			return m, nil
		case key.Matches(msg, km.Project):
			m.taskProject = m.nextProject()
			m.cursor, m.taskIdx = 0, 0
			return m, nil
		}
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
			// чат задачи закрывается вместе с карточкой (черновик остаётся)
			if m.active != nil && m.active.DialogID == taskChatDialog(m.taskCard.ChatID) {
				m.closeDialog()
			}
			m.layout()
			return m, nil
		}
		if key.Matches(msg, km.Quit) {
			return m, tea.Quit
		}
	case key.Matches(msg, km.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, km.Tab1):
		return m, m.switchTab(tabHome)
	case key.Matches(msg, km.Tab2):
		return m, m.switchTab(tabTasks)
	case key.Matches(msg, km.Tab3):
		return m, m.switchTab(tabChats)
	case key.Matches(msg, km.Tab4):
		return m, m.switchTab(tabNotify)
	case key.Matches(msg, km.NextTab):
		return m, m.switchTab((m.tab + 1) % tabCount)
	case key.Matches(msg, km.PrevTab):
		return m, m.switchTab((m.tab + tabCount - 1) % tabCount)
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
		if m.tab == tabTasks && m.taskOpen {
			if m.twoPane() {
				// чат уже открыт справа — только фокус в поле
				if m.active == nil {
					m.flash = "у задачи нет чата"
					m.flashAt = m.api.now()
					return m, nil
				}
				m.replyActive()
				return m, m.reply.Focus()
			}
			// комментарий к задаче — это сообщение в её чат
			cmd := m.openTaskChat()
			if m.active == nil {
				return m, nil
			}
			m.replyActive()
			return m, tea.Batch(cmd, m.reply.Focus())
		}
	case key.Matches(msg, km.Select):
		if m.tab == tabChats && m.active != nil {
			m.enterSel()
			return m, nil
		}
	case key.Matches(msg, km.Filter):
		if m.tab == tabChats && m.active != nil {
			// открытый диалог: поиск по его истории, не фильтр списка
			m.focus = focusSearch
			return m, m.searchIn.Focus()
		}
		m.focus = focusFilter
		return m, m.filterIn.Focus()
	case key.Matches(msg, km.ReadAll):
		if m.tab == tabNotify {
			return m, m.readAllCmd()
		}
	case key.Matches(msg, km.MarkRead):
		if m.tab == tabNotify && m.focus == focusList {
			vn := m.visibleNotify()
			if m.cursor < 0 || m.cursor >= len(vn) {
				return m, nil
			}
			n := vn[m.cursor]
			unread := !n.Unread
			m.setNotifyUnread(n.ID, unread)
			return m, m.markNotifyCmd(n.ID, !unread)
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
			for i, s := range m.api.StagesOf(m.taskCard.GroupID) {
				if s.ID == m.taskCard.StageID {
					m.statusIdx = i // меню стадий открывается на текущей
				}
			}
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
			cmd := m.openTaskChat()
			return m, cmd
		}
	}

	// навигация по спискам
	switch {
	case key.Matches(msg, km.Down):
		if m.visibleCount() > 0 && m.cursor < m.visibleCount()-1 {
			m.cursor++
		}
		return m, tea.Batch(m.followCard(), m.followChat(), m.notifyMoreCmd())
	case key.Matches(msg, km.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, tea.Batch(m.followCard(), m.followChat())
	case key.Matches(msg, km.Top):
		m.cursor = 0
		return m, tea.Batch(m.followCard(), m.followChat())
	case key.Matches(msg, km.Bottom):
		m.clampCursors()
		m.cursor = m.visibleCount() - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, tea.Batch(m.followCard(), m.followChat(), m.notifyMoreCmd())
	case key.Matches(msg, km.Open):
		return m.enterKey()
	}

	// скролл истории, если диалог открыт и нет фокуса ввода; на уведах и
	// пульте скрытая история не скроллится — там свои клавиши
	if m.active != nil && (m.tab == tabChats || m.tab == tabTasks && m.taskOpen) {
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

// paste — вставка из терминала (bracketed paste, tea.PasteMsg): текст в поле
// под фокусом. CRLF нормализуем; в однострочные поля переводы строк — в пробел.
// При фокусе на списке вставке некуда попасть — игнорируем.
func (m Model) paste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	content := strings.ReplaceAll(msg.Content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	switch m.focus {
	case focusReply:
		var cmd tea.Cmd
		m.reply, cmd = m.reply.Update(tea.PasteMsg{Content: content})
		return m, cmd
	case focusFilter:
		var cmd tea.Cmd
		m.filterIn, cmd = m.filterIn.Update(tea.PasteMsg{Content: strings.ReplaceAll(content, "\n", " ")})
		m.filter = m.filterIn.Value()
		m.clampCursors()
		return m, cmd
	case focusSearch:
		var cmd tea.Cmd
		m.searchIn, cmd = m.searchIn.Update(tea.PasteMsg{Content: strings.ReplaceAll(content, "\n", " ")})
		return m, cmd
	case focusAttach:
		var cmd tea.Cmd
		m.attachIn, cmd = m.attachIn.Update(tea.PasteMsg{Content: strings.ReplaceAll(content, "\n", " ")})
		return m, cmd
	}
	return m, nil
}

// openTaskChat — чат открытой карточки; нет чата — флеш, active остаётся nil.
func (m *Model) openTaskChat() tea.Cmd {
	if m.taskCard.ChatID <= 0 {
		m.flash = "у задачи нет чата"
		m.flashAt = m.api.now()
		return nil
	}
	id := taskChatDialog(m.taskCard.ChatID)
	m.openChatByDialog(id)
	m.active.Title = m.taskCard.Title
	return m.historyCmd(id, 0)
}

// selKey — клавиши режима выделения сообщения (v): навигация и действия
// над выделенным; необработанные клавиши игнорируются.
func (m Model) selKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, km.Down):
		m.selStep(1)
		m.selTo(m.sel)
	case key.Matches(msg, km.Up):
		m.selStep(-1)
		m.selTo(m.sel)
	case key.Matches(msg, km.Bottom):
		for i := len(m.history) - 1; i >= 0; i-- {
			if !m.history[i].System {
				m.selTo(i)
				break
			}
		}
	case key.Matches(msg, km.Top):
		for i := range m.history {
			if !m.history[i].System {
				m.selTo(i)
				break
			}
		}
	case key.Matches(msg, km.Back), key.Matches(msg, km.Quit):
		m.selTo(-1)
	case key.Matches(msg, km.ReplyQuote):
		m.replyTo = m.history[m.sel]
		m.sel = -1
		m.replyActive()
		return m, m.reply.Focus()
	case key.Matches(msg, km.Copy):
		if m.history[m.sel].Text == "" {
			m.flash = "нечего копировать"
			m.flashAt = m.api.now()
			return m, nil
		}
		m.flash = "✓ скопировано"
		m.flashAt = m.api.now()
		return m, tea.SetClipboard(m.history[m.sel].Text)
	case key.Matches(msg, km.Edit):
		if !m.history[m.sel].Mine {
			m.flash = "можно только свои"
			m.flashAt = m.api.now()
			return m, nil
		}
		m.editID = m.history[m.sel].ID
		m.reply.SetValue(m.history[m.sel].Text)
		m.replyActive()
		return m, m.reply.Focus()
	case key.Matches(msg, km.Delete):
		if !m.history[m.sel].Mine {
			m.flash = "можно только свои"
			m.flashAt = m.api.now()
			return m, nil
		}
		m.delID = m.history[m.sel].ID
		m.overlay = ovConfirm
	case key.Matches(msg, km.Like):
		// оптимистичный лайк, ответ сервера только подтвердит
		msg0 := m.history[m.sel]
		msg0.LikedByMe = !msg0.LikedByMe
		if msg0.LikedByMe {
			msg0.Likes++
		} else if msg0.Likes > 0 {
			msg0.Likes--
		}
		m.history[m.sel] = msg0
		m.renderHistory()
		m.scrollToSel()
		return m, m.likeCmd(msg0.ID, msg0.LikedByMe)
	case key.Matches(msg, km.Mention):
		msg0 := m.history[m.sel]
		if msg0.Name == "" {
			return m, nil
		}
		m.reply.SetValue(m.reply.Value() + "@" + msg0.Name + " ")
		m.mentions[msg0.Name] = msg0.AuthorID
		m.replyActive()
		return m, m.reply.Focus()
	case key.Matches(msg, km.FileOpen):
		// вложение выделенного сообщения: картинка — оверлей, файл — вьювер
		return m.openAttach()
	}
	return m, nil
}

// switchTab — смена вкладки: фильтр вкладко-специфичен, сбрасываем;
// курсор клампится под новый список. Возврат на задачи перепривязывает
// чат задачи, если на чужой вкладке открыли свой диалог.
func (m *Model) switchTab(t tab) tea.Cmd {
	m.tab = t
	m.filter = ""
	m.filterIn.SetValue("")
	m.clampCursors()
	m.layout()
	if t == tabTasks {
		return m.syncTaskChat()
	}
	return nil
}

func (m Model) enterKey() (tea.Model, tea.Cmd) {
	switch m.tab {
	case tabChats:
		if m.active != nil {
			// уже в диалоге: enter на другом чате — мгновенное переключение
			// и фокус в его поле ответа
			vc := m.visibleChats()
			if m.cursor >= len(vc) || vc[m.cursor].DialogID == m.active.DialogID {
				return m, nil
			}
			m.saveDraft()
			m.openDialog(m.cursor)
			if m.active != nil {
				m.replyActive()
				return m, m.historyCmd(m.active.DialogID, 0)
			}
			return m, nil
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
			if n.URL != "" {
				// переход по ссылке уведа (календарь и пр.): открыть и прочитать
				m.flash = "открываю в браузере"
				m.flashAt = m.api.now()
				var cmds []tea.Cmd
				if n.Unread {
					m.clearNotifyUnread(n.ID)
					cmds = append(cmds, m.readNotifyCmd(n.ID))
				}
				cmds = append(cmds, openURLCmd(n.URL))
				return m, tea.Batch(cmds...)
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
	n := m.menuLen()
	if m.statusIdx <= 0 && up || m.statusIdx >= n-1 && !up {
		return m, nil
	}
	if up {
		m.statusIdx--
	} else {
		m.statusIdx++
	}
	return m, nil
}

// menuLen — пункты меню ovStatus: стадии проекта, если есть канбан,
// иначе статусы.
func (m Model) menuLen() int {
	if st := m.api.StagesOf(m.taskCard.GroupID); len(st) > 0 {
		return len(st)
	}
	return len(statusOptions(m.taskCard.Status))
}

// cardPage — страница прокрутки описания в карточке.
func (m Model) cardPage() int { return max(m.cardDescMax()-1, 1) }

func (m Model) statusConfirm() (tea.Model, tea.Cmd) {
	if st := m.api.StagesOf(m.taskCard.GroupID); len(st) > 0 {
		if m.statusIdx < 0 || m.statusIdx >= len(st) {
			return m, nil
		}
		m.overlay = ovNone
		if st[m.statusIdx].ID == m.taskCard.StageID {
			return m, nil // уже в этой стадии
		}
		return m.setStage(m.taskCard, m.statusIdx)
	}
	opts := statusOptions(m.taskCard.Status)
	if m.statusIdx < 0 || m.statusIdx >= len(opts) {
		return m, nil
	}
	return m, m.statusCmd(m.taskCard.ID, opts[m.statusIdx].code)
}
