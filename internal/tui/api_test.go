package tui

import (
	"encoding/json"
	"testing"
	"time"
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
