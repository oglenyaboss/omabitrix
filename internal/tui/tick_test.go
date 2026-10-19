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
	if cmd == nil {
		t.Fatal("нет команды (карточка + чтение)")
	}
}
