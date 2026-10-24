package tui

import (
	"encoding/json"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// Ожидания по времени записаны в зоне ПК (UTC+5), а CI крутится в UTC.
func TestMain(m *testing.M) {
	time.Local = time.FixedZone("UTC+5", 5*3600)
	m.Run()
}

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
	chats := ChatsFrom(raw, now, 0)
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

// Коллаб: строка recent указывает на системный чат, живое сообщение —
// в под-чате (message.chat_id). Диалог должен открыться в под-чате,
// иначе вместо переписки — «вступил в проект» (живой кейс коробки).
func TestChatsFromCollabSubchat(t *testing.T) {
	raw := []byte(`{"items":[
		{"id":"chat127","dialogId":"chat127","chat_id":127,"title":"Команда","counter":0,
		 "message":{"id":589233,"text":"нет понимания…","date":"2026-10-20T14:24:21+03:00","chat_id":9097}},
		{"id":"chat6611","chat_id":6611,"title":"Команда_general","counter":0,
		 "message":{"text":"обычный чат","date":"2026-10-19T09:00:00+03:00","chat_id":6611}},
		{"id":13,"chat_id":1305,"title":"Орлова","counter":0,
		 "message":{"text":"личный","date":"2026-10-19T08:00:00+03:00","chat_id":1305}},
		{"id":"chat4271","chat_id":4271,"title":"SMM. команда","counter":0,
		 "message":{"text":"старый формат без chat_id","date":"2026-10-19T07:00:00+03:00"}}
	]}`)
	chats := ChatsFrom(raw, now, 0)
	byTitle := map[string]Chat{}
	for _, c := range chats {
		byTitle[c.Title] = c
	}
	if c := byTitle["Команда"]; c.DialogID != "chat9097" || c.ChatID != 9097 {
		t.Errorf("коллаб не перепривязан к под-чату: %+v", c)
	}
	if c := byTitle["Команда_general"]; c.DialogID != "chat6611" {
		t.Errorf("обычный групповой чат перепривязан зря: %+v", c)
	}
	if c := byTitle["Орлова"]; c.DialogID != "13" {
		t.Errorf("личный диалог перепривязан зря: %+v", c)
	}
	if c := byTitle["SMM. команда"]; c.DialogID != "chat4271" {
		t.Errorf("без message.chat_id чат должен остаться как был: %+v", c)
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
	rows := MessagesFrom(raw, 75, now, 0)
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
	items := NotifyFrom(raw, now, 0)
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

// Часы сервера спешат на 2 часа: метки в ответах на 2 часа впереди, skew
// (местное − серверное) = −2ч сдвигает их обратно ко времени ПК.
func TestSkewShiftsStamps(t *testing.T) {
	if got := ShiftISO("2026-10-19T12:00:00+03:00", 0); got != "2026-10-19T12:00:00+03:00" {
		t.Errorf("нулевой skew меняет метку: %q", got)
	}
	if got := ShiftISO("2026-10-19T12:00:00+03:00", -2*time.Hour); got != "2026-10-19T10:00:00+03:00" {
		t.Errorf("метка не сдвинулась: %q", got)
	}
	raw := []byte(`{"messages":[
		{"id":1,"date":"2026-10-19T12:00:00+03:00","author_id":19,"text":"привет"}],
		"users":[{"id":19,"name":"Ш"}]}`)
	rows := MessagesFrom(raw, 75, now, -2*time.Hour)
	if len(rows) != 1 {
		t.Fatalf("len = %d", len(rows))
	}
	// 12:00+03 − 2ч = 07:00Z; локально (тесты — в зоне ПК) это на 2 часа
	// раньше несдвинутой метки: было бы «14:00», стало «12:00»
	if rows[0].Time != "12:00" {
		t.Errorf("время не сдвинулось: %q", rows[0].Time)
	}
	if rows0 := MessagesFrom(raw, 75, now, 0); rows0[0].Time != "14:00" {
		t.Errorf("без skew время другое, чем ожидалось: %q", rows0[0].Time)
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
	items := NotifyFrom(raw, time.Now(), 0)
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
	chats := ChatsFrom(raw, now, 0)
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

// params: живой портал шлёт и объект с REPLY_ID/LIKE, и пустой массив [].
func TestMessagesFromParams(t *testing.T) {
	raw := []byte(`{"messages":[
		{"id":9,"date":"2026-10-19T08:45:00+03:00","author_id":19,"text":"ответ",
		 "params":{"REPLY_ID":"5","LIKE":[75,3]}},
		{"id":8,"date":"2026-10-19T08:40:00+03:00","author_id":19,"text":"просто",
		 "params":[]}
	],"users":[{"id":19,"name":"Петров"}]}`)
	rows := MessagesFrom(raw, 75, now, 0)
	if len(rows) != 2 {
		t.Fatalf("len = %d (params [] не должен ломать сообщение)", len(rows))
	}
	// порядок после реверса: id 8 сверху
	if rows[0].ID != 8 {
		t.Fatalf("порядок: %+v", rows)
	}
	if rows[0].ReplyID != 0 || rows[0].Likes != 0 || rows[0].LikedByMe {
		t.Errorf("пустой params: %+v", rows[0])
	}
	if rows[1].ReplyID != 5 || rows[1].Likes != 2 || !rows[1].LikedByMe {
		t.Errorf("REPLY_ID/LIKE (мой id 75): %+v", rows[1])
	}
}

func TestMentions(t *testing.T) {
	raw := []byte(`{"messages":[
		{"id":3,"date":"2026-10-19T08:45:00+03:00","author_id":19,
		 "text":"[USER=75]Лёня[/USER] привет"},
		{"id":2,"date":"2026-10-19T08:40:00+03:00","author_id":19,
		 "text":"[user=all]Все[/user] зашли"}
	],"users":[{"id":19,"name":"Петров"}]}`)
	rows := MessagesFrom(raw, 75, now, 0)
	// после реверса: id 2 (all) сверху, id 3 (личное) ниже
	if rows[1].Text != "@Лёня привет" || !rows[1].MentionsMe {
		t.Errorf("личное упоминание: %+v", rows[1])
	}
	if rows[0].Text != "@Все зашли" || !rows[0].MentionsMe {
		t.Errorf("all: %+v", rows[0])
	}
	// чужой id — не я
	raw = []byte(`{"messages":[{"id":1,"date":"2026-10-19T08:35:00+03:00","author_id":19,"text":"[USER=19]Петров[/USER] смотри"}],"users":[]}`)
	if m := MessagesFrom(raw, 75, now, 0)[0]; m.MentionsMe || m.Text != "@Петров смотри" {
		t.Errorf("чужое упоминание: %+v", m)
	}
}

func TestChatsFromChatID(t *testing.T) {
	raw := []byte(`{"items":[
		{"id":"chat10839","chat_id":10839,"title":"SRV. задача","counter":0},
		{"id":"19","title":"Петров","counter":0}
	]}`)
	chats := ChatsFrom(raw, now, 0)
	if len(chats) != 2 || chats[0].ChatID != 10839 {
		t.Fatalf("chat_id группового: %+v", chats)
	}
	if chats[1].ChatID != 0 {
		t.Errorf("у личного без chat_id: %d", chats[1].ChatID)
	}
}

func TestSearchFrom(t *testing.T) {
	raw := []byte(`{"messages":[
		{"id":11,"authorId":19,"date":"2026-10-19T08:45:00+03:00","text":"[b]нашёл[/b] [USER=75]Лёня[/USER]","params":[]},
		{"id":10,"authorId":75,"date":"2026-10-19T08:30:00+03:00","text":"старее","params":{"LIKE":[19,3]}}
	],"users":[{"id":19,"name":"Петров"},{"id":75,"name":"Лёня"}]}`)
	rows := SearchFrom(raw, 75, now, 0)
	if len(rows) != 2 {
		t.Fatalf("len = %d", len(rows))
	}
	// порядок как пришёл: свежие первыми
	if rows[0].ID != 11 || rows[0].Name != "Петров" {
		t.Errorf("порядок и имя: %+v", rows[0])
	}
	if rows[0].Text != "нашёл @Лёня" || !rows[0].MentionsMe {
		t.Errorf("текст/упоминание: %+v", rows[0])
	}
	if rows[1].Likes != 2 || rows[1].LikedByMe || !rows[1].Mine {
		t.Errorf("лайки/mine: %+v", rows[1])
	}
}

// Живая форма страницы im.notify.get: id числом, авторы в users[], ссылка —
// в тексте (calendar) или в params.COMPONENT_PARAMS.SUBJECT (tasks);
// непрочитанные первыми, дальше свежие.
func TestNotifyPageFromLiveShape(t *testing.T) {
	raw := []byte(`{"total_count":3,"total_unread_count":1,"chat_id":1341,
		"notifications":[
			{"id":588283,"chat_id":1341,"author_id":1,"date":"2026-10-20T07:42:22+03:00",
			 "notify_type":2,"notify_module":"calendar","notify_event":"info",
			 "notify_tag":"CALENDAR|INVITE|3025|decline","notify_sub_tag":"CALENDAR|INVITE|3025",
			 "notify_title":"","setting_name":"calendar|info",
			 "text":"Я не приму участие в организованной вами встрече \"[URL=https://crm.example.ru/company/personal/user/75/calendar/?EVENT_ID=3025]Встреча[/URL]\", Понедельник, 12 октября 2026 15:00",
			 "params":null,"notify_read":"Y"},
			{"id":588200,"chat_id":1341,"author_id":89,"date":"2026-10-19T10:05:40+03:00",
			 "notify_type":2,"notify_module":"tasks","notify_event":"manage",
			 "notify_tag":"TASKS|TASK|15947|75","notify_sub_tag":"TASKS|TASK|15947|75",
			 "notify_title":"","setting_name":"tasks|manage",
			 "text":"Назначил вас исполнителем в задаче [#15947] SRV. Подготовить фид",
			 "params":{"COMPONENT_ID":"DefaultEntity","COMPONENT_PARAMS":{"SUBJECT":"#AUTHOR# назначил вас исполнителем в задаче [URL=https://crm.example.ru/workgroups/group/185/tasks/task/view/15947/?ta_sec=chat&ta_el=title_click][#15947] SRV. Подготовить фид[/URL]","PLAIN_TEXT":null},"USERS":[75]},"notify_read":"N"},
			{"id":588100,"chat_id":1341,"author_id":89,"date":"2026-10-15T12:00:00+03:00",
			 "notify_type":2,"notify_module":"tasks","notify_event":"manage",
			 "notify_tag":"TASKS|TASK|16001|75","notify_sub_tag":"TASKS|TASK|16001|75",
			 "notify_title":"","setting_name":"tasks|manage",
			 "text":"Задача закрыта как выполненная",
			 "params":null,"notify_read":"Y"}],
		"users":[{"id":1,"name":"Коллега A"},{"id":89,"name":"Коллега B"}]}`)
	items, unread := NotifyPageFrom(raw, now, 0)
	if unread != 1 {
		t.Fatalf("total_unread_count: %d, хочу 1", unread)
	}
	if len(items) != 3 {
		t.Fatalf("len=%d: %+v", len(items), items)
	}
	// порядок: непрочитанная задача, дальше прочитанные по свежести
	if items[0].ID != "588200" || items[1].ID != "588283" || items[2].ID != "588100" {
		t.Fatalf("порядок: %s,%s,%s", items[0].ID, items[1].ID, items[2].ID)
	}
	if !items[0].Unread || items[1].Unread || items[2].Unread {
		t.Fatalf("флаги чтения: %+v", items)
	}
	if items[0].Author != "Коллега B" || items[1].Author != "Коллега A" || items[2].Author != "Коллега B" {
		t.Fatalf("авторы: %q %q %q", items[0].Author, items[1].Author, items[2].Author)
	}
	if items[0].URL != "https://crm.example.ru/workgroups/group/185/tasks/task/view/15947/?ta_sec=chat&ta_el=title_click" {
		t.Errorf("URL задачи: %q", items[0].URL)
	}
	if items[1].URL != "https://crm.example.ru/company/personal/user/75/calendar/?EVENT_ID=3025" {
		t.Errorf("URL события: %q", items[1].URL)
	}
	if items[2].URL != "" {
		t.Errorf("URL без ссылки: %q", items[2].URL)
	}
	if items[0].TaskID != 15947 {
		t.Errorf("TaskID: %d, хочу 15947", items[0].TaskID)
	}
}

// Полная дата уведа — «02.01.2006 15:04» в локальной зоне ПК.
func TestNotifyFullDate(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("x", 5*3600)
	defer func() { time.Local = old }()

	zn := time.Date(2026, 10, 19, 12, 0, 0, 0, time.FixedZone("x", 5*3600))
	raw := []byte(`{"notifications":[
		{"id":588283,"notify_module":"calendar","date":"2026-10-20T07:42:22+03:00",
		 "notify_read":"Y","text":"событие"}]}`)
	items := NotifyFrom(raw, zn, 0)
	if items[0].Date != "20.10.2026 09:42" {
		t.Fatalf("полная дата: %q, хочу 20.10.2026 09:42", items[0].Date)
	}
}

// Номер задачи в тексте уведа не теряется: [#15947] → #15947 до StripBB.
func TestNotifyTaskRefKept(t *testing.T) {
	raw := []byte(`{"notifications":[
		{"id":1,"notify_module":"tasks","notify_read":"Y",
		 "text":"Назначил вас исполнителем в задаче [#15947] SRV. X",
		 "date":"2026-10-19T10:00:00+03:00"}]}`)
	items := NotifyFrom(raw, now, 0)
	if items[0].Text != "Назначил вас исполнителем в задаче #15947 SRV. X" {
		t.Fatalf("текст уведа: %q", items[0].Text)
	}
}
