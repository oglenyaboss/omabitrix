package b24

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Фикстуры повторяют реальные ответы portal.example.com (гейт №0).

const imEventFixture = `{
  "eventId": 1,
  "type": "ONIMV2MESSAGEADD",
  "date": "2026-10-19T07:17:42+03:00",
  "data": {
    "message": {"id": 586429, "chatId": 10839, "chat_id": 10839, "authorId": 75, "author_id": 75, "date": "2026-10-19T07:17:42+03:00", "text": "тест [USER=75]Лёня[/USER] привет", "isSystem": false, "params": []},
    "chat": {"id": 10839, "dialogId": "chat10839", "type": "tasksTask", "entityType": "TASKS_TASK", "entityId": "12475", "name": "SRV. Какая-то задача", "entityLink": {"type": "TASKS", "url": "/company/personal/user/75/tasks/task/view/12475/?ta_sec=chat_tasks", "id": "12475"}}
  }
}`

const taskFixture = `{"id": 15617, "title": "Просроченная задача", "status": 2, "subStatus": 2, "deadline": "2026-10-16T18:00:00+05:00", "changedDate": "2026-10-15T10:00:00+05:00", "createdDate": "2026-10-01T10:00:00+05:00", "newCommentsCount": 0, "chatId": 10839, "group": {"id": 5, "name": "SRV"}}`

func newTestServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL + "/rest/1/sec")
}

func TestImV2Get(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/im.v2.Event.get.json") {
			t.Errorf("путь %s", r.URL.Path)
		}
		fmt.Fprintf(w, `{"result":{"events":[%s],"hasMore":false,"nextOffset":2},"time":{}}`, imEventFixture)
	})
	evs, next, more, err := c.ImV2Get(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || next != 2 || more {
		t.Fatalf("evs=%d next=%d more=%v", len(evs), next, more)
	}
	ev := evs[0]
	if ev.Type != "ONIMV2MESSAGEADD" || int(ev.Data.Message.ID) != 586429 || ev.Data.Message.AuthorID != 75 {
		t.Errorf("message: %+v", ev.Data.Message)
	}
	ch := ev.Data.Chat
	if ch.EntityType != "TASKS_TASK" || ch.DialogID != "chat10839" || ch.EntityLink == nil || ch.EntityLink.URL == "" {
		t.Errorf("chat: %+v", ch)
	}
	if !strings.Contains(ev.Data.Message.Text, "[USER=75]") {
		t.Errorf("текст без BB-кода: %q", ev.Data.Message.Text)
	}
}

func TestTasksPagination(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Start int `json:"start"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		page := body.Start / 50
		tasks := []string{}
		for i := 0; i < 50; i++ {
			id := page*50 + i + 1
			if id > 60 {
				break
			}
			tasks = append(tasks, fmt.Sprintf(`{"id":%d,"title":"t%d","changedDate":"2026-10-01T10:00:00+05:00"}`, id, id))
		}
		fmt.Fprintf(w, `{"result":{"tasks":[%s],"total":60},"total":60}`, strings.Join(tasks, ","))
	})
	tasks, err := c.Tasks(context.Background(), TaskQuery{Filter: map[string]any{"STATUS": -1}, Fields: []string{"ID"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 60 {
		t.Fatalf("задач %d, хочу 60", len(tasks))
	}
}

func TestCountersFlexShapes(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"TYPE":{"ALL":9,"NOTIFY":7,"CHAT":{"10839":2},"DIALOG":0,"LINES":null}}}`)
	})
	ct, err := c.Counters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ct.Chat.Total != 2 || ct.Chat.ByID["10839"] != 2 || ct.Dialog.Total != 0 || ct.Notify != 7 || ct.Lines.Total != 0 {
		t.Errorf("counters: %+v", ct)
	}
}

func TestRecentFlexID(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"items":[{"id":77,"title":"Иван","counter":3,"type":"user"},{"id":"chat10839","title":"Задача","counter":1,"type":"chat"}],"hasMorePages":false}}`)
	})
	rec, err := c.RecentUnread(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec) != 2 || rec[0].ID != "77" || rec[1].ID != "chat10839" || rec[0].Counter != 3 {
		t.Errorf("recent: %+v", rec)
	}
}

func TestBatch(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			CMD map[string]string `json:"cmd"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		res := map[string]any{}
		errs := map[string]any{}
		for k, s := range body.CMD {
			method, _, _ := strings.Cut(s, "?")
			switch method {
			case "im.counters.get":
				res[k] = map[string]any{"TYPE": map[string]any{"ALL": 1, "NOTIFY": 1, "CHAT": 0, "DIALOG": 0}}
			case "boom":
				errs[k] = map[string]string{"error": "WRONG_AUTH_TYPE", "error_description": "нет контекста"}
			default:
				res[k] = map[string]any{"ok": true}
			}
		}
		fmt.Fprintf(w, `{"result":{"result":%s,"result_error":%s}}`, mustJSON(res), mustJSON(errs))
	})
	res, errs, err := c.Batch(context.Background(), map[string]Cmd{
		"counters": {Method: "im.counters.get"},
		"boom":     {Method: "boom"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res["counters"]; !ok {
		t.Error("нет результата counters")
	}
	if errs["boom"] == nil || !strings.Contains(errs["boom"].Error(), "WRONG_AUTH_TYPE") {
		t.Errorf("ошибка boom не разобрана: %v", errs["boom"])
	}
}

func TestEncodeParams(t *testing.T) {
	got := encodeParams(map[string]any{
		"filter":      map[string]any{"STATUS": -1, "!STATUS": []int{5, 6}, ">CHANGED_DATE": "2026-10-19 14:00:00"},
		"select":      []string{"ID", "TITLE"},
		"UNREAD_ONLY": "Y",
	})
	// ключевые фрагменты, порядок map не детерминирован — проверяем вхождение
	for _, want := range []string{
		"filter%5BSTATUS%5D=-1",
		"filter%5B%21STATUS%5D%5B0%5D=5",
		"filter%5B%21STATUS%5D%5B1%5D=6",
		"filter%5B%3ECHANGED_DATE%5D=2026-10-19+14%3A00%3A00",
		"select%5B0%5D=ID",
		"UNREAD_ONLY=Y",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("нет %q в %q", want, got)
		}
	}
}

func TestAPIError(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":"QUERY_LIMIT_EXCEEDED","error_description":"слишком часто"}`)
	})
	_, _, _, err := c.ImV2Get(context.Background(), 0)
	var ae *Error
	if err == nil || !strings.Contains(err.Error(), "QUERY_LIMIT_EXCEEDED") {
		t.Fatalf("ожидал Error, got %v", err)
	}
	if e, ok := err.(*Error); !ok || e.Code != "QUERY_LIMIT_EXCEEDED" {
		_ = ae
		t.Errorf("не *Error: %T", err)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestFlexInt(t *testing.T) {
	var v struct {
		A FlexInt `json:"a"` // строкой, как коробка
		B FlexInt `json:"b"` // числом
		C FlexInt `json:"c"` // null
		D FlexInt `json:"d"` // пустая строка
	}
	if err := json.Unmarshal([]byte(`{"a":"15617","b":42,"c":null,"d":""}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.A != 15617 || v.B != 42 || v.C != 0 || v.D != 0 {
		t.Errorf("FlexInt: %+v", v)
	}
}

func TestTaskStringFields(t *testing.T) {
	// реальный формат коробки: числовые поля строками (гейт №0, portal.example.com)
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"tasks":[{"id":"15617","title":"Просроченная","status":"2","subStatus":"2","deadline":"2026-10-16T18:00:00+05:00","changedDate":"2026-10-15T10:00:00+05:00","createdDate":"2026-10-01T10:00:00+05:00","newCommentsCount":"0","chatId":"10839","group":{"id":"5","name":"SRV"}}],"total":1},"total":1}`)
	})
	tasks, err := c.Tasks(context.Background(), TaskQuery{Filter: map[string]any{"STATUS": -1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("задач %d", len(tasks))
	}
	task := tasks[0]
	if task.ID != 15617 || task.Status != 2 || task.NewCommentsCount != 0 || task.ChatID != 10839 {
		t.Errorf("строчные числовые поля не разобраны: %+v", task)
	}
	if task.Group == nil || task.Group.ID != 5 {
		t.Errorf("group: %+v", task.Group)
	}
}
