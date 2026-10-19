package tui

import (
	"encoding/json"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestStripBB(t *testing.T) {
	cases := map[string]string{
		"[b]жирный[/b]":            "жирный",
		"[USER=75]Лёня[/USER]: q":  "Лёня: q", // имя упоминаемого остаётся
		"  plain  ":                "plain",
		"[sendput=Кнопка|=dialog]": "",
		"":                         "",
	}
	for in, want := range cases {
		if got := StripBB(in); got != want {
			t.Errorf("StripBB(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFmtTimeAndDay(t *testing.T) {
	if got := FmtTime("2026-10-19T08:39:34+03:00", now); got != "10:39" {
		t.Errorf("сегодня: %q", got)
	}
	if got := FmtTime("2026-10-18T20:00:00+03:00", now); got != "18.10" {
		t.Errorf("вчера: %q", got)
	}
	key, label := DayInfo("2026-10-19T08:39:34+03:00", now)
	if label != "сегодня" || key != "2026-10-19" {
		t.Errorf("day сегодня: %q %q", key, label)
	}
	_, label = DayInfo("2026-10-18T20:00:00+03:00", now)
	if label != "вчера" {
		t.Errorf("day вчера: %q", label)
	}
	_, label = DayInfo("2026-10-01T10:00:00+03:00", now)
	if label != "01.10" {
		t.Errorf("day дата: %q", label)
	}
}

func TestIsTaskChat(t *testing.T) {
	if !IsTaskChat("SRV. Улучшить Roistat fallback") {
		t.Error("SRV. должен быть чатом задачи")
	}
	if IsTaskChat("Петров Алексей") {
		t.Error("человек не чат задачи")
	}
}

func TestChatsFrom(t *testing.T) {
	raw := []byte(`{"items":[
		{"id":"chat10839","title":"SRV. задача","counter":0,
		 "message":{"text":"[b]старое[/b]","date":"2026-10-18T09:00:00+03:00"}},
		{"id":"19","title":"Петров","counter":2,
		 "message":{"text":"привет","date":"2026-10-19T09:00:00+03:00"},
		 "user":{"status":"online"}}
	]}`)
	chats := ChatsFrom(raw, now)
	if len(chats) != 2 {
		t.Fatalf("len = %d", len(chats))
	}
	// непрочитанный сверху
	if chats[0].DialogID != "19" || chats[0].Counter != 2 || !chats[0].Online {
		t.Errorf("первый: %+v", chats[0])
	}
	if chats[0].Time != "11:00" {
		t.Errorf("время сегодня не HH:MM: %q", chats[0].Time)
	}
	if chats[1].Preview != "старое" || chats[1].Time != "18.10" {
		t.Errorf("второй: %+v", chats[1])
	}
}

func TestMessagesFrom(t *testing.T) {
	// портал отдаёт свежие первыми
	raw := []byte(`{"messages":[
		{"id":5,"date":"2026-10-19T08:45:00+03:00","author_id":19,"text":"удалено",
		 "params":{"IS_DELETED":"Y"}},
		{"id":4,"date":"2026-10-19T08:40:00+03:00","author_id":75,"text":"моё"},
		{"id":2,"date":"2026-10-19T08:35:00+03:00","author_id":0,"text":"системное"},
		{"id":3,"date":"2026-10-19T08:30:00+03:00","author_id":19,"text":"старое"}
	],"users":[
		{"id":19,"name":"Петров"},
		{"id":75,"name":"Лёня"}
	]}`)
	rows := MessagesFrom(raw, 75, now)
	if len(rows) != 3 {
		t.Fatalf("len = %d (удалённое должно быть выброшено)", len(rows))
	}
	if rows[0].ID != 3 || rows[2].ID != 4 {
		t.Fatalf("порядок от старых к свежим: %+v", rows)
	}
	if !rows[1].System {
		t.Error("author_id=0 должно быть system")
	}
	if !rows[2].Mine || rows[0].Mine {
		t.Error("mine по author_id==75")
	}
	if rows[0].Name != "Петров" {
		t.Errorf("имя из users[]: %q", rows[0].Name)
	}
	if rows[0].DayLabel != "сегодня" {
		t.Errorf("dayLabel: %q", rows[0].DayLabel)
	}
}

func TestNotifyFrom(t *testing.T) {
	raw := []byte(`{"notifications":[
		{"id":"10","notify_module":"tasks","notify_title":"[b]SRV задача[/b]",
		 "text":"новый комментарий","date":"2026-10-19T07:00:00+03:00","notify_read":"Y"},
		{"id":"11","notify_module":"im","notify_title":"",
		 "text":"сообщение","date":"2026-10-19T08:00:00+03:00","notify_read":"N"}
	]}`)
	items := NotifyFrom(raw, now)
	if len(items) != 2 {
		t.Fatalf("len = %d", len(items))
	}
	if items[0].ID != "11" || !items[0].Unread {
		t.Errorf("свежие сверху + unread: %+v", items[0])
	}
	if items[1].Title != "SRV задача" {
		t.Errorf("title без BB: %q", items[1].Title)
	}
}

func TestTaskFrom(t *testing.T) {
	raw := []byte(`{"task":{"id":12475,"title":"SRV. задача","status":2,
		"deadline":"2026-10-19 14:00:00","description":"описание","chatId":10839,
		"creator":{"name":"Лёня"}}}`)
	task := TaskFrom(raw)
	if task.ID != 12475 || task.Title != "SRV. задача" || task.Status != "2" {
		t.Errorf("поля: %+v", task)
	}
	if task.ChatID != 10839 || task.CreatorName != "Лёня" {
		t.Errorf("chat/creator: %+v", task)
	}
	// status числом тоже заходит
	task = TaskFrom([]byte(`{"task":{"id":1,"title":"x","status":5}}`))
	if task.Status != "5" {
		t.Errorf("числовой статус: %+v", task)
	}
}

func TestFlexInt(t *testing.T) {
	var v struct {
		A FlexInt  `json:"a"`
		B FlexInt  `json:"b"`
		C FlexInt  `json:"c"`
		D *FlexInt `json:"d"`
	}
	if err := json.Unmarshal([]byte(`{"a":123,"b":"456","c":null,"d":null}`), &v); err != nil {
		t.Fatal(err)
	}
	if int64(v.A) != 123 || int64(v.B) != 456 || v.D != nil {
		t.Errorf("FlexInt: %+v", v)
	}
}

func TestNotifyTaskID(t *testing.T) {
	raw := []byte(`{"notifications":[
	  {"id":"1","notify_module":"tasks","notify_event":"manage",
	   "notify_tag":"TASKS|TASK|15947|75","notify_title":"","notify_read":"N",
	   "text":"Назначил вас исполнителем в задаче [#15947] SRV. Подготовить фид",
	   "date":"2026-10-19T10:05:40+03:00"},
	  {"id":"2","notify_module":"calendar","notify_event":"reminder",
	   "notify_tag":"CALENDAR|INVITE|3025","notify_title":"","notify_read":"Y",
	   "text":"Напоминание о событии","date":"2026-10-12T12:45:31+03:00"}]}`)
	items := NotifyFrom(raw, time.Now())
	if len(items) != 2 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].TaskID != 15947 {
		t.Errorf("TaskID=%d want 15947", items[0].TaskID)
	}
	if items[1].TaskID != 0 {
		t.Errorf("calendar TaskID=%d want 0", items[1].TaskID)
	}
}

func TestChatsFromNumericID(t *testing.T) {
	raw := []byte(`{"items":[
	  {"id":"chat6299","title":"alerts","counter":0},
	  {"id":89,"title":"Зайцев","counter":2},
	  {"id":null,"title":"битый","counter":0},
	  {"id":"chat127","dialogId":"chat127","title":"Команда","counter":0},
	  {"id":13,"dialogId":"13","title":"Орлова","counter":0}]}`)
	now := time.Now()
	chats := ChatsFrom(raw, now)
	if len(chats) != 4 {
		t.Fatalf("len=%d (битый null должен отфильтроваться): %+v", len(chats), chats)
	}
	byTitle := map[string]string{}
	for _, c := range chats {
		byTitle[c.Title] = c.DialogID
	}
	if byTitle["Зайцев"] != "89" {
		t.Errorf("личный чат: DialogID=%q want 89", byTitle["Зайцев"])
	}
	if byTitle["Команда"] != "chat127" {
		t.Errorf("групповой: DialogID=%q", byTitle["Команда"])
	}
	if byTitle["Орлова"] != "13" {
		t.Errorf("dialogId-фолбэк: DialogID=%q want 13", byTitle["Орлова"])
	}
	for _, c := range chats {
		if c.DialogID == "" {
			t.Errorf("пустой DialogID у %q — источник DIALOG_ID_EMPTY", c.Title)
		}
	}
}
