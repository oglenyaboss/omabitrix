package rules

import (
	"testing"
	"time"

	"omabitrix/internal/b24"
)

func event(entityType, chatType, dialogID string, author int, text string, link string) b24.Event {
	var ev b24.Event
	ev.Type = "ONIMV2MESSAGEADD"
	ev.Data.Message = b24.EventMessage{ID: 1, ChatID: 10839, AuthorID: b24.FlexInt(author), Text: text}
	ev.Data.Chat = b24.EventChat{ID: 10839, DialogID: dialogID, Type: chatType, EntityType: entityType, EntityID: "12475", Name: "Задача X"}
	if link != "" {
		ev.Data.Chat.EntityLink = &b24.EntityLink{URL: link}
	}
	return ev
}

func TestIsMention(t *testing.T) {
	cases := []struct {
		text string
		myID int
		want bool
	}{
		{"[USER=75]Лёня[/USER] глянь", 75, true},
		{"[USER=all]всем[/USER] внимание", 75, true},
		{"[USER=99]Лёня[/USER] глянь", 75, false},
		{"просто текст", 75, false},
	}
	for _, c := range cases {
		if got := IsMention(c.text, c.myID); got != c.want {
			t.Errorf("IsMention(%q) = %v", c.text, got)
		}
	}
}

func TestClassify(t *testing.T) {
	portal := "https://crm.example.ru"

	// Комментарий в задаче: entityLink относительный → абсолютный; источник — в теле
	ev := event("TASKS_TASK", "tasksTask", "chat10839", 99, "fix ready", "/company/personal/user/75/tasks/task/view/5/")
	ev.Data.User = b24.EventUser{ID: 99, Name: "Иван Тестов"}
	tst := Classify(ev, 75, portal)
	if tst == nil || tst.Kind != KindComment || tst.URL != portal+"/company/personal/user/75/tasks/task/view/5/" {
		t.Errorf("comment: %+v", tst)
	}
	if tst.Title != "Задача X" || tst.Body != "Иван Тестов: fix ready" {
		t.Errorf("comment источник: title=%q body=%q", tst.Title, tst.Body)
	}

	// Комментарий без entityLink → фоллбэк на диалог
	tst = Classify(event("TASKS_TASK", "tasksTask", "chat10839", 99, "fix", ""), 75, portal)
	if tst == nil || tst.Kind != KindComment || tst.URL != portal+"/online/?IM_DIALOG=chat10839" {
		t.Errorf("comment fallback: %+v", tst)
	}

	// Личное сообщение: автор крупно, сообщение телом
	ev = event("", "", "99", 99, "привет", "")
	ev.Data.User = b24.EventUser{ID: 99, FirstName: "Иван", LastName: "Тестов"}
	tst = Classify(ev, 75, portal)
	if tst == nil || tst.Kind != KindDialog || tst.URL != portal+"/online/?IM_DIALOG=99" {
		t.Errorf("dialog: %+v", tst)
	}
	if tst.Title != "Иван Тестов" || tst.Body != "привет" {
		t.Errorf("dialog источник: title=%q body=%q", tst.Title, tst.Body)
	}

	// Группа с упоминанием / без
	tst = Classify(event("", "chat", "chat200", 99, "[USER=75]Лёня[/USER] смотри", ""), 75, portal)
	if tst == nil || tst.Kind != KindMention {
		t.Errorf("mention: %+v", tst)
	}
	if tst = Classify(event("", "chat", "chat200", 99, "флуд", ""), 75, portal); tst != nil {
		t.Errorf("группа без упоминания должна молчать: %+v", tst)
	}

	// Своё сообщение и не-сообщение — молчим
	if tst = Classify(event("", "", "99", 75, "сам себе", ""), 75, portal); tst != nil {
		t.Errorf("своё сообщение тостится: %+v", tst)
	}
	ev2 := event("", "chat", "chat200", 99, "x", "")
	ev2.Type = "ONIMV2MESSAGEDELETE"
	if tst = Classify(ev2, 75, portal); tst != nil {
		t.Errorf("DELETE тостится: %+v", tst)
	}
}

func TestSnippet(t *testing.T) {
	if got := Snippet("[USER=75]Лёня[/USER] смотри [B]важно[/B]"); got != "Лёня смотри важно" {
		t.Errorf("snippet: %q", got)
	}
	long := make([]rune, 300)
	for i := range long {
		long[i] = 'а'
	}
	if got := Snippet(string(long)); len([]rune(got)) != 120 {
		t.Errorf("snippet длина %d", len([]rune(got)))
	}
}

func TestDNDActive(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 19, h, m, 0, 0, time.UTC) }
	cases := []struct {
		h, m       int
		start, end string
		want       bool
	}{
		{23, 30, "23:00", "08:00", true}, // в окне
		{3, 0, "23:00", "08:00", true},   // после полуночи — всё ещё окно
		{12, 0, "23:00", "08:00", false}, // день
		{13, 0, "12:00", "14:00", true},  // простое окно
		{15, 0, "12:00", "14:00", false}, // после окна
		{12, 0, "12:00", "12:00", false}, // вырожденное
		{10, 0, "", "", false},           // не задано
		{10, 0, "25:99", "08:00", false}, // кривой формат
	}
	for _, c := range cases {
		if got := DNDActive(at(c.h, c.m), c.start, c.end); got != c.want {
			t.Errorf("DND(%02d:%02d, %q..%q) = %v", c.h, c.m, c.start, c.end, got)
		}
	}
}

func TestKeys(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	if got := DayKey(KindOverdue, "7", now); got != "overdue|7|2026-10-19" {
		t.Errorf("DayKey = %q", got)
	}
	if got := MsgKey(42, now); got != "msg|42|2026-10-19" {
		t.Errorf("MsgKey = %q", got)
	}
}

func TestCollapse(t *testing.T) {
	many := make([]Toast, MaxToasts+5)
	if got := Collapse(many); len(got) != 1 || got[0].Kind != KindSummary {
		t.Errorf("ожидал summary, got %d", len(got))
	}
	few := make([]Toast, 3)
	if got := Collapse(few); len(got) != 3 {
		t.Errorf("3 тоста не должны схлопываться")
	}
}

func TestDeadlineSoon(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	in30 := now.Add(30 * time.Minute)
	in2h := now.Add(2 * time.Hour)
	past := now.Add(-time.Minute)
	mk := func(d time.Time) b24.Task { return b24.Task{Deadline: &d} }
	if !DeadlineSoon(mk(in30), now) {
		t.Error("дедлайн через 30 мин не распознан")
	}
	if DeadlineSoon(mk(in2h), now) {
		t.Error("дедлайн через 2 ч не должен тоститься")
	}
	if DeadlineSoon(mk(past), now) {
		t.Error("прошедший дедлайн — зона overdue, не soon")
	}
}
