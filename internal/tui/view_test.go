package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func viewModel(w, h int, chats []Chat, tasks []Task, notify []NotifyItem, active *Chat, msgs []Message) Model {
	m := NewModel(&API{}, nil)
	m.width, m.height = w, h
	m.ready = true
	m.chats = chats
	m.tasks = tasks
	m.notify = notify
	m.buckets = Buckets(tasks, time.UTC, time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC))
	m.active = active
	m.history = msgs
	m.layout()
	m.renderHistory()
	return m
}

func TestViewFitsScreen(t *testing.T) {
	chats := make([]Chat, 20)
	for i := range chats {
		chats[i] = Chat{DialogID: "c", Title: "Чат с длинным названием и ещё длиннее", Counter: i % 3, Preview: "превью сообщения тоже не короткое", Time: "10:39"}
	}
	tasks := make([]Task, 20)
	for i := range tasks {
		tasks[i] = Task{ID: int64(i + 1), Title: "Задача с достаточно длинным названием", Deadline: "2026-10-19 14:00:00"}
	}
	notify := make([]NotifyItem, 20)
	for i := range notify {
		notify[i] = NotifyItem{ID: "1", Title: "уведомление длинное-длинное", Text: "текст уведомления тоже длинный", Time: "10:39", Unread: true}
	}
	msgs := make([]Message, 40)
	for i := range msgs {
		msgs[i] = Message{ID: int64(i), Name: "Автор", Text: "сообщение средней длины", Time: "10:39", DayKey: "2026-10-19", DayLabel: "сегодня"}
	}

	for _, w := range []int{80, 120, 200} {
		for _, h := range []int{24, 30, 36, 50} {
			for _, tabix := range []tab{tabChats, tabTasks, tabNotify} {
				var active *Chat
				if tabix == tabChats {
					active = &chats[0]
				}
				m := viewModel(w, h, chats, tasks, notify, active, msgs)
				m.tab = tabix
				v := m.View().Content
				if got := lipgloss.Height(v); got > h {
					t.Errorf("w=%d h=%d tab=%d: высота %d > %d\n%q", w, h, tabix, got, h, firstLines(v, 3))
				}
				for i, line := range strings.Split(v, "\n") {
					if lw := lipgloss.Width(line); lw > w {
						t.Errorf("w=%d h=%d tab=%d: строка %d шире %d: %d", w, h, tabix, i, w, lw)
						break
					}
				}
			}
		}
	}
}

func TestStatusOptions(t *testing.T) {
	open := statusOptions("2")
	for _, o := range open {
		if o.code == "2" {
			t.Error("текущий статус не должен быть в списке")
		}
	}
	closed := statusOptions("5")
	if closed[0].name != "переоткрыть" {
		t.Errorf("для закрытой первый пункт: %+v", closed[0])
	}
}

func TestTruncateWide(t *testing.T) {
	s := truncate("🏠 КЛЕВЕР ХАБ 🟠 ЦИАН новое сообщение", 20)
	if w := lipgloss.Width(s); w > 20 {
		t.Fatalf("ширина %d > 20 ячеек: %q", w, s)
	}
	if !strings.HasSuffix(s, "…") {
		t.Fatalf("нет эллипсиса: %q", s)
	}
}

func TestViewFitsEmoji(t *testing.T) {
	chats := make([]Chat, 6)
	for i := range chats {
		chats[i] = Chat{DialogID: fmt.Sprintf("d%d", i),
			Title: "🏠 КЛЕВЕР ХАБ 🟠 ЦИАН", Preview: "🟠 новое сообщение в чате", Time: "14:15", Counter: 1}
	}
	m := viewModel(120, 36, chats, nil, nil, nil, nil)
	m.tab = tabChats
	for i, line := range strings.Split(m.View().Content, "\n") {
		if lw := lipgloss.Width(line); lw > 120 {
			t.Fatalf("строка %d шире 120: %d", i, lw)
		}
	}
}

func TestHistoryLoading(t *testing.T) {
	m := viewModel(120, 36, nil, nil, nil, nil, nil)
	m.historyLoading = true
	m.renderHistory()
	if v := m.vp.View(); !strings.Contains(v, "загрузка…") {
		t.Fatalf("пустая история при загрузке без «загрузка…»:\n%q", v)
	}
	// ошибка первого запроса тоже снимает флаг
	mm, _ := m.Update(historyMsg{err: errors.New("boom")})
	if m2 := mm.(Model); m2.historyLoading {
		t.Fatal("historyMsg с ошибкой не снял historyLoading")
	}
}

func TestNoMoreMarker(t *testing.T) {
	msgs := []Message{{ID: 1, Name: "А", Text: "привет", Time: "10:00", DayKey: "d", DayLabel: "сегодня"}}
	m := viewModel(120, 36, nil, nil, nil, nil, msgs)
	m.noMore = true
	m.renderHistory()
	v := m.vp.View()
	if i, j := strings.Index(v, "начало переписки"), strings.Index(v, "привет"); i < 0 || i > j {
		t.Fatalf("«начало переписки» не первой строкой:\n%s", v)
	}
	// H при noMore — флеш вместо повторного запроса
	m.active = &Chat{DialogID: "d"}
	m2, _ := keyPress(m, "H")
	if m2.flash != "это начало переписки" {
		t.Fatalf("H при noMore без флеша: %q", m2.flash)
	}
}

func firstLines(s string, n int) string {
	parts := strings.Split(s, "\n")
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, "\n")
}

// keyPress — механический перевод v1-хелпера (KeyRunes+[]rune(key)):
// клавиша, чьё String() равно key. Для «enter» это руны, а не спец-код
// (как и в v1 — Type-свитчи оверлеев её не ловили).
func keyPress(m Model, key string) (Model, tea.Cmd) {
	msg := tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	mm, cmd := m.Update(msg)
	return mm.(Model), cmd
}

func TestStatusOverlayKeys(t *testing.T) {
	m := viewModel(120, 36, nil, []Task{{ID: 1, Title: "x", Status: "2"}}, nil, nil, nil)
	m.tab = tabTasks
	m.taskOpen = true
	m.taskCard = Task{ID: 1, Status: "2"}
	m2, _ := keyPress(m, "s")
	if m2.overlay != ovStatus {
		t.Fatalf("s не открыл меню: %v", m2.overlay)
	}
	v := m2.View().Content
	if !strings.Contains(v, "в работе") {
		t.Fatalf("меню статусов не в кадре:\n%s", v)
	}
	m3, _ := keyPress(m2, "j")
	m4, _ := keyPress(m3, "enter")
	if m4.overlay != ovStatus {
		t.Fatalf("до ответа сервера overlay держим: %v", m4.overlay)
	}
	// сервер ответил успехом — overlay закрылся
	m5, _ := m4.Update(statusMsg{})
	mm := m5.(Model)
	if mm.overlay != ovNone {
		t.Fatalf("после ответа overlay не закрыт: %v", mm.overlay)
	}
}

func TestDeadlineOverlayAndTaskChat(t *testing.T) {
	m := viewModel(120, 36, nil, []Task{{ID: 1, Title: "x", Status: "2", ChatID: 7}}, nil, nil, nil)
	m.tab = tabTasks
	m.taskOpen = true
	m.taskCard = Task{ID: 1, Status: "2", ChatID: 7, Deadline: "2026-10-21T10:30:00+05:00"}
	m2, _ := keyPress(m, "d")
	if m2.overlay != ovDeadline {
		t.Fatalf("d не открыл меню срока: %v", m2.overlay)
	}
	if v := m2.View().Content; !strings.Contains(v, "сегодня 18:00") || !strings.Contains(v, "снять дедлайн") {
		t.Fatalf("меню срока не в кадре")
	}
	m3, _ := keyPress(m2, "enter") // сегодня 18:00
	if m3.overlay != ovDeadline {
		t.Fatalf("до ответа сервера overlay держим: %v", m3.overlay)
	}
	m4, _ := m3.Update(deadlineMsg{})
	if mm := m4.(Model); mm.overlay != ovNone {
		t.Fatalf("после ответа overlay не закрыт: %v", mm.overlay)
	}
	// c — чат задачи
	m5, _ := keyPress(m, "c")
	if m5.active == nil || m5.active.DialogID != "chat7" {
		t.Fatalf("c не открыл чат задачи: %+v", m5.active)
	}
	// задача без чата
	m6 := m
	m6.taskCard.ChatID = 0
	m7, _ := keyPress(m6, "c")
	if m7.active != nil {
		t.Fatalf("чат открылся у задачи без чата")
	}
}

func TestTaskCardClamp(t *testing.T) {
	long := strings.Repeat("строка описания\n", 300)
	m := viewModel(120, 36, nil, nil, nil, nil, nil)
	m.tab = tabTasks
	m.taskOpen = true
	m.taskCard = Task{ID: 1, Title: "x", Status: "2", ChatID: 7, Description: long}
	m.overlay = ovDeadline
	v := m.View().Content
	if got := strings.Count(v, "\n") + 1; got > 36 {
		t.Fatalf("View=%d строк при экране 36 — меню под фолдом", got)
	}
	if !strings.Contains(v, "сегодня 18:00") {
		t.Fatalf("меню срока не в кадре после клампа")
	}
}

func TestMouseClickGeometry(t *testing.T) {
	chats := make([]Chat, 8)
	for i := range chats {
		chats[i] = Chat{DialogID: fmt.Sprintf("d%d", i), Title: fmt.Sprintf("чат %d", i), Time: "10:39"}
	}
	m := viewModel(120, 36, chats, nil, nil, nil, nil)
	m.tab = tabChats

	// строка 0 — верхняя грань рамки, 1-2 — первый чат, 3-4 — второй
	mm, _ := m.Update(tea.MouseClickMsg{X: 10, Y: 1, Button: tea.MouseLeft})
	m1 := mm.(Model)
	if m1.cursor != 0 || m1.active == nil || m1.active.DialogID != "d0" {
		t.Fatalf("клик по строке 1: cursor=%d active=%v", m1.cursor, m1.active)
	}
	// второй чат при открытом диалоге — переключение по клику
	m2, _ := m1.Update(tea.MouseClickMsg{X: 10, Y: 3, Button: tea.MouseLeft})
	m3 := m2.(Model)
	if m3.active == nil || m3.active.DialogID != "d1" {
		t.Fatalf("клик по строке 3 не переключил диалог: %+v", m3.active)
	}
	// вторая строка чата (превью) — тот же чат
	m4, _ := m3.Update(tea.MouseClickMsg{X: 10, Y: 4, Button: tea.MouseLeft})
	if got := m4.(Model).active.DialogID; got != "d1" {
		t.Fatalf("клик по превью открыл %s, хочу d1", got)
	}
	// правее списка — ничего
	m5, _ := m4.(Model).Update(tea.MouseClickMsg{X: m4.(Model).listW + 5, Y: 1, Button: tea.MouseLeft})
	if got := m5.(Model).active.DialogID; got != "d1" {
		t.Fatalf("клик мимо списка переключил на %s", got)
	}
	// фильтр сдвигает сетку на 2 строки
	m6 := m4.(Model)
	m6.filter = "чат"
	m6.focus = focusList
	vc := m6.visibleChats()
	if len(vc) != 8 {
		t.Fatalf("фильтр «чат» должен пропускать все: %d", len(vc))
	}
	m7, _ := m6.Update(tea.MouseClickMsg{X: 10, Y: 5, Button: tea.MouseLeft})
	if got := m7.(Model).active.DialogID; got != "d1" {
		t.Fatalf("клик с фильтром (Y=5 → второй чат): открылся %s", got)
	}
}

func TestMouseWheel(t *testing.T) {
	chats := make([]Chat, 30)
	for i := range chats {
		chats[i] = Chat{DialogID: fmt.Sprintf("d%d", i), Title: fmt.Sprintf("чат %d", i)}
	}
	msgs := make([]Message, 60)
	for i := range msgs {
		msgs[i] = Message{ID: int64(i), Name: "A", Text: "текст", Time: "10:39", DayKey: "2026-10-19", DayLabel: "сегодня"}
	}
	m := viewModel(120, 36, chats, nil, nil, &chats[0], msgs)
	m.tab = tabChats
	m.active = &chats[0]
	// колесо над диалогом — скролл вниз
	mm, _ := m.Update(tea.MouseWheelMsg{X: 100, Y: 10, Button: tea.MouseWheelDown})
	m1 := mm.(Model)
	if m1.vp.YOffset() == 0 {
		t.Fatal("колесо над диалогом не скроллит")
	}
	// колесо над списком — курсор
	m2, _ := m1.Update(tea.MouseWheelMsg{X: 5, Y: 10, Button: tea.MouseWheelDown})
	if got := m2.(Model).cursor; got != 1 {
		t.Fatalf("колесо над списком: cursor=%d, хочу 1", got)
	}
}

func TestHistoryGrouping(t *testing.T) {
	msgs := []Message{
		{ID: 1, AuthorID: 1, Name: "Анна", Text: "первое", Time: "10:00", DayKey: "d", DayLabel: "сегодня"},
		{ID: 2, AuthorID: 1, Name: "Анна", Text: "второе", Time: "10:01", DayKey: "d", DayLabel: "сегодня"},
		{ID: 3, AuthorID: 2, Name: "Борис", Text: "ответ", Time: "10:02", DayKey: "d", DayLabel: "сегодня"},
		{ID: 4, AuthorID: 7, Name: "Я", Text: "моё", Time: "10:03", DayKey: "d", DayLabel: "сегодня", Mine: true},
	}
	m := viewModel(120, 36, nil, nil, nil, nil, msgs)
	m.renderHistory()
	v := m.vp.View()
	if got := strings.Count(v, "Анна"); got != 1 {
		t.Fatalf("имя Анны в кадре %d раз (группировка сломана):\n%s", got, v)
	}
	if !strings.Contains(v, "первое") || !strings.Contains(v, "второе") || !strings.Contains(v, "ответ") {
		t.Fatalf("текст потерялся:\n%s", v)
	}
	if !strings.Contains(v, "›") {
		t.Fatalf("нет маркера своего сообщения:\n%s", v)
	}
	// разделитель дня по центру: начинается и кончается тире (паддинг
	// вьюпорта справа не считаем)
	// v2 lipgloss Render всегда эмитит ANSI; в v1 тесты видели ASCII,
	// снимаем коды, чтобы сравнивать сам разделитель
	dayLine := ""
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "сегодня") {
			dayLine = strings.TrimRight(ansi.Strip(l), " ")
			break
		}
	}
	if !strings.HasPrefix(dayLine, "─") || !strings.HasSuffix(dayLine, "─") {
		t.Fatalf("разделитель дня не по центру: %q", dayLine)
	}
}

func TestMouseClickCardGateAndTabClamp(t *testing.T) {
	tasks := make([]Task, 3)
	for i := range tasks {
		tasks[i] = Task{ID: int64(i + 1), Title: fmt.Sprintf("задача %d", i+1)}
	}
	notify := make([]NotifyItem, 12)
	for i := range notify {
		notify[i] = NotifyItem{ID: fmt.Sprintf("n%d", i), Title: fmt.Sprintf("увед %d", i), Time: "10:39"}
	}
	m := viewModel(120, 36, nil, tasks, notify, nil, nil)
	m.tab = tabTasks
	m.taskOpen = true
	m.taskCard = tasks[0]

	// клик по правой панели (карточке) — не выбирает задачу
	mm, _ := m.Update(tea.MouseClickMsg{X: m.listW + 10, Y: 3, Button: tea.MouseLeft})
	if got := mm.(Model).cursor; got != 0 {
		t.Fatalf("клик по карточке сдвинул курсор на %d", got)
	}
	// клик по списку слева — выбирает (строка 4 = первая строка задачи 1:
	// 0 — секция, 1-2 — задача 0, 3-4 — задача 1)
	mm, _ = mm.(Model).Update(tea.MouseClickMsg{X: 5, Y: 4, Button: tea.MouseLeft})
	if got := mm.(Model).cursor; got != 1 {
		t.Fatalf("клик по списку: cursor=%d, хочу 1", got)
	}
	// курсор 11 на уведах → вкладка чатов (0 чатов... берём 1 чат) — кламп
	m2 := mm.(Model)
	m2.tab = tabNotify
	m2.cursor = 11
	m3, _ := m2.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if got := m3.(Model).cursor; got != 0 {
		t.Fatalf("переключение вкладки не клампит курсор: %d", got)
	}
}

func TestReplySendKeepsFocusAndTyped(t *testing.T) {
	active := &Chat{DialogID: "chat1"}

	// enter: текст уходит сразу, поле чистое, фокус и sending на месте
	m := viewModel(120, 36, nil, nil, nil, active, nil)
	m.replyActive()
	m.reply.SetValue("привет")
	mm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m1 := mm.(Model)
	if m1.reply.Value() != "" || !m1.sending || m1.focus != focusReply || cmd == nil {
		t.Fatalf("enter: value=%q sending=%v focus=%v cmd=%v",
			m1.reply.Value(), m1.sending, m1.focus, cmd)
	}

	// ответ пришёл: допечатанное «ещё» не стёрто, из поля не выкинуло
	m1.reply.SetValue("ещё")
	mm2, _ := m1.Update(sentMsg{dialogID: "chat1", id: 5, text: "привет"})
	m2 := mm2.(Model)
	if m2.reply.Value() != "ещё" || m2.focus != focusReply {
		t.Fatalf("sentMsg: value=%q focus=%v", m2.reply.Value(), m2.focus)
	}
	if n := len(m2.history); n == 0 || m2.history[n-1].Text != "привет" {
		t.Fatalf("sentMsg: последнее сообщение: %+v", m2.history)
	}

	// ошибка отправки: текст возвращается в пустое поле
	m3 := viewModel(120, 36, nil, nil, nil, active, nil)
	m3.replyActive()
	mm4, _ := m3.Update(sentMsg{dialogID: "chat1", err: errors.New("x"), text: "привет"})
	if got := mm4.(Model).reply.Value(); got != "привет" {
		t.Fatalf("ошибка отправки: value=%q, хочу «привет»", got)
	}

	// ctrl+c в поле — как esc: черновик сохранён, фокус в списке
	m5 := viewModel(120, 36, nil, nil, nil, active, nil)
	m5.replyActive()
	m5.reply.SetValue("черн")
	mm6, _ := m5.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m6 := mm6.(Model)
	if m6.focus != focusList || m6.drafts["chat1"] != "черн" {
		t.Fatalf("ctrl+c: focus=%v draft=%q", m6.focus, m6.drafts["chat1"])
	}

	// e) успех, но юзер уже в другом чате: чужую историю не трогаем
	m7 := viewModel(120, 36, nil, nil, nil, active, nil)
	m7.replyActive()
	m7.reply.SetValue("привет")
	mm8, _ := m7.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m8 := mm8.(Model)
	m8.active = &Chat{DialogID: "chat2"}
	mm9, _ := m8.Update(sentMsg{dialogID: "chat1", id: 5, text: "привет"})
	m9 := mm9.(Model)
	if len(m9.history) != 0 {
		t.Fatalf("чужому чату дописали сообщение: %+v", m9.history)
	}

	// f) ошибка при уходе в другой чат: текст — черновиком chat1, поле chat2 цело
	m10 := viewModel(120, 36, nil, nil, nil, active, nil)
	m10.replyActive()
	m10.reply.SetValue("привет")
	mm11, _ := m10.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m11 := mm11.(Model)
	m11.active = &Chat{DialogID: "chat2"}
	m11.reply.SetValue("другое")
	m11.focus = focusList
	mm12, _ := m11.Update(sentMsg{dialogID: "chat1", err: errors.New("x"), text: "привет"})
	m12 := mm12.(Model)
	if m12.drafts["chat1"] != "привет" || m12.reply.Value() != "другое" || m12.focus != focusList {
		t.Fatalf("ошибка при смене чата: draft=%q value=%q focus=%v",
			m12.drafts["chat1"], m12.reply.Value(), m12.focus)
	}

	// g) диалог закрыли во время отправки: успех без active — без паники
	m13 := viewModel(120, 36, nil, nil, nil, active, nil)
	m13.replyActive()
	m13.reply.SetValue("привет")
	mm14, _ := m13.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m14 := mm14.(Model)
	m14.active = nil
	mm15, _ := m14.Update(sentMsg{dialogID: "chat1", id: 5, text: "привет"})
	if len(mm15.(Model).history) != 0 {
		t.Fatal("успех без active дописал историю")
	}
}

func TestHelpOverlayAnyKey(t *testing.T) {
	m := viewModel(120, 36, nil, nil, nil, nil, nil)
	m2, _ := keyPress(m, "?")
	if m2.overlay != ovHelp {
		t.Fatalf("? не открыл справку: %v", m2.overlay)
	}
	v := m2.View().Content
	// полная карта: колонки всех групп, включая общий ctrl+c и shift+tab
	for _, want := range []string{"shift+tab", "ctrl+c", "хоткеи", "всё прочитано"} {
		if !strings.Contains(v, want) {
			t.Fatalf("в полной карте нет %q:\n%s", want, v)
		}
	}
	for i, line := range strings.Split(v, "\n") {
		if lw := lipgloss.Width(line); lw > 120 {
			t.Fatalf("строка справки %d шире 120: %d", i, lw)
		}
	}
	// любая клавиша закрывает
	m3, _ := keyPress(m2, "x")
	if m3.overlay != ovNone {
		t.Fatalf("x не закрыл справку: %v", m3.overlay)
	}
}

func TestTabCycleKeys(t *testing.T) {
	m := viewModel(120, 36, nil, nil, nil, nil, nil)
	m.tab = tabChats
	mm, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := mm.(Model).tab; got != tabTasks {
		t.Fatalf("tab: вкладка %v, хочу задачи", got)
	}
	mm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if got := mm.(Model).tab; got != tabNotify {
		t.Fatalf("shift+tab: вкладка %v, хочу уведы", got)
	}
	// смена вкладки сбрасывает фильтр
	m3 := viewModel(120, 36, []Chat{{DialogID: "d", Title: "x"}}, nil, nil, nil, nil)
	m3.tab = tabChats
	m3.filter = "x"
	m3.filterIn.SetValue("x")
	mm, _ = m3.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m4 := mm.(Model)
	if m4.filter != "" || m4.filterIn.Value() != "" {
		t.Fatalf("смена вкладки не сбросила фильтр: %q / %q", m4.filter, m4.filterIn.Value())
	}
}

func TestBackFromCard(t *testing.T) {
	m := viewModel(120, 36, nil, []Task{{ID: 1, Title: "x"}}, nil, nil, nil)
	m.tab = tabTasks
	m.taskOpen = true
	m.taskCard = Task{ID: 1, Title: "x"}
	m.cardReq = 1
	m.active = &Chat{DialogID: "chat1"} // диалог остался с вкладки чатов
	mm, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m2 := mm.(Model)
	if m2.taskOpen || m2.cardReq != 0 {
		t.Fatalf("q не закрыл карточку: taskOpen=%v cardReq=%d", m2.taskOpen, m2.cardReq)
	}
	if m2.active == nil || m2.active.DialogID != "chat1" {
		t.Fatalf("q тронул диалог: %+v", m2.active)
	}
	if cmd != nil {
		t.Fatal("q с открытой карточкой не должен выходить из программы")
	}
}

func TestFooterTaskCardHints(t *testing.T) {
	m := viewModel(120, 36, nil, []Task{{ID: 1, Title: "x"}}, nil, nil, nil)
	m.tab = tabTasks
	m.taskOpen = true
	m.taskCard = Task{ID: 1, Title: "x", ChatID: 7}
	f := m.footer()
	for _, want := range []string{"статус", "срок"} {
		if !strings.Contains(f, want) {
			t.Fatalf("футер карточки без %q:\n%s", want, f)
		}
	}
}

func TestFilterAnyTab(t *testing.T) {
	m := viewModel(120, 36, nil, []Task{{ID: 1, Title: "x"}}, nil, nil, nil)
	m.tab = tabTasks
	m2, _ := keyPress(m, "/")
	if m2.focus != focusFilter {
		t.Fatalf("/ не включил фильтр на задачах: %v", m2.focus)
	}
}

// Бейдж новых комментариев: число справа в строке задачи, строка в ширину.
func TestTaskBadgeInView(t *testing.T) {
	tasks := []Task{{ID: 1, Title: "задача с бейджем", NewComments: 4}}
	m := viewModel(120, 36, nil, tasks, nil, nil, nil)
	m.tab = tabTasks
	v := m.View().Content
	var badgeLine string
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(ansi.Strip(l), "задача с бейджем") {
			badgeLine = l
			break
		}
	}
	if badgeLine == "" {
		t.Fatal("строки задачи нет в кадре")
	}
	if !strings.Contains(ansi.Strip(badgeLine), "4") {
		t.Fatalf("бейджа «4» нет в строке задачи: %q", badgeLine)
	}
	for i, line := range strings.Split(v, "\n") {
		if lw := lipgloss.Width(line); lw > 120 {
			t.Fatalf("строка %d шире 120: %d", i, lw)
		}
	}
}

func TestFlatTasksFilter(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "Обновить фид", Deadline: "2026-10-18 10:00:00"},
		{ID: 2, Title: "Отчёт Roistat", Deadline: "2026-10-19 15:00:00"},
	}
	m := viewModel(120, 36, nil, tasks, nil, nil, nil)
	m.tab = tabTasks
	m.filter = "фид"
	rows := m.flatTasks()
	if len(rows) != 1 || rows[0].task.ID != 1 {
		t.Fatalf("фильтр «фид»: %d строк, хочу только задачу 1", len(rows))
	}
	m.filter = ""
	if n := len(m.flatTasks()); n != 2 {
		t.Fatalf("пустой фильтр: %d строк, хочу 2", n)
	}
}

func TestVisibleNotifyFilter(t *testing.T) {
	notify := []NotifyItem{
		{ID: "1", Title: "A", Text: "про фид"},
		{ID: "2", Title: "B"},
	}
	m := viewModel(120, 36, nil, nil, notify, nil, nil)
	m.tab = tabNotify
	m.filter = "фид"
	vn := m.visibleNotify()
	if len(vn) != 1 || vn[0].ID != "1" {
		t.Fatalf("фильтр «фид»: %d уведов, хочу только ID 1", len(vn))
	}
	if m.visibleCount() != 1 {
		t.Fatalf("visibleCount с фильтром: %d, хочу 1", m.visibleCount())
	}
	m.filter = ""
	if n := len(m.visibleNotify()); n != 2 {
		t.Fatalf("пустой фильтр: %d уведов, хочу 2", n)
	}
}

func TestFilterRowInView(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "Обновить фид", Deadline: "2026-10-18 10:00:00"},
		{ID: 2, Title: "Отчёт Roistat", Deadline: "2026-10-19 15:00:00"},
	}
	m := viewModel(120, 36, nil, tasks, nil, nil, nil)
	m.tab = tabTasks
	m.filter = "фид"
	v := m.View().Content
	if !strings.Contains(v, "фильтр: фид") {
		t.Fatalf("строки фильтра нет в панели:\n%s", v)
	}
	// опустевшая корзина «сегодня» — без заголовка
	if strings.Contains(v, "СЕГОДНЯ") {
		t.Fatalf("заголовок пустой корзины:\n%s", v)
	}
	for i, line := range strings.Split(v, "\n") {
		if lw := lipgloss.Width(line); lw > 120 {
			t.Fatalf("строка %d шире 120: %d", i, lw)
		}
	}
}

func TestNotifyFilterEnterClearsUnread(t *testing.T) {
	notify := []NotifyItem{
		{ID: "1", Title: "B", TaskID: 8, Unread: true},
		{ID: "2", Title: "A", Text: "про фид", TaskID: 7, Unread: true},
	}
	m := viewModel(120, 36, nil, nil, notify, nil, nil)
	m.tab = tabNotify
	m.filter = "фид"
	if len(m.visibleNotify()) != 1 || m.visibleNotify()[0].ID != "2" {
		t.Fatalf("фильтр: %d уведов, хочу ID 2", len(m.visibleNotify()))
	}
	m1, _ := keyPress(m, "enter")
	if m1.cardReq != 7 {
		t.Fatalf("enter взял не тот элемент: cardReq=%d, хочу 7", m1.cardReq)
	}
	// Unread гасим в исходном списке по ID, сосед не тронут
	if m1.notify[1].Unread || !m1.notify[0].Unread {
		t.Fatalf("Unread: %+v", m1.notify)
	}
}

func TestBrowserKey(t *testing.T) {
	tasks := []Task{{ID: 1, Title: "a"}, {ID: 2, Title: "b"}}
	m := viewModel(120, 36, nil, tasks, nil, nil, nil)
	m.tab = tabTasks
	m2, cmd := keyPress(m, "o")
	if cmd != nil || m2.flash != "нечего открыть в браузере" {
		t.Fatalf("без портала: cmd=%v flash=%q", cmd != nil, m2.flash)
	}
	m.api.Portal = "https://x.bitrix24.ru"
	if _, cmd := keyPress(m, "o"); cmd == nil {
		t.Fatal("o на задаче не дал команду открытия")
	}
}

func TestFollowOnNav(t *testing.T) {
	tasks := []Task{{ID: 1, Title: "a"}, {ID: 2, Title: "b"}}
	m := viewModel(120, 36, nil, tasks, nil, nil, nil)
	m.tab = tabTasks
	m.taskOpen = true
	m.taskCard = m.flatTasks()[0].task
	// уход с открытой задачи — тик карточки; возврат на неё — запрос не нужен
	for _, step := range []struct {
		k      string
		follow bool
	}{{"j", true}, {"k", false}, {"G", true}, {"g", false}} {
		before := m.followSeq
		var cmd tea.Cmd
		m, cmd = keyPress(m, step.k)
		if got := cmd != nil && m.followSeq == before+1; got != step.follow {
			t.Fatalf("%s: follow=%v, ждали %v", step.k, got, step.follow)
		}
	}
}

func TestBrowserURLFilteredNotify(t *testing.T) {
	notify := []NotifyItem{{ID: "1", Title: "без задачи"}, {ID: "2", Title: "фид", TaskID: 9}}
	m := viewModel(120, 36, nil, nil, notify, nil, nil)
	m.api.Portal, m.api.My = "https://x.bitrix24.ru", 7
	m.tab = tabNotify
	m.filter = "фид"
	if got := m.browserURL(); !strings.HasSuffix(got, "/task/view/9/") {
		t.Fatalf("курсор на отфильтрованном уведе: %q", got)
	}
}

func TestReplyFromTaskCard(t *testing.T) {
	m := viewModel(120, 36, nil, []Task{{ID: 1, Title: "x", Status: "2", ChatID: 7}}, nil, nil, nil)
	m.tab = tabTasks
	m.taskOpen = true
	m.taskCard = Task{ID: 1, Title: "x", Status: "2", ChatID: 7}
	m2, _ := keyPress(m, "r")
	if m2.active == nil || m2.active.DialogID != "chat7" || m2.focus != focusReply {
		t.Fatalf("r на карточке: чат задачи с полем ответа, got active=%+v focus=%v", m2.active, m2.focus)
	}
	m.taskCard.ChatID = 0
	m3, _ := keyPress(m, "r")
	if m3.active != nil || m3.focus != focusList || m3.flash == "" {
		t.Fatalf("r без чата задачи: флеш, без перехода; got active=%+v flash=%q", m3.active, m3.flash)
	}
}
