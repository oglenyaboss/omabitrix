package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"omabitrix/internal/b24"
	"omabitrix/internal/config"
	"omabitrix/internal/rules"
	"omabitrix/internal/state"
)

type fakeSender struct {
	mu     sync.Mutex
	toasts []rules.Toast
}

func (f *fakeSender) Send(_ context.Context, t rules.Toast) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.toasts = append(f.toasts, t)
	return nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.toasts)
}

func (f *fakeSender) has(kind rules.Kind) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.toasts {
		if t.Kind == kind {
			return true
		}
	}
	return false
}

func taskJSON(id int, title string, newComments int, changed, created string) map[string]any {
	if created == "" {
		created = changed
	}
	return map[string]any{
		"id": id, "title": title, "status": 2, "subStatus": 2,
		"deadline":         "2026-10-19T12:30:00+00:00",
		"changedDate":      changed,
		"createdDate":      created,
		"newCommentsCount": newComments,
	}
}

func commentEvent(eventID, author int) map[string]any {
	return map[string]any{
		"eventId": eventID,
		"type":    "ONIMV2MESSAGEADD",
		"data": map[string]any{
			"message": map[string]any{"id": 1000 + eventID, "chatId": 10839, "authorId": author, "text": "fix ready", "date": "2026-10-19T12:00:00+03:00"},
			"chat": map[string]any{
				"id": 10839, "dialogId": "chat10839", "type": "tasksTask", "entityType": "TASKS_TASK",
				"entityId": "12475", "name": "SRV. Задача",
				"entityLink": map[string]any{"type": "TASKS", "url": "/company/personal/user/75/tasks/task/view/12475/", "id": "12475"},
			},
		},
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// mockPortal уважает фильтры дельты (>CHANGED_DATE) и offset событий —
// иначе тесты не ловят watermark-баги (урок оракула).
type mockPortal struct {
	mu       sync.Mutex
	events   []map[string]any // eventId растёт, offset-фильтр уважается
	overdue  []map[string]any
	today    []map[string]any
	delta    []map[string]any // фильтруется по >CHANGED_DATE
	fullPass []map[string]any // для не-batch tasks.task.list (полный проход)
	allErrs  bool
}

func (m *mockPortal) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := filepath.Base(r.URL.Path)
		if base == "im.v2.Event.subscribe.json" {
			fmt.Fprint(w, `{"result":true}`)
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.allErrs {
			fmt.Fprint(w, `{"result":{"result":{},"result_error":{"im":{"error":"API_KEY_NOT_FOUND"},"counters":{"error":"API_KEY_NOT_FOUND"},"recent":{"error":"API_KEY_NOT_FOUND"},"ov_resp":{"error":"API_KEY_NOT_FOUND"}}}}`)
			return
		}
		if base == "tasks.task.list.json" { // полный проход вне batch, с пагинацией
			var body struct {
				Start int `json:"start"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			end := body.Start + 50
			if end > len(m.fullPass) {
				end = len(m.fullPass)
			}
			list := []any{}
			for _, task := range m.fullPass[body.Start:end] {
				list = append(list, task)
			}
			fmt.Fprintf(w, `{"result":{"tasks":%s,"total":%d},"total":%d}`, mustJSON(list), len(m.fullPass), len(m.fullPass))
			return
		}
		if base != "batch.json" {
			t.Errorf("неожиданный путь %s", r.URL.Path)
			fmt.Fprint(w, `{"result":{}}`)
			return
		}
		var body struct {
			CMD map[string]string `json:"cmd"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		res := map[string]any{}
		for k, s := range body.CMD {
			method, q, _ := strings.Cut(s, "?")
			vals, _ := url.ParseQuery(q)
			switch method {
			case "im.counters.get":
				res[k] = map[string]any{"TYPE": map[string]any{"ALL": 1, "NOTIFY": 1, "MESSENGER": 1, "CHAT": 0, "DIALOG": 0}}
			case "im.v2.Event.get":
				offset := 0
				fmt.Sscanf(vals.Get("offset"), "%d", &offset)
				evs := []any{}
				maxID := 0
				for _, e := range m.events {
					id := e["eventId"].(int)
					if id > offset {
						evs = append(evs, e)
					}
					if id > maxID {
						maxID = id
					}
				}
				res[k] = map[string]any{"events": evs, "hasMore": false, "nextOffset": maxID + 1}
			case "im.recent.list":
				res[k] = map[string]any{"items": []any{map[string]any{"id": "chat10839", "chat_id": 10839, "title": "SRV. Задача", "counter": 2}}, "hasMorePages": false}
			case "tasks.task.list":
				if vals.Has("filter[STATUS]") {
					res[k] = map[string]any{"tasks": m.overdue, "total": len(m.overdue)}
				} else if vals.Has("filter[>=DEADLINE]") {
					res[k] = map[string]any{"tasks": m.today, "total": len(m.today)}
				} else if vals.Has("filter[>CHANGED_DATE]") {
					cutoff, _ := time.ParseInLocation("2006-01-02 15:04:05", vals.Get("filter[>CHANGED_DATE]"), time.UTC)
					list := []any{}
					for _, task := range m.delta {
						changed, _ := time.Parse(time.RFC3339, task["changedDate"].(string))
						if changed.After(cutoff) {
							list = append(list, task)
						}
					}
					res[k] = map[string]any{"tasks": list, "total": len(list)}
				} else {
					res[k] = map[string]any{"tasks": []any{}, "total": 0}
				}
			}
		}
		fmt.Fprintf(w, `{"result":{"result":%s}}`, mustJSON(res))
	}
}

func newTestDaemon(t *testing.T, m *mockPortal) (*Daemon, *fakeSender, config.Config) {
	t.Helper()
	srv := httptest.NewServer(m.handler(t))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	cfg := config.Config{
		Portal: srv.URL, WebhookUser: 1, WebhookSecret: "sec", UserID: 75,
		PollIntervalSec: 10,
		StatePath:       filepath.Join(dir, "state.json"),
		SnapshotPath:    filepath.Join(dir, "snap.json"),
	}
	clock := time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC)
	sender := &fakeSender{}
	d := &Daemon{
		Cfg: cfg, Client: b24.New(srv.URL + "/rest/1/sec"),
		State: state.New(), Send: sender,
		Now: func() time.Time { return clock },
		Loc: time.UTC,
	}
	return d, sender, cfg
}

func readSnapshot(t *testing.T, path string) state.Snapshot {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snap state.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestCycle(t *testing.T) {
	m := &mockPortal{
		overdue: []map[string]any{taskJSON(1, "Просроченная задача", 0, "2026-10-15T10:00:00+00:00", "")},
		// старая задача в дельте ДО watermark — не должна попасть даже в baseline
		delta: []map[string]any{taskJSON(9, "Старая задача", 0, "2026-10-19T10:30:00+00:00", "2026-01-01T00:00:00+00:00")},
	}
	d, sender, cfg := newTestDaemon(t, m)

	// Цикл 1: baseline — тихий, watermark инициализируется
	if err := d.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sender.count() != 0 {
		t.Fatalf("baseline не тихий: %d тостов", sender.count())
	}
	if !d.State.BaselineDone || !d.State.KnownTasks[1] {
		t.Fatal("baseline не зафиксирован")
	}
	for _, role := range []string{"resp", "acc", "aud"} {
		if d.State.LastChanged[role] != "2026-10-19 11:00:00" {
			t.Fatalf("watermark %s = %q, хочу 2026-10-19 11:00:00", role, d.State.LastChanged[role])
		}
	}
	snap := readSnapshot(t, cfg.SnapshotPath)
	if snap.Overdue != 1 || snap.Error != "" {
		t.Errorf("snapshot: %+v", snap)
	}
	// MESSENGER=1 при CHAT=0/DIALOG=0 (комменты коллаба): бейдж бара видит их
	if snap.Chats != 1 {
		t.Errorf("snap.Chats = %d, хочу 1 (MESSENGER)", snap.Chats)
	}
	if len(snap.Unread) != 1 || snap.Unread[0].DialogID != "chat10839" || snap.Unread[0].Counter != 2 {
		t.Errorf("unread: %+v", snap.Unread)
	}

	// Цикл 2: изменение ПОСЛЕ watermark ловится; старая задача молчит; событие-комментарий тостится
	m.mu.Lock()
	m.events = append(m.events, commentEvent(2, 99))
	m.delta = append(m.delta,
		taskJSON(2, "Новая задача", 0, "2026-10-19T11:00:30+00:00", "2026-10-19T11:00:20+00:00"),
		taskJSON(3, "Древняя задача", 0, "2026-10-19T11:00:40+00:00", "2026-01-01T00:00:00+00:00"),
	)
	m.mu.Unlock()
	if err := d.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []rules.Kind{rules.KindComment, rules.KindTaskNew, rules.KindOverdue} {
		if !sender.has(want) {
			t.Errorf("нет тоста %v", want)
		}
	}
	if d.State.KnownTasks[3] == false {
		t.Error("старая задача не запомнена")
	}
	found := false
	for _, toast := range sender.toasts {
		if toast.Kind == rules.KindTaskNew && strings.Contains(toast.Title, "Древняя") {
			found = true
		}
	}
	if found {
		t.Error("древняя задача тостится как новая (окно 7 дней не работает)")
	}

	// Цикл 3: идемпотентность
	before := sender.count()
	if err := d.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sender.count(); got != before {
		t.Errorf("дубли: %d -> %d", before, got)
	}

	// Цикл 4: DND глушит всё
	d.Cfg.DND = config.DND{Start: "10:00", End: "12:00"}
	m.mu.Lock()
	m.events = append(m.events, commentEvent(5, 99))
	m.mu.Unlock()
	if err := d.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sender.count(); got != before {
		t.Errorf("DND не глушит: %d -> %d", before, got)
	}

	// Цикл 5: catch-up через полный проход (NEW_COMMENTS_COUNT не фильтруется, CHANGED_DATE не бампается)
	d.Cfg.DND = config.DND{}
	d.State.Cycles = 359 // следующий цикл = 360 → полный проход
	m.mu.Lock()
	m.fullPass = []map[string]any{taskJSON(2, "Новая задача", 3, "2026-10-19T11:00:30+00:00", "2026-10-19T11:00:20+00:00")}
	m.mu.Unlock()
	if err := d.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !sender.has(rules.KindComment) || sender.count() != before+1 {
		t.Errorf("catch-up не сработал: %d (было %d)", sender.count(), before)
	}
}

func TestCycleWebhookDead(t *testing.T) {
	m := &mockPortal{allErrs: true}
	d, _, cfg := newTestDaemon(t, m)
	err := d.Cycle(context.Background())
	if err == nil {
		t.Fatal("ожидал ошибку при мёртвом вебхуке")
	}
	snap := readSnapshot(t, cfg.SnapshotPath)
	if snap.Error == "" {
		t.Fatal("тихая смерть: snapshot.Error пуст при мёртвом вебхуке")
	}
}

// Одно уродливое событие не должно ронять страницу: data.user-массив декодируется
// толерантно (тост без автора), совсем битое — скипается с логом, offset проходит
// страницу целиком.
func TestCycleSurvivesBrokenEvent(t *testing.T) {
	m := &mockPortal{}
	d, sender, _ := newTestDaemon(t, m)
	if err := d.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	} // baseline

	poison := commentEvent(7, 99)
	poison["data"].(map[string]any)["user"] = []any{} // PHP-пустота живого портала
	garbage := map[string]any{"eventId": 8, "type": "ONIMV2MESSAGEADD", "data": "мусор"}
	m.mu.Lock()
	m.events = append(m.events, poison, garbage, commentEvent(9, 99))
	m.mu.Unlock()

	if err := d.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sender.count(); got != 2 {
		t.Errorf("тостов %d, хочу 2: user-массив и хорошее событие, мусор скипнут", got)
	}
	if d.State.ImV2Offset != 10 {
		t.Errorf("offset %d, хочу 10 — страница пройдена целиком", d.State.ImV2Offset)
	}
}
