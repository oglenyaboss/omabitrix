package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
