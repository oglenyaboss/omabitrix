package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oglenyaboss/omabitrix/internal/b24"
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

// Рольные задачи идут в свои корзины без дедлайн-раскладки; «делаю» —
// по прежней логике (пустой Role тоже).
func TestBucketsRoles(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	b := Buckets([]Task{
		{ID: 1, Role: "do", Deadline: "2026-10-19 18:00:00"},
		{ID: 2, Role: "help"},
		{ID: 3, Role: "watch"},
		{ID: 4, Deadline: ""},
	}, time.UTC, now)
	if len(b.Help) != 1 || b.Help[0].ID != 2 || len(b.Watch) != 1 || b.Watch[0].ID != 3 {
		t.Errorf("help/watch: %+v %+v", b.Help, b.Watch)
	}
	if len(b.Today) != 1 || b.Today[0].ID != 1 || len(b.NoDate) != 1 || b.NoDate[0].ID != 4 {
		t.Errorf("обычные корзины: today=%+v nodate=%+v", b.Today, b.NoDate)
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

// Chats — непрочитанные под-чатов коллаба (CHANNEL_COMMENT в im.counters.get)
// докладываются на строку коллаба: recent.list их в counter не кладёт,
// и сообщение было бы невидимо в списке (живой кейс коробки).
func TestChatsCollabUnreadFromCounters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/im.recent.list.json"):
			fmt.Fprint(w, `{"result":{"items":[
				{"id":"chat127","chat_id":127,"title":"Команда","counter":0,
				 "message":{"text":"живое обсуждение","date":"2026-10-21T05:00:00+03:00","chat_id":9097}},
				{"id":13,"title":"Орлова","counter":0,
				 "message":{"text":"привет","date":"2026-10-20T09:00:00+03:00"}}]}}`)
		case strings.HasSuffix(r.URL.Path, "/im.counters.get.json"):
			fmt.Fprint(w, `{"result":{"TYPE":{"ALL":2,"MESSENGER":2,"CHAT":0,"DIALOG":0},
				"CHANNEL_COMMENT":{"127":{"9097":2}}}}`)
		default:
			t.Errorf("метод: %s", r.URL.Path)
			fmt.Fprint(w, `{"result":{}}`)
		}
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75}
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	a.Now = func() time.Time { return now }
	chats, err := a.Chats(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 2 {
		t.Fatalf("len=%d: %+v", len(chats), chats)
	}
	if chats[0].Title != "Команда" || chats[0].Counter != 2 || chats[0].DialogID != "chat9097" {
		t.Errorf("коллаб без бейджа под-чата: %+v", chats[0])
	}
	if chats[1].Counter != 0 {
		t.Errorf("обычный чат задет: %+v", chats[1])
	}
}

// MarkNotify — ровно одно уведомление: ACTION=Y, ONLY_CURRENT=Y (без него
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
	if err := a.MarkNotify(context.Background(), "42", true); err != nil {
		t.Fatal(err)
	}
	if params["ID"] != "42" || params["ACTION"] != "Y" || params["ONLY_CURRENT"] != "Y" {
		t.Errorf("params: %v", params)
	}
	if a.allNotify[0].Unread || !a.allNotify[1].Unread {
		t.Errorf("флаги: %+v", a.allNotify)
	}
}

// OpenTasks — один batch из трёх tasks.task.list (RESPONSIBLE_ID/ACCOMPLICE/
// AUDITOR): дубль между ролями берётся с приоритетом do, newCommentsCount —
// бейдж новых комментариев.
func TestOpenTasksRolesBatch(t *testing.T) {
	var mu sync.Mutex
	var cmds map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/batch.json") {
			t.Errorf("метод: %s, хочу batch", r.URL.Path)
		}
		var body struct {
			CMD map[string]string `json:"cmd"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		cmds = body.CMD
		mu.Unlock()
		res := map[string]any{
			"do": map[string]any{"tasks": []any{
				map[string]any{"id": 7, "title": "делаю", "status": "2",
					"deadline": "2026-10-19 18:00:00", "newCommentsCount": 3},
			}},
			"help": map[string]any{"tasks": []any{
				map[string]any{"id": 7, "title": "делаю", "status": "2", "deadline": "2026-10-19 18:00:00"},
				map[string]any{"id": 8, "title": "помогаю", "status": "2", "deadline": "2026-10-19 18:00:00"},
			}},
			"watch": map[string]any{"tasks": []any{
				map[string]any{"id": 7, "title": "делаю", "status": "2", "deadline": "2026-10-19 18:00:00"},
			}},
		}
		b, _ := json.Marshal(map[string]any{"result": res, "result_error": []any{}})
		fmt.Fprintf(w, `{"result":%s}`, b)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75}
	tasks, err := a.OpenTasks(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("задач %d, хочу 2 без дублей: %+v", len(tasks), tasks)
	}
	if tasks[0].ID != 7 || tasks[0].Role != "do" || tasks[0].NewComments != 3 {
		t.Errorf("дубль 7: %+v, хочу Role do и NewComments 3", tasks[0])
	}
	if tasks[1].ID != 8 || tasks[1].Role != "help" {
		t.Errorf("help-задача: %+v", tasks[1])
	}
	mu.Lock()
	defer mu.Unlock()
	// три команды batch: фильтр своей роли + общий !STATUS/select/order
	fields := map[string]string{"do": "RESPONSIBLE_ID", "help": "ACCOMPLICE", "watch": "AUDITOR"}
	for key, field := range fields {
		q, err := url.ParseQuery(strings.TrimPrefix(cmds[key], "tasks.task.list?"))
		if err != nil {
			t.Fatalf("%s: %v (%q)", key, err, cmds[key])
		}
		if got := q.Get("filter[" + field + "]"); got != "75" {
			t.Errorf("%s: filter[%s]=%q, хочу 75", key, field, got)
		}
		if q.Get("filter[!STATUS][0]") != "5" {
			t.Errorf("%s: нет !STATUS 5: %v", key, q)
		}
		if q.Get("order[DEADLINE]") != "asc" {
			t.Errorf("%s: order: %v", key, q)
		}
		// select — отдельные элементы: запятая-строку портал молча игнорирует
		// и не возвращает newCommentsCount (живой факт коробки)
		if q.Get("select[0]") != "ID" || q.Get("select[6]") != "NEW_COMMENTS_COUNT" {
			t.Errorf("%s: select: %v", key, q)
		}
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

// Send: replyID>0 → REPLY_ID в im.message.add, replyID=0 → без него.
func TestSendMessageReply(t *testing.T) {
	var mu sync.Mutex
	var params map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if strings.HasSuffix(r.URL.Path, "/im.message.add.json") {
			params = nil
			json.NewDecoder(r.Body).Decode(&params)
		}
		mu.Unlock()
		fmt.Fprint(w, `{"result":42}`)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75}
	id, err := a.Send(context.Background(), "chat10839", "привет", 9)
	if err != nil || id != 42 {
		t.Fatalf("Send: id=%d err=%v", id, err)
	}
	mu.Lock()
	if rid, _ := params["REPLY_ID"].(float64); rid != 9 {
		t.Errorf("REPLY_ID: %v", params["REPLY_ID"])
	}
	mu.Unlock()

	if _, err := a.Send(context.Background(), "chat10839", "ещё", 0); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if _, ok := params["REPLY_ID"]; ok {
		t.Errorf("REPLY_ID не должен отправляться: %v", params)
	}
}

// UpdateMessage: пустой текст — ошибка без запроса (портал удалил бы сообщение).
func TestUpdateMessageEmpty(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL)}
	if err := a.UpdateMessage(context.Background(), 42, "   "); err == nil {
		t.Fatal("ожидалась ошибка на пустой текст")
	}
	if called {
		t.Error("запрос не должен был уйти")
	}
}

// LikeMessage: WITHOUT_CHANGES (реакция уже в нужном состоянии) — не ошибка.
func TestLikeMessageWithoutChanges(t *testing.T) {
	var params map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&params)
		fmt.Fprint(w, `{"error":"WITHOUT_CHANGES","error_description":"состояние реакции уже было таким же"}`)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL)}
	if err := a.LikeMessage(context.Background(), 42, true); err != nil {
		t.Fatalf("WITHOUT_CHANGES должен глотаться: %v", err)
	}
	if params["ACTION"] != "plus" {
		t.Errorf("ACTION: %v", params["ACTION"])
	}
	if id, _ := params["MESSAGE_ID"].(float64); id != 42 {
		t.Errorf("MESSAGE_ID: %v", params["MESSAGE_ID"])
	}
	if err := a.LikeMessage(context.Background(), 42, false); err != nil {
		t.Fatalf("minus: %v", err)
	}
	if params["ACTION"] != "minus" {
		t.Errorf("ACTION minus: %v", params["ACTION"])
	}
}

// SearchMessages: CHAT_ID + SEARCH_MESSAGE + LIMIT 50, ответ через SearchFrom.
func TestSearchMessagesAPI(t *testing.T) {
	var params map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&params)
		fmt.Fprint(w, `{"result":{"messages":[
			{"id":11,"authorId":19,"date":"2026-10-19T08:45:00+03:00","text":"нашлось","params":[]}
		],"users":[{"id":19,"name":"Петров"}]}}`)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75}
	rows, err := a.SearchMessages(context.Background(), 10839, "  ищем ")
	if err != nil {
		t.Fatal(err)
	}
	if params["CHAT_ID"] != float64(10839) || params["SEARCH_MESSAGE"] != "ищем" || params["LIMIT"] != float64(50) {
		t.Errorf("params: %v", params)
	}
	if len(rows) != 1 || rows[0].ID != 11 || rows[0].Text != "нашлось" || rows[0].Name != "Петров" {
		t.Errorf("rows: %+v", rows)
	}
	// отказы на своей стороне — без запроса к порталу
	if _, err := a.SearchMessages(context.Background(), 0, "ищем"); err == nil {
		t.Error("chatID=0 должен быть ошибкой")
	}
	if _, err := a.SearchMessages(context.Background(), 10839, "ab"); err == nil {
		t.Error("меньше 3 символов должен быть ошибкой")
	}
}

// Стадии и проекты — второй batch по группам задач: стадия 0 портала — первая
// по SORT, имя проекта из sonet_group.get; повторно мета не запрашивается.
func TestOpenTasksStages(t *testing.T) {
	var mu sync.Mutex
	var metaCalls int
	var metaCmds map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			CMD map[string]string `json:"cmd"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		var res map[string]any
		if _, ok := body.CMD["do"]; ok {
			res = map[string]any{
				"do": map[string]any{"tasks": []any{
					map[string]any{"id": 1, "title": "a", "status": "2", "groupId": "13", "stageId": "83"},
					map[string]any{"id": 2, "title": "b", "status": "2", "groupId": "13", "stageId": "0"},
					map[string]any{"id": 3, "title": "c", "status": "2", "groupId": "0", "stageId": "0"},
				}},
				"help": map[string]any{"tasks": []any{}}, "watch": map[string]any{"tasks": []any{}},
			}
		} else {
			mu.Lock()
			metaCalls++
			metaCmds = body.CMD
			mu.Unlock()
			res = map[string]any{
				"st13": map[string]any{
					"83": map[string]any{"ID": "83", "TITLE": "DOING", "SORT": "300", "COLOR": "75d900"},
					"79": map[string]any{"ID": "79", "TITLE": "INBOX", "SORT": "100", "COLOR": "00c4fb"},
					"81": map[string]any{"ID": "81", "TITLE": "TODO", "SORT": "200", "COLOR": "47d1e2"},
				},
				"g13": []any{map[string]any{"ID": "13", "NAME": "DEV. Портал"}},
			}
		}
		b, _ := json.Marshal(map[string]any{"result": res, "result_error": []any{}})
		fmt.Fprintf(w, `{"result":%s}`, b)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75}
	tasks, err := a.OpenTasks(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	if got := byID[1]; got.Stage != "DOING" || got.StageID != 83 || got.StageColor != "75d900" || got.StageSort != 2 || got.Project != "DEV. Портал" || got.GroupID != 13 {
		t.Errorf("задача 1: %+v", got)
	}
	if got := byID[2]; got.Stage != "INBOX" || got.StageID != 79 || got.StageSort != 0 {
		t.Errorf("стадия 0 → первая по SORT: %+v", got)
	}
	if got := byID[3]; got.Stage != "" || got.Project != "" {
		t.Errorf("без проекта — без стадии: %+v", got)
	}
	if st := a.StagesOf(13); len(st) != 3 || st[0].Title != "INBOX" || st[2].Title != "DOING" {
		t.Errorf("StagesOf(13): %+v", st)
	}
	mu.Lock()
	if q := metaCmds["st13"]; !strings.HasPrefix(q, "task.stages.get?") || !strings.Contains(q, "entityId=13") {
		t.Errorf("st13: %q", q)
	}
	mu.Unlock()
	if _, err := a.OpenTasks(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if metaCalls != 1 {
		t.Errorf("мета запрошена %d раз, хочу 1 (кэш)", metaCalls)
	}
}

func TestMoveStageAPI(t *testing.T) {
	var got url.Values
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		got = url.Values{}
		for k, v := range body {
			got.Set(k, fmt.Sprint(v))
		}
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()
	a := &API{C: b24.New(srv.URL), tasksAt: time.Now()}
	if err := a.MoveStage(context.Background(), 14999, 85); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "/task.stages.movetask.json") || got.Get("id") != "14999" || got.Get("stageId") != "85" {
		t.Errorf("запрос: %s %v", path, got)
	}
	if !a.tasksAt.IsZero() {
		t.Error("кэш задач не сброшен")
	}
}

// notifyPage — страница im.notify.get с заданными id: прочитанные, даты
// растут вместе с id (свежие первыми, как на портале); unread — счётчик страницы.
func notifyPage(unread int, ids ...int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `{"total_unread_count":%d,"notifications":[`, unread)
	for i, id := range ids {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"id":%d,"notify_module":"tasks","notify_tag":"TASKS|TASK|%d|75",`+
			`"notify_read":"Y","text":"дело %d","date":"2026-10-%02dT10:00:00+03:00"}`, id, id, id, id)
	}
	sb.WriteString(`],"users":[]}`)
	return sb.String()
}

func idsOf(items []NotifyItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

// Догрузка старых уведов: LAST_ID = минимальный id, LAST_TYPE=3, LIMIT=50;
// дубли не добавляются, пустая страница — конец (дальше запросов нет),
// перечитка первой страницы догруженные строки сохраняет.
func TestNotifyOlderPagination(t *testing.T) {
	var mu sync.Mutex
	var params map[string]any
	gets := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/im.notify.get.json") {
			t.Errorf("метод: %s, хочу im.notify.get", r.URL.Path)
			return
		}
		mu.Lock()
		gets++
		params = nil
		json.NewDecoder(r.Body).Decode(&params)
		mu.Unlock()
		if params["LAST_ID"] == nil {
			fmt.Fprintf(w, `{"result":%s,"time":{}}`, notifyPage(1, 30, 20, 10))
			return
		}
		if got := fmt.Sprint(params["LAST_ID"]); got != "10" {
			// курсор ушёл глубже первой догрузки — старых больше нет
			fmt.Fprint(w, `{"result":{"notifications":[],"users":[]},"time":{}}`)
			return
		}
		if got := fmt.Sprint(params["LAST_TYPE"]); got != "3" {
			t.Errorf("LAST_TYPE: %v, хочу 3", params["LAST_TYPE"])
		}
		if got := fmt.Sprint(params["LIMIT"]); got != "50" {
			t.Errorf("LIMIT: %v, хочу 50", params["LIMIT"])
		}
		fmt.Fprintf(w, `{"result":%s,"time":{}}`, notifyPage(0, 9, 8, 10))
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75}
	if _, err := a.Notify(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if a.NotifyUnread() != 1 {
		t.Fatalf("счётчик страницы: %d, хочу 1", a.NotifyUnread())
	}
	n, err := a.NotifyOlder(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("NotifyOlder: n=%d err=%v (дубль 10 не считается)", n, err)
	}
	if got := strings.Join(idsOf(a.allNotify), ","); got != "30,20,10,9,8" {
		t.Fatalf("после догрузки: %s", got)
	}
	// вторая догрузка: LAST_ID=8, пустая страница — конец
	n, err = a.NotifyOlder(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("вторая догрузка: n=%d err=%v", n, err)
	}
	if !a.NotifyDone() {
		t.Fatal("пустая страница не выставила notifyDone")
	}
	mu.Lock()
	before := gets
	mu.Unlock()
	if _, err := a.NotifyOlder(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	after := gets
	mu.Unlock()
	if after != before {
		t.Fatal("NotifyOlder после конца сделал запрос")
	}

	// перечитка первой страницы: догруженные 9 и 8 остаются
	if _, err := a.Notify(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(idsOf(a.allNotify), ","); got != "30,20,10,9,8" {
		t.Fatalf("перечитка потеряла старые: %s", got)
	}
	if !a.NotifyDone() {
		t.Fatal("Notify сбросил notifyDone")
	}
}

// MarkNotify: ACTION Y/N + ONLY_CURRENT=Y; флаг и счётчик двигаются только
// при реальной смене; ID вне списка — портал зовётся, счётчик на месте.
func TestMarkNotifyAPI(t *testing.T) {
	var mu sync.Mutex
	var calls []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/im.notify.read.json") {
			t.Errorf("метод: %s", r.URL.Path)
		}
		var p map[string]any
		json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		calls = append(calls, p)
		mu.Unlock()
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75,
		allNotify:    []NotifyItem{{ID: "42", Unread: false}},
		notifyUnread: 2}
	if err := a.MarkNotify(context.Background(), "42", false); err != nil {
		t.Fatal(err)
	}
	if p := calls[0]; p["ID"] != "42" || p["ACTION"] != "N" || p["ONLY_CURRENT"] != "Y" {
		t.Errorf("unread params: %v", p)
	}
	if !a.allNotify[0].Unread || a.NotifyUnread() != 3 {
		t.Fatalf("unread: flag=%v counter=%d", a.allNotify[0].Unread, a.NotifyUnread())
	}
	if err := a.MarkNotify(context.Background(), "42", true); err != nil {
		t.Fatal(err)
	}
	if p := calls[1]; p["ACTION"] != "Y" || p["ONLY_CURRENT"] != "Y" {
		t.Errorf("read params: %v", p)
	}
	if a.allNotify[0].Unread || a.NotifyUnread() != 2 {
		t.Fatalf("read: flag=%v counter=%d", a.allNotify[0].Unread, a.NotifyUnread())
	}
	if err := a.MarkNotify(context.Background(), "999", true); err != nil {
		t.Fatal(err)
	}
	if a.NotifyUnread() != 2 {
		t.Fatalf("чужой id сдвинул счётчик: %d", a.NotifyUnread())
	}
}

// Прочитать все: локальные флаги и серверный счётчик в ноль.
func TestReadAllNotifyResetsCounter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":true}`)
	}))
	defer srv.Close()

	a := &API{C: b24.New(srv.URL), My: 75,
		allNotify:    []NotifyItem{{ID: "1", Unread: true}, {ID: "2", Unread: true}},
		notifyUnread: 5}
	if err := a.ReadAllNotify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.allNotify[0].Unread || a.allNotify[1].Unread {
		t.Fatalf("флаги: %+v", a.allNotify)
	}
	if a.NotifyUnread() != 0 {
		t.Fatalf("счётчик: %d, хочу 0", a.NotifyUnread())
	}
}
