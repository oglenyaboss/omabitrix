package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"omabitrix/internal/b24"
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

// / в открытом диалоге — фокус поиска; без диалога и на других вкладках — фильтр.
func TestSearchFocusRouting(t *testing.T) {
	chats := []Chat{{DialogID: "chat7", ChatID: 7, Title: "Анна"}}
	active := chats[0]
	m := viewModel(120, 36, chats, nil, nil, &active, nil)
	m2, _ := keyPress(m, "/")
	if m2.focus != focusSearch {
		t.Fatalf("/ в диалоге: focus=%v, хочу focusSearch", m2.focus)
	}
	m3 := viewModel(120, 36, chats, nil, nil, nil, nil)
	m4, _ := keyPress(m3, "/")
	if m4.focus != focusFilter {
		t.Fatalf("/ без диалога: focus=%v, хочу focusFilter", m4.focus)
	}
	m5 := viewModel(120, 36, chats, nil, nil, &active, nil)
	m5.tab = tabTasks
	m6, _ := keyPress(m5, "/")
	if m6.focus != focusFilter {
		t.Fatalf("/ на задачах: focus=%v, хочу focusFilter", m6.focus)
	}
}

// Enter в поиске шлёт im.dialog.messages.search с числовым CHAT_ID,
// ответ рендерится в панели: имя, текст, заголовок со счётчиком.
func TestSearchEnterAndResults(t *testing.T) {
	var mu sync.Mutex
	var params map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/im.dialog.messages.search.json") {
			mu.Lock()
			params = nil
			json.NewDecoder(r.Body).Decode(&params)
			mu.Unlock()
			fmt.Fprint(w, `{"result":{"messages":[{"id":5,"authorId":9,`+
				`"date":"2026-10-19T10:00:00+03:00","text":"нашлось про проверка"}],`+
				`"users":[{"id":9,"name":"Анна"}]}}`)
			return
		}
		fmt.Fprint(w, `{"result":{}}`)
	}))
	defer srv.Close()

	chats := []Chat{{DialogID: "chat7", ChatID: 7, Title: "Анна"}}
	active := chats[0]
	m := viewModel(120, 36, chats, nil, nil, &active, nil)
	m.api = &API{C: b24.New(srv.URL), My: 75}
	m.ctx = context.Background()

	m2, _ := keyPress(m, "/")
	if m2.focus != focusSearch {
		t.Fatalf("/ в диалоге: focus=%v, хочу focusSearch", m2.focus)
	}
	m2.searchIn.SetValue("проверка")
	mm, cmd := m2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m3 := mm.(Model)
	if !m3.searchOn || !m3.searching || m3.focus != focusList || cmd == nil {
		t.Fatalf("enter: searchOn=%v searching=%v focus=%v cmd=%v",
			m3.searchOn, m3.searching, m3.focus, cmd)
	}
	msg, ok := cmd().(searchMsg)
	if !ok || msg.err != nil {
		t.Fatalf("cmd вернул %T %+v", msg, msg)
	}
	mu.Lock()
	if id, _ := params["CHAT_ID"].(float64); int64(id) != 7 {
		t.Errorf("CHAT_ID: %v, хочу 7", params["CHAT_ID"])
	}
	if q, _ := params["SEARCH_MESSAGE"].(string); q != "проверка" {
		t.Errorf("SEARCH_MESSAGE: %v", params["SEARCH_MESSAGE"])
	}
	mu.Unlock()

	mm2, _ := m3.Update(msg)
	m4 := mm2.(Model)
	if len(m4.searchRes) != 1 || m4.searching {
		t.Fatalf("результат не применён: res=%d searching=%v", len(m4.searchRes), m4.searching)
	}
	v := m4.View().Content
	if !strings.Contains(v, "Анна") || !strings.Contains(ansi.Strip(v), "нашлось про проверка") {
		t.Fatalf("результат не в кадре:\n%s", v)
	}
	if !strings.Contains(v, "поиск: проверка · 1") {
		t.Fatalf("заголовок поиска не в кадре:\n%s", v)
	}
	for i, line := range strings.Split(v, "\n") {
		if lw := lipgloss.Width(line); lw > 120 {
			t.Fatalf("строка %d шире 120: %d", i, lw)
		}
	}
	// скроллбар — ровной колонкой: у всех строк панели результатов с
	// колонкой ширина префикса до неё одинакова
	got := -1
	for i, line := range strings.Split(m4.searchPane(), "\n") {
		s := ansi.Strip(line)
		if len(s) < 3 || !strings.HasPrefix(s, "│") {
			continue // грани рамки
		}
		inner := s[2 : len(s)-2]
		if !strings.Contains(inner, "│") {
			continue // строка без колонки (подсказка)
		}
		pos := strings.LastIndex(inner, "│")
		w := lipgloss.Width(s[:2+pos+1]) // префикс до колонки включительно
		if got < 0 {
			got = w
		} else if w != got {
			t.Fatalf("скроллбар плавает: строка %d col %d, хочу %d", i, w, got)
		}
	}
	if got < 0 {
		t.Fatal("скроллбара не видно в панели результатов")
	}
	// r поверх результатов не действует: фокус остаётся в списке
	m5, cmd := keyPress(m4, "r")
	if m5.focus != focusList || cmd != nil {
		t.Fatalf("r в результатах: focus=%v cmd=%v", m5.focus, cmd)
	}
}

// esc/q в результатах — назад к истории, диалог не закрывается.
func TestSearchEscBack(t *testing.T) {
	chats := []Chat{{DialogID: "chat7", ChatID: 7, Title: "Анна"}}
	active := chats[0]
	hist := []Message{{ID: 1, Name: "Анна", Text: "история диалога", Time: "10:00", DayKey: "d", DayLabel: "сегодня"}}
	m := viewModel(120, 36, chats, nil, nil, &active, hist)
	m.searchOn = true
	m.searchQ = "запрос"
	m.searchRes = []Message{{ID: 2, AuthorID: 9, Name: "Анна", Text: "находка запрос", Time: "10:05", DayKey: "d", DayLabel: "сегодня"}}

	mm, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m2 := mm.(Model)
	if m2.searchOn || m2.active == nil || m2.active.DialogID != "chat7" {
		t.Fatalf("esc: searchOn=%v active=%v", m2.searchOn, m2.active)
	}
	v := m2.View().Content
	if strings.Contains(v, "поиск:") || !strings.Contains(v, "история диалога") {
		t.Fatalf("esc не вернул историю:\n%s", v)
	}
	// q — тоже назад, а не выход из программы
	m4, _ := keyPress(m, "q")
	if m4.searchOn || m4.active == nil {
		t.Fatalf("q: searchOn=%v active=%v", m4.searchOn, m4.active)
	}
}

// j/k/pgup/pgdn листают окно результатов с клампами по краям.
func TestSearchScroll(t *testing.T) {
	res := make([]Message, 30)
	for i := range res {
		res[i] = Message{ID: int64(i), AuthorID: 9, Name: "Анна",
			Text: fmt.Sprintf("нашлось совпадение номер %d", i), Time: "10:00", DayKey: "d", DayLabel: "сегодня"}
	}
	m := viewModel(120, 36, nil, nil, nil, &Chat{DialogID: "chat7", ChatID: 7}, nil)
	m.searchOn = true
	m.searchQ = "нашлось"
	m.searchRes = res
	if m.searchMaxOff() == 0 {
		t.Fatal("все результаты влезли — прокрутку не проверить")
	}
	m2, _ := keyPress(m, "j")
	if m2.searchOff != 2 {
		t.Fatalf("j: off=%d, хочу 2", m2.searchOff)
	}
	m3, _ := keyPress(m2, "f")
	if m3.searchOff != m3.searchMaxOff() {
		t.Fatalf("f: off=%d, хочу кламп %d", m3.searchOff, m3.searchMaxOff())
	}
	m4, _ := keyPress(m3, "k")
	if m4.searchOff != m3.searchMaxOff()-2 {
		t.Fatalf("k: off=%d, хочу %d", m4.searchOff, m3.searchMaxOff()-2)
	}
	for i := 0; i < 20; i++ {
		m4, _ = keyPress(m4, "k")
	}
	if m4.searchOff != 0 {
		t.Fatalf("кламп верха: off=%d", m4.searchOff)
	}
	for i, line := range strings.Split(m3.View().Content, "\n") {
		if lw := lipgloss.Width(line); lw > 120 {
			t.Fatalf("строка %d шире 120: %d", i, lw)
		}
	}
}

// ---- режим выделения сообщения (v)

// selFixture — диалог: чужое, системное, снова чужое и своё сообщение.
func selFixture() Model {
	msgs := []Message{
		{ID: 1, AuthorID: 42, Name: "Анна", Text: "первое", Time: "10:00", DayKey: "d", DayLabel: "сегодня"},
		{ID: 2, AuthorID: 0, System: true, Text: "Анна вошла", Time: "10:01", DayKey: "d", DayLabel: "сегодня"},
		{ID: 3, AuthorID: 42, Name: "Анна", Text: "третье", Time: "10:02", DayKey: "d", DayLabel: "сегодня"},
		{ID: 4, Text: "моё", Time: "10:03", DayKey: "d", DayLabel: "сегодня", Mine: true},
	}
	active := &Chat{DialogID: "chat7"}
	return viewModel(120, 36, nil, nil, nil, active, msgs)
}

func TestSelectEnterAndSkipSystem(t *testing.T) {
	m := selFixture()
	m2, _ := keyPress(m, "v")
	if m2.sel != 3 {
		t.Fatalf("v: sel=%d, хочу 3 (последнее не-системное)", m2.sel)
	}
	m3, _ := keyPress(m2, "k")
	if m3.sel != 2 {
		t.Fatalf("k: sel=%d, хочу 2 (системное перескочено)", m3.sel)
	}
	m4, _ := keyPress(m3, "k")
	if m4.sel != 0 {
		t.Fatalf("k: sel=%d, хочу 0", m4.sel)
	}
	m5, _ := keyPress(m4, "j")
	if m5.sel != 2 {
		t.Fatalf("j: sel=%d, хочу 2 (обратно через системное)", m5.sel)
	}
	m6, _ := keyPress(m5, "G")
	if m6.sel != 3 {
		t.Fatalf("G: sel=%d, хочу 3", m6.sel)
	}
	m7, _ := keyPress(m6, "esc")
	if m7.sel != -1 {
		t.Fatalf("esc не вышел из режима: sel=%d", m7.sel)
	}
}

func TestSelectQuoteReply(t *testing.T) {
	var mu sync.Mutex
	var addParams map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/im.message.add.json") {
			mu.Lock()
			json.NewDecoder(r.Body).Decode(&addParams)
			mu.Unlock()
		}
		fmt.Fprint(w, `{"result":7}`)
	}))
	defer srv.Close()

	m := selFixture()
	m.api = &API{C: b24.New(srv.URL), My: 75}
	m.ctx = context.Background()
	m2, _ := keyPress(m, "v")  // sel=3 — своё
	m3, _ := keyPress(m2, "k") // sel=2 — Анна
	m4, _ := keyPress(m3, "r")
	if m4.sel != -1 || m4.focus != focusReply || m4.replyTo.ID != 3 {
		t.Fatalf("r: sel=%d focus=%v replyTo=%+v", m4.sel, m4.focus, m4.replyTo)
	}
	if v := ansi.Strip(m4.View().Content); !strings.Contains(v, "↳ ответ Анна: третье") {
		t.Fatalf("строки цитаты нет в кадре:\n%s", v)
	}
	m5 := m4
	m5.reply.SetValue("ответ")
	m6, cmd := m5.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter не отправил")
	}
	if sm, ok := cmd().(sentMsg); !ok || sm.err != nil {
		t.Fatalf("команда не отправка: %T", cmd())
	}
	mu.Lock()
	if id, _ := addParams["REPLY_ID"].(float64); int64(id) != 3 {
		t.Errorf("REPLY_ID: %v", addParams["REPLY_ID"])
	}
	if addParams["MESSAGE"] != "ответ" {
		t.Errorf("MESSAGE: %v", addParams["MESSAGE"])
	}
	mu.Unlock()
	// успех — цитата снята
	m7, _ := m6.Update(sentMsg{dialogID: "chat7", id: 7, text: "ответ"})
	if got := m7.(Model).replyTo.ID; got != 0 {
		t.Fatalf("после успешной отправки replyTo.ID=%d, хочу 0", got)
	}
}

func TestSelectEditOwnOnly(t *testing.T) {
	m := selFixture()
	m2, _ := keyPress(m, "v")  // sel=3 — своё
	m3, _ := keyPress(m2, "k") // sel=2 — чужое
	m4, _ := keyPress(m3, "e")
	if m4.editID != 0 || m4.flash != "можно только свои" {
		t.Fatalf("e на чужом: editID=%d flash=%q", m4.editID, m4.flash)
	}
	m5, _ := keyPress(m4, "j") // sel=3
	m6, _ := keyPress(m5, "e")
	if m6.editID != 4 || m6.reply.Value() != "моё" || m6.focus != focusReply {
		t.Fatalf("e на своём: editID=%d value=%q focus=%v",
			m6.editID, m6.reply.Value(), m6.focus)
	}
	if v := ansi.Strip(m6.View().Content); !strings.Contains(v, "✎ редактирование") {
		t.Fatalf("строки правки нет в кадре:\n%s", v)
	}
	// esc при правке: поле чистое, черновик не сохранять
	m7, _ := keyPress(m6, "esc")
	if m7.editID != 0 || m7.reply.Value() != "" || m7.drafts["chat7"] != "" {
		t.Fatalf("esc при правке: editID=%d value=%q draft=%q",
			m7.editID, m7.reply.Value(), m7.drafts["chat7"])
	}
}

func TestSelectEditSendsUpdate(t *testing.T) {
	var mu sync.Mutex
	var updParams map[string]any
	var addHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/im.message.update.json"):
			mu.Lock()
			json.NewDecoder(r.Body).Decode(&updParams)
			mu.Unlock()
		case strings.HasSuffix(r.URL.Path, "/im.message.add.json"):
			mu.Lock()
			addHits++
			mu.Unlock()
		}
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()

	m := selFixture()
	m.api = &API{C: b24.New(srv.URL), My: 75}
	m.ctx = context.Background()
	m2, _ := keyPress(m, "v") // sel=3 — своё (ID 4)
	m3, _ := keyPress(m2, "e")
	m3.reply.SetValue("моё исправлено")
	m4, cmd := m3.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m4.(Model).editID != 0 {
		t.Fatalf("editID не сброшен: %d", m4.(Model).editID)
	}
	if got := m4.(Model).history[3].Text; got != "моё исправлено" {
		t.Fatalf("оптимичной правки нет: %q", got)
	}
	am, ok := cmd().(actionMsg)
	if !ok || am.err != nil {
		t.Fatalf("команда не правка: %T", cmd())
	}
	mu.Lock()
	defer mu.Unlock()
	if id, _ := updParams["MESSAGE_ID"].(float64); int64(id) != 4 {
		t.Errorf("MESSAGE_ID: %v", updParams["MESSAGE_ID"])
	}
	if updParams["MESSAGE"] != "моё исправлено" {
		t.Errorf("MESSAGE: %v", updParams["MESSAGE"])
	}
	if addHits != 0 {
		t.Errorf("правка ушла как im.message.add: %d раз", addHits)
	}
}

func TestSelectDeleteConfirm(t *testing.T) {
	var mu sync.Mutex
	var delHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/im.message.delete.json") {
			mu.Lock()
			delHits++
			mu.Unlock()
		}
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()

	m := selFixture()
	m.api = &API{C: b24.New(srv.URL), My: 75}
	m.ctx = context.Background()
	m2, _ := keyPress(m, "v")  // sel=3 — своё
	m3, _ := keyPress(m2, "k") // sel=2 — чужое
	m4, _ := keyPress(m3, "D")
	if m4.overlay == ovConfirm || m4.flash != "можно только свои" {
		t.Fatalf("D на чужом: overlay=%v flash=%q", m4.overlay, m4.flash)
	}
	m5, _ := keyPress(m4, "j") // sel=3
	m6, _ := keyPress(m5, "D")
	if m6.overlay != ovConfirm {
		t.Fatalf("D не открыл подтверждение: %v", m6.overlay)
	}
	if v := ansi.Strip(m6.View().Content); !strings.Contains(v, "удалить сообщение? y/n") {
		t.Fatalf("подтверждения нет в кадре:\n%s", v)
	}
	// любая другая клавиша закрывает без удаления
	m7, _ := keyPress(m6, "n")
	if m7.overlay != ovNone || len(m7.history) != 4 {
		t.Fatalf("n: overlay=%v history=%d", m7.overlay, len(m7.history))
	}
	// y — удаление: запрос и локальное исчезновение
	m8, _ := keyPress(m7, "D")
	m9, cmd := keyPress(m8, "y")
	if m9.overlay != ovNone || cmd == nil {
		t.Fatalf("y: overlay=%v cmd=%v", m9.overlay, cmd)
	}
	am, ok := cmd().(actionMsg)
	if !ok || am.err != nil {
		t.Fatalf("команда не удаление: %T", cmd())
	}
	m10, _ := m9.Update(am)
	m11 := m10.(Model)
	if len(m11.history) != 3 {
		t.Fatalf("сообщение не убрано из истории: %d", len(m11.history))
	}
	for _, h := range m11.history {
		if h.ID == 4 {
			t.Fatal("удалённое сообщение осталось в истории")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if delHits != 1 {
		t.Errorf("im.message.delete вызван %d раз", delHits)
	}
}

func TestSelectLike(t *testing.T) {
	m := selFixture()
	m2, _ := keyPress(m, "v") // sel=3
	m3, cmd := keyPress(m2, "l")
	if m3.history[3].Likes != 1 || !m3.history[3].LikedByMe {
		t.Fatalf("лайк не применился: %+v", m3.history[3])
	}
	if cmd == nil {
		t.Fatal("нет команды лайка")
	}
	m4, _ := keyPress(m3, "l")
	if m4.history[3].Likes != 0 || m4.history[3].LikedByMe {
		t.Fatalf("повторный l не снял лайк: %+v", m4.history[3])
	}
	// сердечки в рендере: у сообщения с моим лайком
	msgs := []Message{{ID: 1, AuthorID: 42, Name: "Анна", Text: "раз",
		Likes: 2, LikedByMe: true, Time: "10:00", DayKey: "d", DayLabel: "сегодня"}}
	mr := viewModel(120, 36, nil, nil, nil, nil, msgs)
	mr.renderHistory()
	if v := ansi.Strip(mr.vp.View()); !strings.Contains(v, "♥ 2") {
		t.Fatalf("сердечек нет:\n%s", v)
	}
}

func TestSelectMention(t *testing.T) {
	var mu sync.Mutex
	var addParams map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/im.message.add.json") {
			mu.Lock()
			json.NewDecoder(r.Body).Decode(&addParams)
			mu.Unlock()
		}
		fmt.Fprint(w, `{"result":7}`)
	}))
	defer srv.Close()

	m := selFixture()
	m.api = &API{C: b24.New(srv.URL), My: 75}
	m.ctx = context.Background()
	m2, _ := keyPress(m, "v")  // sel=3 — своё (без имени)
	m3, _ := keyPress(m2, "k") // sel=2 — Анна
	m4, _ := keyPress(m3, "@")
	if m4.reply.Value() != "@Анна " || m4.mentions["Анна"] != 42 || m4.focus != focusReply {
		t.Fatalf("@: value=%q mentions=%v focus=%v",
			m4.reply.Value(), m4.mentions, m4.focus)
	}
	m5 := m4
	m5.reply.SetValue(m5.reply.Value() + "что думаешь") // печатаем после подсказки
	_, cmd := m5.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter не отправил")
	}
	if sm, ok := cmd().(sentMsg); !ok || sm.err != nil {
		t.Fatalf("команда не отправка: %T", cmd())
	}
	mu.Lock()
	defer mu.Unlock()
	if addParams["MESSAGE"] != "[USER=42]Анна[/USER] что думаешь" {
		t.Errorf("MESSAGE: %v", addParams["MESSAGE"])
	}
}

func TestSelectRenderQuoteLikesWidth(t *testing.T) {
	msgs := []Message{
		{ID: 1, AuthorID: 42, Name: "Анна", Text: "смотри на это", Time: "10:00", DayKey: "d", DayLabel: "сегодня"},
		{ID: 2, AuthorID: 42, Name: "Кристи娜", Text: "проверь плиз", ReplyID: 1, Likes: 2, LikedByMe: true,
			MentionsMe: true, Time: "10:01", DayKey: "d", DayLabel: "сегодня"},
	}
	active := &Chat{DialogID: "chat7"}
	m := viewModel(80, 24, nil, nil, nil, active, msgs)
	m2, _ := keyPress(m, "v")
	v := m2.View().Content
	s := ansi.Strip(v)
	if !strings.Contains(s, "♥ 2") {
		t.Fatalf("сердечек нет:\n%s", s)
	}
	if !strings.Contains(s, "↳ Анна: смотри на это") {
		t.Fatalf("цитаты нет:\n%s", s)
	}
	marked := false
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "▌") && strings.Contains(line, "проверь плиз") {
			marked = true
		}
	}
	if !marked {
		t.Fatalf("выделенное не помечено:\n%s", s)
	}
	for i, line := range strings.Split(v, "\n") {
		if lw := lipgloss.Width(line); lw > 80 {
			t.Fatalf("строка %d шире 80: %d", i, lw)
		}
	}
}

// Ответ для сменившегося диалога игнорируется целиком.
func TestSearchStaleDialogDropped(t *testing.T) {
	active := &Chat{DialogID: "chat7", ChatID: 7}
	m := viewModel(120, 36, nil, nil, nil, active, nil)
	mm, _ := m.Update(searchMsg{dialogID: "chat1", q: "проверка", rows: []Message{{ID: 1, Text: "чужое"}}})
	m2 := mm.(Model)
	if len(m2.searchRes) != 0 || m2.searchQ != "" || m2.searching {
		t.Fatalf("чужой ответ применился: res=%v q=%q", m2.searchRes, m2.searchQ)
	}
}

// Короткий запрос: портал не дёргается, ошибка — флешем, панель — история.
func TestSearchMinLenFlash(t *testing.T) {
	active := &Chat{DialogID: "chat7", ChatID: 7}
	m := viewModel(120, 36, nil, nil, nil, active, nil)
	m.focus = focusSearch
	m.searchIn.SetValue("ок")
	mm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := mm.(Model)
	if cmd == nil {
		t.Fatal("enter не дал команду")
	}
	msg, ok := cmd().(searchMsg)
	if !ok || msg.err == nil || msg.err.Error() != "минимум 3 символа" {
		t.Fatalf("cmd вернул %T %+v", msg, msg)
	}
	mm3, _ := m2.Update(msg)
	m3 := mm3.(Model)
	if m3.searchOn || m3.flash != "минимум 3 символа" {
		t.Fatalf("searchOn=%v flash=%q", m3.searchOn, m3.flash)
	}
}

func TestSelectSurvivesHistoryMsg(t *testing.T) {
	msgs := []Message{
		{ID: 10, AuthorID: 42, Name: "Анна", Text: "десять", Time: "10:00", DayKey: "d", DayLabel: "сегодня"},
	}
	active := &Chat{DialogID: "chat7"}
	m := viewModel(120, 36, nil, nil, nil, active, msgs)
	m2, _ := keyPress(m, "v") // sel=0
	// догрузка старых: выделение следует за своим сообщением
	m3, _ := m2.Update(historyMsg{dialogID: "chat7", prepend: true,
		rows: []Message{{ID: 5, AuthorID: 42, Name: "Анна", Text: "раньше", DayKey: "d", DayLabel: "сегодня"}}})
	if m3.(Model).sel != 1 || m3.(Model).history[1].ID != 10 {
		t.Fatalf("prepend: sel=%d history[1].ID=%d",
			m3.(Model).sel, m3.(Model).history[1].ID)
	}
	// выделенное удалено на сервере (в свежей странице его нет) — sel сброшен
	m4, _ := m3.Update(historyMsg{dialogID: "chat7",
		rows: []Message{{ID: 9, AuthorID: 42, Name: "Анна", Text: "девять", DayKey: "d", DayLabel: "сегодня"}}})
	m5 := m4.(Model)
	if m5.sel != -1 {
		t.Fatalf("исчезнувшее сообщение: sel=%d", m5.sel)
	}
	if n := len(m5.history); n != 2 || m5.history[1].ID != 9 {
		t.Fatalf("слияние истории: %+v", m5.history)
	}
}

// Правка с упоминанием: в истории — «@Имя», на сервер — BB-код.
func TestSelectEditMentionLocalText(t *testing.T) {
	m := selFixture()
	m.mentions["Анна"] = 42
	m.editID = 4
	m.replyActive()
	m.reply.SetValue("@Анна ок")
	mm, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := mm.(Model).history[3].Text; got != "@Анна ок" {
		t.Fatalf("локальный текст правки: %q", got)
	}
}

// Ответ лайка не снимает флаг идущей отправки.
func TestActionKeepsSending(t *testing.T) {
	m := selFixture()
	m.sending = true
	mm, _ := m.Update(actionMsg{what: "лайк", id: 1})
	if !mm.(Model).sending {
		t.Fatal("лайк сбросил sending идущей отправки")
	}
}

// Отправленное с упоминанием: в истории и в поле при ошибке — «@Имя», не BB-код.
func TestSentMentionReadable(t *testing.T) {
	m := selFixture()
	m.sending = true
	mm, _ := m.Update(sentMsg{dialogID: "chat7", id: 9, text: "[USER=42]Анна[/USER] ок"})
	h := mm.(Model).history
	if got := h[len(h)-1].Text; got != "@Анна ок" {
		t.Fatalf("в истории: %q", got)
	}
	mm, _ = m.Update(sentMsg{dialogID: "chat7", text: "[USER=42]Анна[/USER] ок", err: errors.New("x")})
	if got := mm.(Model).reply.Value(); got != "@Анна ок" {
		t.Fatalf("в поле после ошибки: %q", got)
	}
}
