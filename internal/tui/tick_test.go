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

	tea "github.com/charmbracelet/bubbletea"
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
