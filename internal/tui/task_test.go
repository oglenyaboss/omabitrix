package tui

import (
	"context"
	"encoding/json"
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

// taskMsgs — история чата задачи для правой панели.
func taskMsgs() []Message {
	return []Message{
		{ID: 1, AuthorID: 42, Name: "Анна", Text: "взяла в работу", Time: "10:00", DayKey: "d", DayLabel: "сегодня"},
		{ID: 2, Text: "ок, жду к вечеру", Time: "10:01", DayKey: "d", DayLabel: "сегодня", Mine: true},
		{ID: 3, AuthorID: 42, Name: "Анна", Text: "готово, посмотри", Time: "10:02", DayKey: "d", DayLabel: "сегодня"},
	}
}

// taskCard — карточка с чатом, сроком и описанием; stages — сколько стадий
// у проекта (3 или 5).
func taskFixture(stages int) ([]Task, []Stage) {
	st := []Stage{
		{ID: 10, Title: "INBOX", Color: "00c4fb"},
		{ID: 20, Title: "TODO", Color: "47d1e2"},
		{ID: 30, Title: "DOING", Color: "75d900"},
	}
	if stages == 5 {
		st = append(st,
			Stage{ID: 40, Title: "REVIEW", Color: "ffd500"},
			Stage{ID: 50, Title: "DONE", Color: "75d900"})
	}
	tasks := []Task{{
		ID: 1, Title: "Разработать раздел дашборд", Role: "do", GroupID: 1,
		Project: "SRV. baden-apart", StageID: 20, Stage: "TODO", StageColor: "47d1e2", StageSort: 1,
		Deadline: "2026-10-19 18:00:00", ChatID: 7, CreatorName: "Лёня", NewComments: 2,
		Description: strings.Repeat("абзац описания задачи. ", 100),
	}}
	return tasks, st
}

// taskScreen — экран задачи после реальной загрузки карточки (taskCardMsg):
// двухпанельный при w ≥ 100, чат открыт автоматически.
func taskScreen(w, h, stages int) Model {
	tasks, st := taskFixture(stages)
	m := boardModel(tasks)
	m.api.stages = map[int64][]Stage{1: st}
	m.width, m.height = w, h
	m.layout()
	m.tab = tabTasks
	m.cursor, m.taskIdx = 0, 0
	m.cardReq = 1
	mm, _ := m.Update(taskCardMsg{requested: 1, task: tasks[0]})
	m = mm.(Model)
	m.history = taskMsgs()
	m.renderHistory()
	return m
}

// Вписывание экрана задачи в 80×24, 100×30, 120×36, 200×50; две панели
// при w ≥ 100, справа — чат задачи.
func TestTaskScreenFits(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {100, 30}, {120, 36}, {200, 50}} {
		m := taskScreen(sz.w, sz.h, 3)
		v := m.View().Content
		if got := lipgloss.Height(v); got > sz.h {
			t.Errorf("w=%d h=%d: высота %d > %d", sz.w, sz.h, got, sz.h)
		}
		for i, line := range strings.Split(v, "\n") {
			if lw := lipgloss.Width(line); lw > sz.w {
				t.Errorf("w=%d h=%d: строка %d шире %d: %d", sz.w, sz.h, i, sz.w, lw)
				break
			}
		}
		s := ansi.Strip(v)
		two := sz.w >= 100
		if has := strings.Contains(s, "чат задачи") && strings.Contains(s, "посмотри"); has != two {
			t.Errorf("w=%d: чат в панели = %v, хочу %v:\n%s", sz.w, has, two, s)
		}
		if two && !strings.Contains(s, "#1 · SRV. baden-apart") {
			t.Errorf("w=%d: в заголовке карточки нет «#1 · проект»:\n%s", sz.w, s)
		}
	}
}

// Трек стадий при 120×36: все стадии проекта в одной строке, текущая на месте.
func TestTaskStageTrack(t *testing.T) {
	m := taskScreen(120, 36, 5)
	v := ansi.Strip(m.View().Content)
	var track string
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "INBOX") && strings.Contains(l, "DONE") {
			track = l
			break
		}
	}
	if track == "" {
		t.Fatalf("трек стадий не в одной строке:\n%s", v)
	}
	for _, want := range []string{"INBOX", "TODO", "DOING", "REVIEW", "DONE"} {
		if !strings.Contains(track, want) {
			t.Fatalf("в треке нет %q: %q", want, track)
		}
	}
	// текущая (TODO) между пройденной и будущей
	if i := strings.Index(track, "TODO"); i < strings.Index(track, "INBOX") || i > strings.Index(track, "DONE") {
		t.Fatalf("текущая стадия не в середине трека: %q", track)
	}
}

// s → меню стадий → вниз → enter: m.tasks и m.taskCard получили новую
// стадию, ушёл task.stages.movetask со stageId.
func TestStageMenuMove(t *testing.T) {
	var mu sync.Mutex
	var params map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/task.stages.movetask.json") {
			mu.Lock()
			params = nil
			json.NewDecoder(r.Body).Decode(&params)
			mu.Unlock()
		}
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()

	tasks, st := taskFixture(3)
	m := boardModel(tasks)
	m.api.stages = map[int64][]Stage{1: st}
	m.api.C = b24.New(srv.URL)
	m.ctx = context.Background()
	m = openCard(m, tasks[0])

	m2, _ := keyPress(m, "s")
	if m2.overlay != ovStatus {
		t.Fatalf("s не открыл меню стадий: %v", m2.overlay)
	}
	v := ansi.Strip(m2.View().Content)
	if !strings.Contains(v, "● TODO") || !strings.Contains(v, "○ INBOX") {
		t.Fatalf("меню стадий без маркеров:\n%s", v)
	}
	if m2.statusIdx != 1 {
		t.Fatalf("меню открылось на %d, хочу текущую TODO (1)", m2.statusIdx)
	}
	m3, _ := keyPress(m2, "j") // с текущей TODO вниз на DOING
	mm, cmd := m3.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m4 := mm.(Model)
	if cmd == nil {
		t.Fatal("enter не дал команду переноса")
	}
	if got := m4.tasks[0]; got.StageID != 30 || got.Stage != "DOING" || got.StageSort != 2 {
		t.Fatalf("m.tasks не перенесён: %+v", got)
	}
	if got := m4.taskCard; got.StageID != 30 || got.Stage != "DOING" {
		t.Fatalf("m.taskCard не перенесён: %+v", got)
	}
	if m4.flash != "→ DOING" {
		t.Fatalf("flash: %q", m4.flash)
	}
	if sm, ok := cmd().(stageMsg); !ok || sm.err != nil {
		t.Fatalf("команда не перенос стадии: %T", cmd())
	}
	mu.Lock()
	if id, _ := params["stageId"].(float64); int64(id) != 30 {
		t.Errorf("stageId: %v, хочу 20", params["stageId"])
	}
	if id, _ := params["id"].(float64); int64(id) != 1 {
		t.Errorf("id: %v, хочу 1", params["id"])
	}
	mu.Unlock()
}

// Задача без GroupID: s показывает старое меню статусов.
func TestStageMenuFallbackStatuses(t *testing.T) {
	m := boardModel([]Task{{ID: 1, Title: "x", Status: "2", Role: "do", ChatID: 0}})
	m = openCard(m, Task{ID: 1, Title: "x", Status: "2"})
	m2, _ := keyPress(m, "s")
	if m2.overlay != ovStatus {
		t.Fatalf("s не открыл меню: %v", m2.overlay)
	}
	if v := ansi.Strip(m2.View().Content); !strings.Contains(v, "в работе") {
		t.Fatalf("меню статусов не в кадре:\n%s", v)
	}
	if _, cmd := m2.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Fatal("enter не дал команду статуса")
	}
}

// H на открытой карточке — предыдущая стадия.
func TestCardHKeyPrevStage(t *testing.T) {
	tasks, st := taskFixture(3)
	m := boardModel(tasks)
	m.api.stages = map[int64][]Stage{1: st}
	m = openCard(m, tasks[0]) // StageID 20 (TODO)
	m2, cmd := keyPress(m, "H")
	if cmd == nil {
		t.Fatal("H не дал команду")
	}
	if got := m2.taskCard; got.StageID != 10 || got.Stage != "INBOX" {
		t.Fatalf("H не вернул в предыдущую стадию: %+v", got)
	}
	if m2.flash != "→ INBOX" {
		t.Fatalf("flash: %q", m2.flash)
	}
	// на первой стадии H — край канбана, ничего
	m3, cmd := keyPress(m2, "H")
	if cmd != nil || m3.taskCard.StageID != 10 {
		t.Fatalf("H на первой стадии: cmd=%v stage=%d", cmd, m3.taskCard.StageID)
	}
}

// Фокус на экране задачи: r → поле, esc → карточка (чат остаётся),
// q → доска, чат задачи закрыт, черновик сохранён.
func TestTaskScreenFocusFlow(t *testing.T) {
	m := taskScreen(120, 36, 3)
	m2, _ := keyPress(m, "r")
	if m2.focus != focusReply {
		t.Fatalf("r: focus=%v, хочу поле ответа", m2.focus)
	}
	m2.reply.SetValue("черновик")
	m3, _ := keyPress(m2, "esc")
	if m3.focus != focusList || m3.active == nil || m3.active.DialogID != "chat7" {
		t.Fatalf("esc: focus=%v active=%v", m3.focus, m3.active)
	}
	if m3.reply.Value() != "черновик" {
		t.Fatalf("esc потерял текст поля: %q", m3.reply.Value())
	}
	m4, cmd := keyPress(m3, "q")
	if cmd != nil {
		t.Fatal("q с экрана задачи вышел из программы")
	}
	if m4.taskOpen || m4.cardReq != 0 || m4.active != nil {
		t.Fatalf("q не закрыл экран: taskOpen=%v cardReq=%d active=%v",
			m4.taskOpen, m4.cardReq, m4.active)
	}
	if m4.drafts["chat7"] != "черновик" {
		t.Fatalf("черновик потерян: %q", m4.drafts["chat7"])
	}
	if m4.cursor != 0 {
		t.Fatalf("курсор ушёл с задачи: %d", m4.cursor)
	}
}

// Задача не с доски (из уведов): groupId/stageId из tasks.task.get,
// проект и стадия — из кэша меты.
func TestCardAnnotateFromCache(t *testing.T) {
	_, st := taskFixture(3)
	m := boardModel(nil)
	m.api.stages = map[int64][]Stage{1: st}
	m.api.projects = map[int64]string{1: "SRV. baden-apart"}
	m.width, m.height = 120, 36
	m.layout()
	m.cardReq = 9
	mm, _ := m.Update(taskCardMsg{requested: 9,
		task: Task{ID: 9, Title: "чужая", Status: "2", GroupID: 1, StageID: 20}})
	got := mm.(Model).taskCard
	if got.Project != "SRV. baden-apart" || got.Stage != "TODO" || got.StageColor != "47d1e2" || got.StageSort != 1 {
		t.Fatalf("annotate карточки: %+v", got)
	}
	// меты нет в кэше — без стадии, без догрузки
	m.cardReq = 10
	mm, _ = m.Update(taskCardMsg{requested: 10,
		task: Task{ID: 10, Title: "вторая", Status: "2", GroupID: 77, StageID: 5}})
	got = mm.(Model).taskCard
	if got.Project != "" || got.Stage != "" {
		t.Fatalf("мета без кэша не должна появиться: %+v", got)
	}
}

// j/k/pgup/pgdn скроллят описание, пока фокус на карточке.
func TestCardDescScroll(t *testing.T) {
	m := taskScreen(120, 36, 3)
	m2, _ := keyPress(m, "j")
	if m2.descOff != 1 || m2.cursor != m.cursor {
		t.Fatalf("j: descOff=%d cursor=%d", m2.descOff, m2.cursor)
	}
	// вверх от начала — на месте
	m3, _ := keyPress(m2, "k")
	m4, _ := keyPress(m3, "k")
	if m4.descOff != 0 {
		t.Fatalf("k ушёл в минус: %d", m4.descOff)
	}
	// pgdown — страницей, pgup возвращает (b/f — алиасы пейджинга)
	m5, _ := keyPress(m, "f")
	if m5.descOff <= 0 {
		t.Fatalf("pgdown не сместил окно: %d", m5.descOff)
	}
	m6, _ := keyPress(m5, "b")
	if m6.descOff != 0 {
		t.Fatalf("pgup не вернул окно: %d", m6.descOff)
	}
}

// Описание скроллится индикатором «ещё ↓», шапка и меню на месте.
func TestCardDescIndicator(t *testing.T) {
	m := taskScreen(120, 36, 3)
	v := ansi.Strip(m.View().Content)
	if !strings.Contains(v, "ещё ↓") {
		t.Fatalf("нет индикатора прокрутки описания:\n%s", v)
	}
	// после прокрутки в конец индикатор исчезает
	for i := 0; i < 100; i++ {
		m, _ = keyPress(m, "j")
	}
	v = ansi.Strip(m.View().Content)
	if strings.Contains(v, "ещё ↓") {
		t.Fatalf("индикатор остался в конце описания:\n%s", v)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "абзац описания") {
		t.Fatal("описание пропало")
	}
}

// ChatID == 0 — правая панель-заглушка.
func TestTaskChatStub(t *testing.T) {
	m := boardModel([]Task{{ID: 1, Title: "x", Status: "2", Role: "do"}})
	m = openCard(m, Task{ID: 1, Title: "x", Status: "2"})
	s := ansi.Strip(m.View().Content)
	if !strings.Contains(s, "чат задачи") || !strings.Contains(s, "у задачи нет чата") {
		t.Fatalf("заглушки чата нет:\n%s", s)
	}
}

// openCard — экран задачи после реальной загрузки карточки.
func openCard(m Model, task Task) Model {
	m.tab = tabTasks
	m.cursor, m.taskIdx = 0, 0
	m.cardReq = task.ID
	mm, _ := m.Update(taskCardMsg{requested: task.ID, task: task})
	return mm.(Model)
}

// Тик-обновление истории работает на экране задачи: история активного чата
// перечитывается независимо от вкладки.
func TestTaskScreenTickRefreshesChat(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/im.dialog.messages.get.json") {
			mu.Lock()
			hits++
			mu.Unlock()
		}
		fmt.Fprint(w, `{"result":{"messages":[],"users":[]}}`)
	}))
	defer srv.Close()

	m := taskScreen(120, 36, 3)
	_, st := taskFixture(3)
	m.api = &API{C: b24.New(srv.URL), My: 75, Now: func() time.Time { return boardNow }}
	m.api.stages = map[int64][]Stage{1: st}
	m.ctx = context.Background()

	mm, cmd := m.Update(tickMsg{})
	m2 := mm.(Model)
	if m2.active == nil || m2.active.DialogID != "chat7" {
		t.Fatalf("тик сбросил чат задачи: %+v", m2.active)
	}
	execAll(cmd)
	mu.Lock()
	defer mu.Unlock()
	if hits < 1 {
		t.Fatal("тик не перечитал историю чата задачи")
	}
}

// execAll — исполнить команду, разворачивая Batch рекурсивно; зависающие
// подкоманды (тики) отсекаются таймаутом.
func execAll(c tea.Cmd) {
	if c == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- c() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(500 * time.Millisecond):
		return
	}
	if bm, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range bm {
			execAll(sub)
		}
	}
}

// Мета-строка карточки: срок-просрочка плашкой, автор, бейдж комментариев.
func TestCardMetaLine(t *testing.T) {
	tasks, _ := taskFixture(3)
	m := boardModel(tasks) // сейчас 19 окт 12:00, срок 18:00 сегодня
	m = openCard(m, tasks[0])
	s := ansi.Strip(m.View().Content)
	if !strings.Contains(s, "сегодня 18:00") || !strings.Contains(s, "от Лёня") || !strings.Contains(s, "✎ 2") {
		t.Fatalf("меты нет в карточке:\n%s", s)
	}
	if strings.Contains(s, "статус:") {
		t.Fatalf("строка статуса осталась в карточке:\n%s", s)
	}
}

// layout зовётся при открытии/закрытии карточки: ширины панелей верные.
func TestLayoutTwoPaneWidths(t *testing.T) {
	m := taskScreen(120, 36, 3)
	if m.cardW != 61 || m.dialW != 50 {
		t.Fatalf("ширины: cardW=%d dialW=%d, хочу 61 и 50", m.cardW, m.dialW)
	}
	// закрыли — обратно на всю ширину
	mm, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m2 := mm.(Model)
	if m2.cardW != 116 || m2.dialW != 70 {
		t.Fatalf("после закрытия: cardW=%d dialW=%d, хочу 116 и 70", m2.cardW, m2.dialW)
	}
}

// Поле ответа видно на экране задачи всегда: без фокуса — разделитель и
// плейсхолдер, высота экрана не ломается.
func TestTaskScreenReplyFieldAlways(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{120, 36}, {239, 62}} {
		m := taskScreen(sz.w, sz.h, 3)
		v := m.View().Content
		if got := lipgloss.Height(v); got > sz.h {
			t.Errorf("w=%d: высота %d > %d", sz.w, got, sz.h)
		}
		s := ansi.Strip(v)
		if !strings.Contains(s, "── ответ") || !strings.Contains(s, "enter или r — написать") {
			t.Errorf("w=%d: поля ответа без фокуса нет:\n%s", sz.w, s)
		}
	}
}

// enter на карточке ставит фокус в поле, enter в поле отправляет: сквозная
// с порталом — im.message.add c DIALOG_ID chat7, сообщение в истории.
func TestTaskScreenEnterToReplySend(t *testing.T) {
	var mu sync.Mutex
	var addParams map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if strings.HasSuffix(r.URL.Path, "/im.message.add.json") {
			addParams = nil
			json.NewDecoder(r.Body).Decode(&addParams)
		}
		mu.Unlock()
		fmt.Fprint(w, `{"result":77}`)
	}))
	defer srv.Close()

	m := taskScreen(120, 36, 3)
	_, st := taskFixture(3)
	m.api = &API{C: b24.New(srv.URL), My: 75, Now: func() time.Time { return boardNow }}
	m.api.stages = map[int64][]Stage{1: st}
	m.ctx = context.Background()

	m2, _ := keyPress(m, "enter")
	if m2.focus != focusReply {
		t.Fatalf("enter: focus=%v", m2.focus)
	}
	m3 := m2
	m3.reply.SetValue("привет")
	mm, cmd := m3.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m4 := mm.(Model)
	if !m4.sending || m4.reply.Value() != "" {
		t.Fatalf("sending=%v поле=%q", m4.sending, m4.reply.Value())
	}
	sm, ok := cmd().(sentMsg)
	if !ok || sm.err != nil || sm.dialogID != "chat7" || sm.text != "привет" {
		t.Fatalf("команда не отправка chat7: %T %+v", cmd(), sm)
	}
	mu.Lock()
	if addParams["DIALOG_ID"] != "chat7" || addParams["MESSAGE"] != "привет" {
		t.Errorf("im.message.add: %v", addParams)
	}
	mu.Unlock()
	mm, _ = m4.Update(sm)
	m5 := mm.(Model)
	found := false
	for _, msg := range m5.history {
		if msg.Mine && msg.Text == "привет" {
			found = true
		}
	}
	if !found {
		t.Fatal("сообщения нет в истории")
	}
}

// Мышь на экране задачи: клик по правой панели — фокус в поле, по левой —
// назад к карточке (черновик остаётся), колесо над чатом скроллит историю.
func TestTaskScreenMouseFocus(t *testing.T) {
	m := taskScreen(120, 36, 3) // левая панель 0..64, правая с 66
	mm, _ := m.Update(tea.MouseClickMsg{X: 70, Y: 10, Button: tea.MouseLeft})
	m2 := mm.(Model)
	if m2.focus != focusReply {
		t.Fatalf("клик по чату: focus=%v", m2.focus)
	}
	m2.reply.SetValue("черновик")
	mm, _ = m2.Update(tea.MouseClickMsg{X: 30, Y: 10, Button: tea.MouseLeft})
	m3 := mm.(Model)
	if m3.focus != focusList || m3.drafts["chat7"] != "черновик" {
		t.Fatalf("клик по карточке: focus=%v draft=%q", m3.focus, m3.drafts["chat7"])
	}
	// колесо над правой панелью скроллит историю чата
	for i := range 40 {
		m3.history = append(m3.history, Message{ID: int64(i + 10), Text: "строка", Time: "10:00"})
	}
	m3.renderHistory()
	m3.vp.GotoBottom()
	before := m3.vp.YOffset()
	if before == 0 {
		t.Fatal("история не длиннее панели — проверка скролла пустая")
	}
	mm, _ = m3.Update(tea.MouseWheelMsg{X: 70, Y: 10, Button: tea.MouseWheelUp})
	m4 := mm.(Model)
	if m4.vp.YOffset() >= before {
		t.Fatalf("колесо над чатом не скроллит: %d → %d", before, m4.vp.YOffset())
	}
	// колесо над карточкой скроллит описание, курсор на месте
	mm, _ = m4.Update(tea.MouseWheelMsg{X: 30, Y: 10, Button: tea.MouseWheelDown})
	m5 := mm.(Model)
	if m5.descOff != 3 || m5.cursor != m4.cursor {
		t.Fatalf("колесо над карточкой: descOff=%d cursor=%d", m5.descOff, m5.cursor)
	}
}
