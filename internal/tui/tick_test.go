package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"omabitrix/internal/b24"
)

// Тик обязан перечитывать все три списка: активность мимо TUI (ответ
// собеседника, отправка из веба) иначе не видна, пока не нажмёшь R.
func TestTickReloadsAllLists(t *testing.T) {
	mu := sync.Mutex{}
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[filepath.Base(r.URL.Path)]++
		mu.Unlock()
		switch filepath.Base(r.URL.Path) {
		case "im.recent.list.json":
			fmt.Fprint(w, `{"result":{"items":[]}}`)
		case "tasks.task.list.json":
			fmt.Fprint(w, `{"result":{"tasks":[],"total":0}}`)
		case "im.notify.get.json":
			fmt.Fprint(w, `{"result":{"notifications":[]}}`)
		default:
			fmt.Fprint(w, `{"result":{}}`)
		}
	}))
	defer srv.Close()

	m := NewModel(&API{C: b24.New(srv.URL), My: 75}, context.Background())
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("тик не вернул команду")
	}
	got := countReloads(cmd)
	if got["im.recent.list.json"] != 1 || got["tasks.task.list.json"] != 1 || got["im.notify.get.json"] != 1 {
		t.Errorf("тик перечитал %v, хочу каждый список по разу", got)
	}
}

// countReloads исполняет cmd, разворачивая Batch рекурсивно; зависающие
// подкоманды (tick10s) отсекаются таймаутом и не считаются.
func countReloads(c tea.Cmd) map[string]int {
	out := map[string]int{}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- c() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(500 * time.Millisecond):
		return out
	}
	if bm, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range bm {
			for k, v := range countReloads(sub) {
				out[k] += v
			}
		}
		return out
	}
	if rm, ok := msg.(reloadMsg); ok {
		if rm.chats {
			out["im.recent.list.json"]++
		}
		if rm.tasks {
			out["tasks.task.list.json"]++
		}
		if rm.notify {
			out["im.notify.get.json"]++
		}
	}
	return out
}

// История загрузилась в открытый непрочитанный чат → счётчик гаснет локально
// и уходит im.dialog.read (MESSAGE_ID — последнее сообщение); при Counter=0
// (тик после чтения) запрос не нужен.
func TestReadDialogOnHistory(t *testing.T) {
	var mu sync.Mutex
	var readParams map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/im.dialog.read.json") {
			mu.Lock()
			readParams = nil
			json.NewDecoder(r.Body).Decode(&readParams)
			mu.Unlock()
		}
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()

	msgs := []Message{
		{ID: 9, Name: "Анна", Text: "привет", Time: "10:00", DayKey: "d", DayLabel: "сегодня"},
		{ID: 12, Name: "Анна", Text: "ещё вопрос", Time: "10:01", DayKey: "d", DayLabel: "сегодня"},
	}
	chats := []Chat{{DialogID: "chat7", Title: "Анна", Counter: 2}}
	active := chats[0] // копия: гасить надо и m.active, и элемент m.chats
	m := viewModel(120, 36, chats, nil, nil, &active, msgs)
	m.api = &API{C: b24.New(srv.URL), My: 75}
	m.ctx = context.Background()

	mm, cmd := m.Update(historyMsg{dialogID: "chat7", rows: msgs})
	m2 := mm.(Model)
	if m2.active.Counter != 0 || m2.chats[0].Counter != 0 {
		t.Fatalf("счётчик: active=%d chats=%d", m2.active.Counter, m2.chats[0].Counter)
	}
	if cmd == nil {
		t.Fatal("история не вернула команду чтения")
	}
	if rm, ok := cmd().(readMsg); !ok || rm.err != nil {
		t.Fatalf("чтение не прошло: %v", rm)
	}
	mu.Lock()
	if readParams["DIALOG_ID"] != "chat7" {
		t.Errorf("DIALOG_ID: %v", readParams["DIALOG_ID"])
	}
	if id, _ := readParams["MESSAGE_ID"].(float64); int64(id) != 12 {
		t.Errorf("MESSAGE_ID: %v", readParams["MESSAGE_ID"])
	}
	mu.Unlock()

	fresh := []Chat{{DialogID: "chat7", Title: "Анна"}}
	m3 := viewModel(120, 36, fresh, nil, nil, &fresh[0], msgs)
	if _, cmd := m3.Update(historyMsg{dialogID: "chat7", rows: msgs}); cmd != nil {
		t.Fatal("при Counter=0 чтение запрашивать нельзя")
	}
}

// Enter на уведомлении: непрочитанное гаснет локально и уходит в im.notify.read,
// переход к карточке и flash «нет перехода» работают как раньше.
func TestReadNotifyOnEnter(t *testing.T) {
	notify := []NotifyItem{
		{ID: "1", Title: "непрочитанное", Time: "10:39", Unread: true},
		{ID: "2", Title: "с задачей", Time: "10:39", TaskID: 5, Unread: true},
	}
	m := viewModel(120, 36, nil, nil, notify, nil, nil)
	m.tab = tabNotify

	// без TaskID: флаг погас, flash на месте, команда чтения есть
	mm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := mm.(Model)
	if m2.notify[0].Unread {
		t.Fatal("enter не сбросил Unread")
	}
	if m2.flash != "у уведомления нет перехода" {
		t.Fatalf("flash: %q", m2.flash)
	}
	if cmd == nil {
		t.Fatal("нет команды im.notify.read")
	}
	// с TaskID: карточка открылась, флаг погас, команда есть
	m2.cursor = 1
	mm, cmd = m2.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m3 := mm.(Model)
	if m3.tab != tabTasks {
		t.Fatalf("enter не открыл карточку: tab=%v", m3.tab)
	}
	if m3.notify[1].Unread {
		t.Fatal("enter не сбросил Unread у уведомления с задачей")
	}
	if m3.cardReq != 5 {
		t.Fatalf("cardReq=%d: ответ карточки отбросится гейтом гонки", m3.cardReq)
	}
	if cmd == nil {
		t.Fatal("нет команды (карточка + чтение)")
	}
}

// После перечитки курсор остаётся на том же элементе: индексы плывут из-за
// пересортировки (непрочитанные вверх).
func TestReloadKeepsChatCursor(t *testing.T) {
	chats := []Chat{
		{DialogID: "a", Title: "чат a"},
		{DialogID: "b", Title: "чат b"},
		{DialogID: "c", Title: "чат c"},
	}
	m := viewModel(120, 36, chats, nil, nil, nil, nil)
	m.tab = tabChats
	m.cursor = 1 // курсор на «b»
	// «b» ушёл наверх — курсор должен последовать за ним
	m.api.allChats = []Chat{chats[1], chats[0], chats[2]}
	mm, _ := m.Update(reloadMsg{chats: true})
	if got := mm.(Model).cursor; got != 0 {
		t.Fatalf("после перечитки курсор %d, хочу 0 (тот же чат b)", got)
	}
}

func TestReloadKeepsTaskCursor(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "один"},
		{ID: 2, Title: "два"},
		{ID: 3, Title: "три"},
	}
	m := viewModel(120, 36, nil, tasks, nil, nil, nil)
	m.tab = tabTasks
	m.cursor, m.taskIdx = 1, 1 // курсор и карточка на задаче 2
	m.taskCard = tasks[1]
	m.api.C = b24.New("http://127.0.0.1:1") // для ServerLoc в Buckets, сеть не дёргается
	m.api.allTasks = []Task{tasks[1], tasks[0], tasks[2]}
	mm, _ := m.Update(reloadMsg{tasks: true})
	got := mm.(Model)
	if got.cursor != 0 || got.taskIdx != 0 {
		t.Fatalf("после перечитки cursor=%d taskIdx=%d, хочу 0 и 0 (задача 2)", got.cursor, got.taskIdx)
	}
	if !got.cardMatchesCursor() {
		t.Fatal("карточка перестала совпадать с курсором")
	}
}

func TestReloadKeepsNotifyCursor(t *testing.T) {
	notify := []NotifyItem{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	m := viewModel(120, 36, nil, nil, notify, nil, nil)
	m.tab = tabNotify
	m.cursor = 2 // курсор на «3»
	m.api.allNotify = []NotifyItem{notify[2], notify[0], notify[1]}
	mm, _ := m.Update(reloadMsg{notify: true})
	if got := mm.(Model).cursor; got != 0 {
		t.Fatalf("после перечитки курсор %d, хочу 0 (то же уведомление)", got)
	}
}

func TestMergeHistory(t *testing.T) {
	old := make([]Message, 0, 10)
	for i := 1; i <= 10; i++ {
		old = append(old, Message{ID: int64(i)})
	}
	fresh := make([]Message, 0, 7)
	for i := 6; i <= 12; i++ {
		fresh = append(fresh, Message{ID: int64(i)})
	}
	got := mergeHistory(old, fresh)
	if len(got) != 12 {
		t.Fatalf("len=%d, хочу 12 без дублей", len(got))
	}
	for i, msg := range got {
		if msg.ID != int64(i+1) {
			t.Fatalf("got[%d].ID=%d, хочу %d", i, msg.ID, i+1)
		}
	}
	// свежая страница пустая — старьё (догруженное по H) остаётся
	if kept := mergeHistory(old, nil); len(kept) != 10 || kept[0].ID != 1 {
		t.Fatalf("пустая fresh стёрла историю: len=%d", len(kept))
	}
	// старой истории нет — свежая целиком
	if only := mergeHistory(nil, fresh); len(only) != 7 || only[0].ID != 6 {
		t.Fatalf("пустое old потерял свежее: len=%d", len(only))
	}
}

// Тик-обновление открытого диалога не выбрасывает догруженное по H старьё.
func TestHistoryMsgMergesOld(t *testing.T) {
	msgs := make([]Message, 0, 10)
	for i := 1; i <= 10; i++ {
		msgs = append(msgs, Message{ID: int64(i), Text: "x", DayKey: "d", DayLabel: "сегодня"})
	}
	active := &Chat{DialogID: "d"}
	m := viewModel(120, 36, nil, nil, nil, active, msgs)
	fresh := []Message{{ID: 11, Text: "новое", DayKey: "d", DayLabel: "сегодня"}}
	mm, _ := m.Update(historyMsg{dialogID: "d", rows: fresh})
	got := mm.(Model)
	if len(got.history) != 11 || got.history[0].ID != 1 {
		t.Fatalf("тик стёр старьё: len=%d first=%d", len(got.history), got.history[0].ID)
	}
}
