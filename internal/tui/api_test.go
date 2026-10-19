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

	"omabitrix/internal/b24"
)

func TestBucketsRFC3339(t *testing.T) {
	loc := time.FixedZone("srv", 3*3600)
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, loc)
	tasks := []Task{
		{ID: 1, Deadline: "2026-10-18T18:00:00+05:00"}, // вчера +05 = 16.10 в +03
		{ID: 2, Deadline: "2026-10-19T18:00:00+05:00"}, // сегодня
		{ID: 3, Deadline: "2026-10-19 14:00:00"},       // naive серверная — сегодня
		{ID: 4, Deadline: "2026-10-21 10:00:00"},       // позже
		{ID: 5, Deadline: ""},                          // без дедлайна
	}
	b := Buckets(tasks, loc, now)
	if len(b.Overdue) != 1 || b.Overdue[0].ID != 1 {
		t.Errorf("overdue: %+v", b.Overdue)
	}
	if len(b.Today) != 2 || b.Today[0].ID != 2 || b.Today[1].ID != 3 {
		t.Errorf("today: %+v", b.Today)
	}
	if len(b.Later) != 1 || b.NoDate[0].ID != 5 {
		t.Errorf("later/nodate: %+v %+v", b.Later, b.NoDate)
	}
}

func TestSendParseMessageID(t *testing.T) {
	cases := map[string]int64{
		`12345`:                   12345,
		`{"message_id": 678}`:     678,
		`{"message_id": "91011"}`: 91011,
	}
	for raw, want := range cases {
		var id FlexInt
		if err := json.Unmarshal([]byte(raw), &id); err != nil {
			var wrap struct {
				MessageID FlexInt `json:"message_id"`
			}
			if err2 := json.Unmarshal([]byte(raw), &wrap); err2 != nil {
				t.Errorf("%s: оба парса упали: %v/%v", raw, err, err2)
				continue
			}
			id = wrap.MessageID
		}
		if int64(id) != want {
			t.Errorf("%s: id=%d want %d", raw, id, want)
		}
	}
}

// ReadDialog — im.dialog.read с обязательным MESSAGE_ID; кэш списка
// сбрасывается только при успехе.
func TestReadDialogAPI(t *testing.T) {
	var mu sync.Mutex
	var method string
	var params map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		method = r.URL.Path
		params = nil
		json.NewDecoder(r.Body).Decode(&params)
		mu.Unlock()
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75}
	a.chatsAt = time.Now()
	if err := a.ReadDialog(context.Background(), "chat10839", 586429); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.HasSuffix(method, "/im.dialog.read.json") {
		t.Errorf("метод: %s", method)
	}
	if params["DIALOG_ID"] != "chat10839" {
		t.Errorf("DIALOG_ID: %v", params["DIALOG_ID"])
	}
	if id, _ := params["MESSAGE_ID"].(float64); int64(id) != 586429 {
		t.Errorf("MESSAGE_ID: %v", params["MESSAGE_ID"])
	}
	if !a.chatsAt.IsZero() {
		t.Error("кэш списка не сброшен")
	}
}

// ReadNotify — ровно одно уведомление: ACTION=Y, ONLY_CURRENT=Y (без него
// портал читает все с ID ≥ данного); локальный флаг гаснет только у этого ID.
func TestReadNotifyAPI(t *testing.T) {
	var params map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/im.notify.read.json") {
			t.Errorf("метод: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&params)
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75,
		allNotify: []NotifyItem{{ID: "42", Unread: true}, {ID: "43", Unread: true}}}
	if err := a.ReadNotify(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	if params["ID"] != "42" || params["ACTION"] != "Y" || params["ONLY_CURRENT"] != "Y" {
		t.Errorf("params: %v", params)
	}
	if a.allNotify[0].Unread || !a.allNotify[1].Unread {
		t.Errorf("флаги: %+v", a.allNotify)
	}
}

func TestDeadlineOptions(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.Local)
	// без текущего дедлайна: сегодня/завтра 18:00, сдвиги от now
	o := deadlineOptions(now, "")
	if o[0].label != "сегодня 18:00" || o[1].label != "завтра 18:00" {
		t.Errorf("labels: %+v", o[:2])
	}
	if want := time.Date(2026, 10, 20, 18, 0, 0, 0, time.Local).Format(time.RFC3339); o[1].val != want {
		t.Errorf("завтра: %s want %s", o[1].val, want)
	}
	if o[4].val != "" {
		t.Errorf("снять дедлайн: %q", o[4].val)
	}
	// с дедлайном: сдвиг сохраняет время и дату базы
	o = deadlineOptions(now, "2026-10-21T10:30:00+05:00")
	if got, want := o[2].val, "2026-10-22T10:30:00+05:00"; got != want {
		t.Errorf("+1 день: %s want %s", got, want)
	}
	if got, want := o[3].val, "2026-10-28T10:30:00+05:00"; got != want {
		t.Errorf("+неделя: %s want %s", got, want)
	}
}
