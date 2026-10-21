package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// chatRows: аватар-буква (у групп — «#»), выбранная строка ровно в listW.
// chatFollowFixture — три чата, открыт диалог первого.
func chatFollowFixture() Model {
	chats := []Chat{
		{DialogID: "d0", Title: "чат 0"},
		{DialogID: "d1", Title: "чат 1"},
		{DialogID: "d2", Title: "чат 2"},
	}
	active := chats[0]
	return viewModel(120, 36, chats, nil, nil, &active, nil)
}

// Открытый диалог следует за курсором списка: j ставит тик 250 мс,
// применение тика открывает чат под курсором, фокус остаётся в списке.
func TestChatFollowOnNav(t *testing.T) {
	m := chatFollowFixture()
	mm, cmd := keyPress(m, "j")
	m1 := mm
	if m1.cursor != 1 || m1.active == nil || m1.active.DialogID != "d0" {
		t.Fatalf("j: cursor=%d active=%+v", m1.cursor, m1.active)
	}
	if cmd == nil {
		t.Fatal("j не вернул команду следования диалога")
	}
	fm, ok := cmd().(chatFollowMsg)
	if !ok || fm.dialogID != "d1" {
		t.Fatalf("тик вернул %T %+v", cmd(), cmd())
	}
	mm2, hcmd := m1.Update(fm)
	m2 := mm2.(Model)
	if m2.active == nil || m2.active.DialogID != "d1" {
		t.Fatalf("тик не открыл чат под курсором: %+v", m2.active)
	}
	if m2.focus != focusList {
		t.Fatalf("фокус ушёл из списка: %v", m2.focus)
	}
	if hcmd == nil {
		t.Fatal("нет historyCmd после смены диалога")
	}
}

// Устаревший seq или курсор, ушедший с зафиксированного чата, — тик отброшен.
func TestChatFollowStaleDropped(t *testing.T) {
	m := chatFollowFixture()
	m.cursor = 1
	m.chatFollowSeq = 5
	mm, cmd := m.Update(chatFollowMsg{seq: 4, dialogID: "d1"})
	got := mm.(Model)
	if cmd != nil {
		t.Fatal("устаревший seq запросил историю")
	}
	if got.active == nil || got.active.DialogID != "d0" {
		t.Fatalf("устаревший тик сменил диалог: %+v", got.active)
	}
	// seq актуален, но курсор уже на другом чате
	mm2, cmd2 := m.Update(chatFollowMsg{seq: 5, dialogID: "d2"})
	got2 := mm2.(Model)
	if cmd2 != nil || got2.active.DialogID != "d0" {
		t.Fatalf("тик чужого чата применился: active=%+v cmd=%v", got2.active, cmd2)
	}
}

// Без открытого диалога j только двигает курсор и не открывает чат.
func TestChatFollowRequiresOpenDialog(t *testing.T) {
	chats := []Chat{{DialogID: "d0", Title: "чат 0"}, {DialogID: "d1", Title: "чат 1"}}
	m := viewModel(120, 36, chats, nil, nil, nil, nil)
	mm, cmd := keyPress(m, "j")
	got := mm
	if got.active != nil || got.cursor != 1 || cmd != nil {
		t.Fatalf("j без диалога: active=%+v cursor=%d cmd=%v", got.active, got.cursor, cmd)
	}
}

// Enter в открытом диалоге на другом чате — мгновенное переключение и фокус
// в поле ответа.
func TestEnterSwitchesOpenDialog(t *testing.T) {
	m := chatFollowFixture()
	m.cursor = 1
	mm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := mm.(Model)
	if got.active == nil || got.active.DialogID != "d1" {
		t.Fatalf("enter не открыл чат под курсором: %+v", got.active)
	}
	if got.focus != focusReply {
		t.Fatalf("фокус: %v, хочу focusReply", got.focus)
	}
	if cmd == nil {
		t.Fatal("нет historyCmd")
	}
}

// Клик по строке списка из поля ответа: черновик сохранён, кликнутый чат
// открыт, фокус вернулся в поле ответа; клик в фильтре игнорируется.
func TestMouseClickReplySwitchesChat(t *testing.T) {
	m := chatFollowFixture()
	m.replyActive()
	m.reply.SetValue("черновик")
	mm, cmd := m.Update(tea.MouseClickMsg{X: 10, Y: 4, Button: tea.MouseLeft})
	got := mm.(Model)
	if got.active == nil || got.active.DialogID != "d1" {
		t.Fatalf("клик из поля ответа не открыл чат 1: %+v", got.active)
	}
	if got.drafts["d0"] != "черновик" {
		t.Fatalf("черновик первого чата: %q", got.drafts["d0"])
	}
	if got.focus != focusReply {
		t.Fatalf("фокус: %v, хочу focusReply", got.focus)
	}
	if cmd == nil {
		t.Fatal("нет historyCmd")
	}

	m2 := chatFollowFixture()
	m2.active = nil
	m2.focus = focusFilter
	mm3, cmd3 := m2.Update(tea.MouseClickMsg{X: 10, Y: 2, Button: tea.MouseLeft})
	m3 := mm3.(Model)
	if m3.active != nil || m3.cursor != 0 || cmd3 != nil {
		t.Fatalf("клик в фильтре открыл: active=%+v cursor=%d cmd=%v", m3.active, m3.cursor, cmd3)
	}
}

func TestChatRowsAvatarAndSelection(t *testing.T) {
	chats := []Chat{
		{DialogID: "chat5", Title: "Команда", Counter: 2, Online: true, Preview: "привет", Time: "10:39"},
		{DialogID: "user1", Title: "Орлова Вера", Preview: "щас", Time: "16:47"},
	}
	m := viewModel(120, 36, chats, nil, nil, nil, nil)
	rows := m.chatRows(chats[0], true)
	if s := ansi.Strip(rows[0]); !strings.Contains(s, " # ") {
		t.Fatalf("у группового чата нет аватара «#»: %q", s)
	}
	if w := lipgloss.Width(rows[0]); w != m.listW {
		t.Fatalf("ширина выбранной строки %d, хочу %d", w, m.listW)
	}
	if w := lipgloss.Width(rows[1]); w != m.listW {
		t.Fatalf("ширина второй строки %d, хочу %d", w, m.listW)
	}
	rows2 := m.chatRows(chats[1], false)
	if s := ansi.Strip(rows2[0]); !strings.Contains(s, " К ") {
		t.Fatalf("у личного чата нет аватара «К»: %q", s)
	}
}

// Своё сообщение — той же раскладкой, что чужие: строка «я время»,
// ниже текст обычным цветом, без правого прижатия и «›».
func TestMineMessageRenderedLikePeers(t *testing.T) {
	active := &Chat{DialogID: "chat7"}
	msgs := []Message{{ID: 1, Name: "Ложкин Леонид", Text: "Ок, вечером посмотрю", Time: "15:20", DayKey: "d", DayLabel: "сегодня", Mine: true}}
	m := viewModel(120, 36, nil, nil, nil, active, msgs)
	v := m.vp.View()
	nameLine, textLine := false, ""
	for _, l := range strings.Split(v, "\n") {
		s := ansi.Strip(l)
		if strings.HasPrefix(s, "Ложкин Леонид 15:20") {
			nameLine = true
		}
		if strings.Contains(s, "Ок, вечером") {
			textLine = s
		}
	}
	if !nameLine {
		t.Fatalf("строки имени со временем нет:\n%s", v)
	}
	if strings.TrimSpace(textLine) == "" {
		t.Fatal("текста своего сообщения нет в кадре")
	}
	if w := lipgloss.Width(strings.TrimRight(textLine, " ")); w > m.vp.Width()-1 {
		t.Fatalf("текст шире панели: %q", textLine)
	}
	if strings.Contains(v, "›") {
		t.Fatalf("остался маркер правого прижатия «›»:\n%s", v)
	}
}

// Безымянное своё (optimistic-аппенд) — подпись «я», а не «?».
func TestMineNamelessFallback(t *testing.T) {
	msgs := []Message{{ID: 1, Text: "моё", Time: "15:20", DayKey: "d", DayLabel: "сегодня", Mine: true}}
	m := viewModel(120, 36, nil, nil, nil, &Chat{DialogID: "chat7"}, msgs)
	v := ansi.Strip(m.vp.View())
	if !strings.Contains(v, "\nя 15:20") && !strings.HasPrefix(v, "я 15:20") {
		t.Fatalf("подписи «я» нет:\n%s", v)
	}
}

// Длинное своё сообщение переносится, все строки не шире W.
func TestMineLongMessageWraps(t *testing.T) {
	active := &Chat{DialogID: "chat7"}
	msgs := []Message{{ID: 1, Text: strings.Repeat("слово ", 40), Time: "15:20", DayKey: "d", DayLabel: "сегодня", Mine: true}}
	m := viewModel(120, 36, nil, nil, nil, active, msgs)
	W := m.vp.Width() - 1
	n := 0
	for _, l := range strings.Split(m.vp.View(), "\n") {
		if !strings.Contains(ansi.Strip(l), "слово") {
			continue
		}
		n++
		if w := lipgloss.Width(ansi.Strip(strings.TrimRight(l, " "))); w > W {
			t.Fatalf("строка шире %d: %d", W, w)
		}
	}
	if n < 2 {
		t.Fatalf("длинное сообщение не перенеслось: %d строк", n)
	}
}

// Выделение своего сообщения: «▌» перед строкой, не шире W.
func TestMineSelMarkerAtColumn0(t *testing.T) {
	msgs := []Message{{ID: 1, Text: "моё", Time: "15:20", DayKey: "d", DayLabel: "сегодня", Mine: true}}
	m := viewModel(120, 36, nil, nil, nil, &Chat{DialogID: "chat7"}, msgs)
	m.sel = 0
	m.renderHistory()
	for _, l := range strings.Split(m.vp.View(), "\n") {
		s := ansi.Strip(l)
		if !strings.Contains(s, "моё") {
			continue
		}
		if !strings.HasPrefix(s, "▌") {
			t.Fatalf("маркера в колонке 0 нет: %q", s)
		}
		if w := lipgloss.Width(strings.TrimRight(s, " ")); w > m.vp.Width()-1 {
			t.Fatalf("строка шире W: %d", w)
		}
		return
	}
	t.Fatal("строки своего сообщения нет в кадре")
}

// fixedClock — фиксированное время для тостов, сдвигается сеттером.
func fixedClock() (func() time.Time, func(time.Duration)) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return now }, func(d time.Duration) { now = now.Add(d) }
}

// toastFixture — 20 чатов, у последнего (за окном списка) вырос счётчик;
// возвращает модель и сдвиг фиксированного времени.
func toastFixture() (Model, func(time.Duration)) {
	chats := make([]Chat, 20)
	for i := range chats {
		chats[i] = Chat{DialogID: fmt.Sprintf("d%d", i), Title: fmt.Sprintf("чат %d", i)}
	}
	active := chats[0]
	m := viewModel(120, 36, chats, nil, nil, &active, nil)
	now, shift := fixedClock()
	m.api.Now = now
	fresh := append([]Chat(nil), chats...)
	fresh[19] = Chat{DialogID: "d19", Title: "Дальний чат", Counter: 3, Preview: "новое сообщение", Time: "16:55"}
	m.api.allChats = fresh
	return m, shift
}

// Рост счётчика у неактивного чата — тост с этим чатом в кадре.
func TestToastOnUnreadGrowth(t *testing.T) {
	m, _ := toastFixture()
	mm, _ := m.Update(reloadMsg{chats: true})
	m2 := mm.(Model)
	if m2.toast == nil || m2.toast.chat.DialogID != "d19" {
		t.Fatalf("тост не создан: %+v", m2.toast)
	}
	m2.frame = 6 // выезд закончился — тост целиком в кадре
	if s := ansi.Strip(m2.View().Content); !strings.Contains(s, "Дальний чат") {
		t.Fatalf("тоста с чатом нет в кадре:\n%s", s)
	}
}

// Рост счётчика у активного чата — тоста нет.
func TestToastSkipsActiveChat(t *testing.T) {
	chats := []Chat{{DialogID: "a", Title: "Активный", Counter: 1}, {DialogID: "b", Title: "Борис"}}
	active := chats[0]
	m := viewModel(120, 36, chats, nil, nil, &active, nil)
	now, _ := fixedClock()
	m.api.Now = now
	m.api.allChats = []Chat{{DialogID: "a", Title: "Активный", Counter: 5}, {DialogID: "b", Title: "Борис"}}
	mm, _ := m.Update(reloadMsg{chats: true})
	if got := mm.(Model).toast; got != nil {
		t.Fatalf("тост для активного чата: %+v", got)
	}
}

// Первая загрузка (старый список пуст) — тоста нет.
func TestToastNoFirstLoad(t *testing.T) {
	m := viewModel(120, 36, nil, nil, nil, nil, nil)
	now, _ := fixedClock()
	m.api.Now = now
	m.api.allChats = []Chat{{DialogID: "b", Title: "Борис", Counter: 3}}
	mm, _ := m.Update(reloadMsg{chats: true})
	if got := mm.(Model).toast; got != nil {
		t.Fatalf("тост при первой загрузке: %+v", got)
	}
}

// Через 6 с тост исчез из кадра (сам чат — за окном списка, не путать).
func TestToastExpires(t *testing.T) {
	m, shift := toastFixture()
	mm, _ := m.Update(reloadMsg{chats: true})
	m2 := mm.(Model)
	if m2.toast == nil {
		t.Fatal("тост не создан")
	}
	m2.frame = 6
	if s := ansi.Strip(m2.View().Content); !strings.Contains(s, "Дальний чат") {
		t.Fatal("тоста нет в кадре до истечения")
	}
	shift(6 * time.Second)
	if s := ansi.Strip(m2.View().Content); strings.Contains(s, "Дальний чат") {
		t.Fatalf("просроченный тост в кадре:\n%s", s)
	}
}

// Тост вписывается в экран при выезде и после него.
func TestToastFits(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 36}} {
		m := viewModel(sz[0], sz[1], []Chat{{DialogID: "d", Title: "Т"}}, nil, nil, nil, nil)
		now, _ := fixedClock()
		m.api.Now = now
		m.toast = &toastInfo{chat: Chat{DialogID: "x", Title: "Смирнова Анна", Preview: "ща 10-15 мин", Time: "16:55"}, at: now()}
		if w := lipgloss.Width(m.toastView()); w != 34 {
			t.Fatalf("ширина тоста %d, хочу 34", w)
		}
		for _, frame := range []int{0, 3, 6} {
			m.frame = frame
			v := m.View().Content
			if frame == 6 && !strings.Contains(ansi.Strip(v), "Смирнова") {
				t.Fatal("тоста не видно в кадре при frame=6")
			}
			if h := lipgloss.Height(v); h > sz[1] {
				t.Fatalf("%dx%d frame=%d: высота %d", sz[0], sz[1], frame, h)
			}
			for i, line := range strings.Split(v, "\n") {
				if w := lipgloss.Width(ansi.Strip(line)); w > sz[0] {
					t.Fatalf("%dx%d frame=%d: строка %d шире: %d", sz[0], sz[1], frame, i, w)
				}
			}
		}
	}
}

// Вставка из терминала в поле ответа: CRLF → LF, многострочность сохранена,
// фокус на месте, отправка не запускается (вставка — не Enter).
func TestPasteIntoReply(t *testing.T) {
	m := chatFollowFixture()
	m2, _ := keyPress(m, "r")
	if m2.focus != focusReply {
		t.Fatalf("r не в поле ответа: %v", m2.focus)
	}
	mm, _ := m2.Update(tea.PasteMsg{Content: "привет\r\nмир"})
	m3 := mm.(Model)
	if m3.reply.Value() != "привет\nмир" {
		t.Fatalf("вставка в поле: %q", m3.reply.Value())
	}
	if m3.focus != focusReply || m3.sending {
		t.Fatalf("вставка сбила состояние: focus=%v sending=%v", m3.focus, m3.sending)
	}
	// хвостовой перевод строки — не Enter: отправки нет
	mm, _ = m3.Update(tea.PasteMsg{Content: "хвост\n"})
	m4 := mm.(Model)
	if m4.sending || m4.reply.Value() == "" {
		t.Fatalf("вставка с \n на конце: sending=%v value=%q", m4.sending, m4.reply.Value())
	}
}

// Вставка в фильтр чатов: список отфильтрован, как при наборе с клавиатуры.
func TestPasteIntoFilter(t *testing.T) {
	chats := []Chat{{DialogID: "d0", Title: "Анна"}, {DialogID: "d1", Title: "Борис"}}
	m := viewModel(120, 36, chats, nil, nil, nil, nil)
	m2, _ := keyPress(m, "/")
	if m2.focus != focusFilter {
		t.Fatalf("/ не в фильтре: %v", m2.focus)
	}
	mm, _ := m2.Update(tea.PasteMsg{Content: "Бор"})
	m3 := mm.(Model)
	vc := m3.visibleChats()
	if len(vc) != 1 || vc[0].Title != "Борис" {
		t.Fatalf("фильтр по вставке: %+v", vc)
	}
}

// Вставка в поиск по истории: однострочное поле, переводы строк — в пробел.
func TestPasteIntoSearch(t *testing.T) {
	m := chatFollowFixture()
	m2, _ := keyPress(m, "/")
	if m2.focus != focusSearch {
		t.Fatalf("/ не в поиске: %v", m2.focus)
	}
	mm, _ := m2.Update(tea.PasteMsg{Content: "a\nb"})
	m3 := mm.(Model)
	if m3.searchIn.Value() != "a b" {
		t.Fatalf("вставка в поиск: %q", m3.searchIn.Value())
	}
}

// Вставка при фокусе на списке никуда не попадает и не паникует.
func TestPasteIgnoredOnListFocus(t *testing.T) {
	m := chatFollowFixture()
	m.active = nil
	m.reply.SetValue("было")
	mm, cmd := m.Update(tea.PasteMsg{Content: "x"})
	m2 := mm.(Model)
	if cmd != nil {
		t.Fatal("вставка в списке дала команду")
	}
	if m2.reply.Value() != "было" || m2.filterIn.Value() != "" || m2.searchIn.Value() != "" {
		t.Fatalf("вставка тронула поля: %q/%q/%q",
			m2.reply.Value(), m2.filterIn.Value(), m2.searchIn.Value())
	}
	if m2.focus != focusList {
		t.Fatalf("фокус слетел: %v", m2.focus)
	}
}

// y на выделенном: текст в системный буфер (OSC 52), флеш, режим остаётся;
// пустой текст — флеш «нечего копировать» без команды.
func TestSelCopyToClipboard(t *testing.T) {
	msgs := []Message{{ID: 1, Text: "текст", Time: "15:20", DayKey: "d", DayLabel: "сегодня"}}
	m := chatFollowFixture()
	m.history = msgs
	m.renderHistory()
	m2, _ := keyPress(m, "v")
	if m2.sel != 0 {
		t.Fatalf("v не встал на сообщение: sel=%d", m2.sel)
	}
	m3, cmd := keyPress(m2, "y")
	if cmd == nil {
		t.Fatal("y не дал команду буфера")
	}
	if m3.flash != "✓ скопировано" || m3.sel != 0 {
		t.Fatalf("y: flash=%q sel=%d", m3.flash, m3.sel)
	}
	if s := fmt.Sprintf("%T", cmd()); !strings.Contains(s, "lipboard") {
		t.Fatalf("команда не буфер обмена: %s", s)
	}

	m4 := chatFollowFixture()
	m4.history = []Message{{ID: 1, Text: "", Time: "15:20"}}
	m4.renderHistory()
	m5, _ := keyPress(m4, "v")
	m6, cmd2 := keyPress(m5, "y")
	if cmd2 != nil || m6.flash != "нечего копировать" {
		t.Fatalf("пустой текст: cmd=%v flash=%q", cmd2, m6.flash)
	}
}

// Подсказка режима выделения показывает «y копировать».
func TestSelHelpShowsCopy(t *testing.T) {
	msgs := []Message{{ID: 1, Text: "текст", Time: "15:20", DayKey: "d", DayLabel: "сегодня"}}
	m := chatFollowFixture()
	m.history = msgs
	m.renderHistory()
	m2, _ := keyPress(m, "v")
	if m2.sel != 0 {
		t.Fatalf("v не встал на сообщение: sel=%d", m2.sel)
	}
	if s := ansi.Strip(m2.View().Content); !strings.Contains(s, "копировать") {
		t.Fatalf("подсказки копирования нет:\n%s", s)
	}
}
