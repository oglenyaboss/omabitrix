// Package rules — чистая логика: какие тосты рождаются из событий,
// идемпотентность, DND, ограничение бурь. Без IO — всё тестируемо.
package rules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oglenyaboss/omabitrix/internal/b24"
)

type Kind string

const (
	KindDialog       Kind = "dialog"        // личное сообщение
	KindMention      Kind = "mention"       // упоминание [USER=id] в чате
	KindComment      Kind = "comment"       // комментарий в моей задаче (чат tasksTask)
	KindTaskNew      Kind = "task_new"      // задача впервые в моём скоупе
	KindDeadlineSoon Kind = "deadline_soon" // дедлайн в пределах часа
	KindOverdue      Kind = "overdue"
	KindSummary      Kind = "summary"
)

type Toast struct {
	Kind     Kind
	Title    string
	Body     string
	Urgency  string // low | normal | critical
	URL      string
	DialogID string // клик в режиме toast_open=tui: omabitrix --chat
	TaskID   int64  // клик в режиме toast_open=tui: omabitrix --task
}

// MaxToasts — выше этого летит один summary-тост (после простоя).
const MaxToasts = 20

var mentionRe = regexp.MustCompile(`\[USER=(all|\d+)\]`)

// IsMention: [USER=<мой id>] или [USER=all].
func IsMention(text string, myID int) bool {
	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		if m[1] == "all" {
			return true
		}
		if n, err := strconv.Atoi(m[1]); err == nil && n == myID {
			return true
		}
	}
	return false
}

var bbRe = regexp.MustCompile(`\[[^\]]*\]`)

// Snippet — срезать BB-коды и ужать до 120 рун.
func Snippet(text string) string {
	s := strings.TrimSpace(bbRe.ReplaceAllString(text, ""))
	if s == "" {
		s = strings.TrimSpace(text)
	}
	r := []rune(s)
	if len(r) > 120 {
		return string(r[:119]) + "…"
	}
	return s
}

// Classify решает судьбу im-события. nil — молчим (только счётчики).
func Classify(ev b24.Event, myID int, portal string) *Toast {
	if ev.Type != "ONIMV2MESSAGEADD" {
		return nil
	}
	msg := ev.Data.Message
	chat := ev.Data.Chat
	if int(msg.AuthorID) == myID {
		return nil // своё не тостим
	}
	dialogURL := portal + "/online/?IM_DIALOG=" + string(chat.DialogID)
	body := Snippet(msg.Text)
	author := ev.Data.User.DisplayName()

	switch {
	case chat.EntityType == "TASKS_TASK" || chat.Type == "tasksTask":
		u := dialogURL
		if chat.EntityLink != nil && chat.EntityLink.URL != "" {
			u = chat.EntityLink.URL
			if !strings.HasPrefix(u, "http") {
				u = portal + u
			}
		}
		// заголовок задачи в Title, кто написал — в Body перед текстом
		if author != "" {
			body = author + ": " + body
		}
		return &Toast{Kind: KindComment, Title: chat.Name, Body: body, Urgency: "normal", URL: u, DialogID: chat.DialogID}
	case chat.DialogID != "" && !strings.HasPrefix(chat.DialogID, "chat"):
		// личный диалог: автор крупно, сообщение — телом
		title := author
		if title == "" {
			title = chat.Name
		}
		return &Toast{Kind: KindDialog, Title: title, Body: body, Urgency: "normal", URL: dialogURL, DialogID: chat.DialogID}
	case IsMention(msg.Text, myID):
		title := "Упоминание: " + chat.Name
		if author != "" {
			title = author + " → " + chat.Name
		}
		return &Toast{Kind: KindMention, Title: title, Body: body, Urgency: "normal", URL: dialogURL, DialogID: chat.DialogID}
	default:
		return nil // групповой чат без упоминания — только счётчик
	}
}

// TaskNewWindow — тостим «новая задача» только для действительно свежих:
// baseline сеет KnownTasks неполно (overdue+today), старые задачи
// при первом попадании в дельту молча запоминаются.
const TaskNewWindow = 7 * 24 * time.Hour

func IsRecentlyCreated(task b24.Task, now time.Time) bool {
	return task.CreatedDate.After(now.Add(-TaskNewWindow))
}

// TaskURL — веб-карточка задачи (паттерн подтверждён гейтом).
func TaskURL(portal string, userID, taskID int) string {
	return fmt.Sprintf("%s/company/personal/user/%d/tasks/task/view/%d/", portal, userID, taskID)
}

// TaskNewToast — задача впервые в моём скоупе.
func TaskNewToast(portal string, userID int, task b24.Task) Toast {
	return Toast{
		Kind:    KindTaskNew,
		Title:   "Новая задача: " + task.Title,
		Body:    "",
		Urgency: "normal",
		URL:     TaskURL(portal, userID, int(task.ID)),
		TaskID:  int64(task.ID),
	}
}

// OverdueToast — просрочка.
func OverdueToast(portal string, userID int, task b24.Task) Toast {
	return Toast{
		Kind:    KindOverdue,
		Title:   "Просрочено: " + task.Title,
		Urgency: "critical",
		URL:     TaskURL(portal, userID, int(task.ID)),
		TaskID:  int64(task.ID),
	}
}

// DeadlineSoonToast — дедлайн ближе часа (normal: critical у dunst игнорирует таймаут).
func DeadlineSoonToast(portal string, userID int, task b24.Task) Toast {
	body := ""
	if task.Deadline != nil {
		body = "до " + task.Deadline.In(time.Local).Format("15:04")
	}
	return Toast{
		Kind:    KindDeadlineSoon,
		Title:   "Дедлайн: " + task.Title,
		Body:    body,
		Urgency: "critical",
		URL:     TaskURL(portal, userID, int(task.ID)),
		TaskID:  int64(task.ID),
	}
}

// DeadlineSoon: дедлайн в [now, now+1h].
func DeadlineSoon(task b24.Task, now time.Time) bool {
	return task.Deadline != nil && !task.Deadline.Before(now) && task.Deadline.Before(now.Add(time.Hour))
}

// DayKey — ключ идемпотентности «раз в сутки» (kind|id|YYYY-MM-DD).
func DayKey(kind Kind, id string, now time.Time) string {
	return string(kind) + "|" + id + "|" + now.Format("2006-01-02")
}

// MsgKey — ключ идемпотентности события (тоже с датой: чистится вместе со всеми).
func MsgKey(eventID int, now time.Time) string {
	return "msg|" + strconv.Itoa(eventID) + "|" + now.Format("2006-01-02")
}

func parseHHMM(s string) (int, bool) {
	var h, m int
	if n, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || n != 2 || h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// DNDActive — окно тишины, включая переход через полночь.
func DNDActive(now time.Time, start, end string) bool {
	s, ok := parseHHMM(start)
	if !ok {
		return false
	}
	e, ok := parseHHMM(end)
	if !ok || s == e {
		return false
	}
	m := now.Hour()*60 + now.Minute()
	if s < e {
		return m >= s && m < e
	}
	return m >= s || m < e
}

// Collapse: больше MaxToasts — один summary.
func Collapse(toasts []Toast) []Toast {
	if len(toasts) <= MaxToasts {
		return toasts
	}
	return []Toast{{
		Kind:    KindSummary,
		Title:   "omabitrix",
		Body:    fmt.Sprintf("За время отсутствия: %d событий", len(toasts)),
		Urgency: "normal",
		URL:     "",
	}}
}
