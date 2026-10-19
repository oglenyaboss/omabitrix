package tui

import (
	"context"
	"encoding/json"
	"time"

	"omabitrix/internal/b24"
)

// API — REST-слой TUI поверх b24.Client. Живьём проверенные методы:
// im.recent.list (items, v2), im.dialog.messages.get (LAST_ID), im.message.add,
// im.dialog.read (отправка и открытие чата),
// im.notify.get/read/read.all, tasks.task.list/get/update.
type API struct {
	C      *b24.Client
	My     int64  // id себя, из config.webhook_user
	Portal string // адрес портала из config.portal (без хвостового «/»)
	Now    func() time.Time

	chatsAt   time.Time
	tasksAt   time.Time
	notifyAt  time.Time
	allChats  []Chat // полный список, включая чаты задач
	allTasks  []Task
	allNotify []NotifyItem
}

// Chats — список чатов; чаты задач отфильтрованы в Tasks. TTL 20s.
func (a *API) Chats(ctx context.Context, force bool) ([]Chat, error) {
	if !force && time.Since(a.chatsAt) < 20*time.Second && a.allChats != nil {
		return a.allChats, nil
	}
	var raw json.RawMessage
	if err := a.C.Call(ctx, "im.recent.list", map[string]any{"LIMIT": 40}, &raw); err != nil {
		return nil, err
	}
	now := a.now()
	all := ChatsFrom(raw, now)
	people := make([]Chat, 0, len(all))
	for _, c := range all {
		if !IsTaskChat(c.Title) {
			people = append(people, c)
		}
	}
	a.allChats = people
	a.chatsAt = now
	return people, nil
}

// History — сообщения диалога; lastId>0 — страница старее (пагинация LAST_ID).
// noMore=true — достигнуто начало переписки.
func (a *API) History(ctx context.Context, dialogID string, lastID int64) (rows []Message, noMore bool, err error) {
	params := map[string]any{"DIALOG_ID": dialogID, "LIMIT": 30}
	if lastID > 0 {
		params["LAST_ID"] = lastID
	}
	var raw json.RawMessage
	if err = a.C.Call(ctx, "im.dialog.messages.get", params, &raw); err != nil {
		return nil, false, err
	}
	rows = MessagesFrom(raw, a.My, a.now())
	return rows, len(rows) == 0, nil
}

// Send отправляет сообщение и сразу читает диалог (optimistic).
// Портал шлёт result то числом, то {"message_id": N} (живая проверка) — парсим оба.
func (a *API) Send(ctx context.Context, dialogID, text string) (int64, error) {
	var raw json.RawMessage
	if err := a.C.Call(ctx, "im.message.add", map[string]any{
		"DIALOG_ID": dialogID, "MESSAGE": text,
	}, &raw); err != nil {
		return 0, err
	}
	var id FlexInt
	if err := json.Unmarshal(raw, &id); err != nil {
		var wrap struct {
			MessageID FlexInt `json:"message_id"`
		}
		if err2 := json.Unmarshal(raw, &wrap); err2 != nil {
			return 0, err
		}
		id = wrap.MessageID
	}
	_ = a.ReadDialog(ctx, dialogID, int64(id))
	a.chatsAt = time.Time{} // сброс кэша списка: превью/счётчик обновятся
	return int64(id), nil
}

// ReadDialog — прочитать диалог до lastID включительно (MESSAGE_ID обязателен).
func (a *API) ReadDialog(ctx context.Context, dialogID string, lastID int64) error {
	err := a.C.Call(ctx, "im.dialog.read", map[string]any{
		"DIALOG_ID": dialogID, "MESSAGE_ID": lastID,
	}, nil)
	if err == nil {
		a.chatsAt = time.Time{} // счётчик чата в списке погаснет
	}
	return err
}

// Notify — системные уведомления. TTL 20s.
func (a *API) Notify(ctx context.Context, force bool) ([]NotifyItem, error) {
	if !force && time.Since(a.notifyAt) < 20*time.Second && a.allNotify != nil {
		return a.allNotify, nil
	}
	var raw json.RawMessage
	if err := a.C.Call(ctx, "im.notify.get", map[string]any{"LIMIT": 30}, &raw); err != nil {
		return nil, err
	}
	a.allNotify = NotifyFrom(raw, a.now())
	a.notifyAt = a.now()
	return a.allNotify, nil
}

// ReadAllNotify — пометить все прочитанными + локально сбросить флаги.
func (a *API) ReadAllNotify(ctx context.Context) error {
	if err := a.C.Call(ctx, "im.notify.read.all", nil, nil); err != nil {
		return err
	}
	for i := range a.allNotify {
		a.allNotify[i].Unread = false
	}
	return nil
}

// ReadNotify — прочитать ровно одно уведомление (ONLY_CURRENT обязателен:
// без него портал читает все с ID ≥ данного).
func (a *API) ReadNotify(ctx context.Context, id string) error {
	if err := a.C.Call(ctx, "im.notify.read", map[string]any{
		"ID": id, "ACTION": "Y", "ONLY_CURRENT": "Y",
	}, nil); err != nil {
		return err
	}
	for i := range a.allNotify {
		if a.allNotify[i].ID == id {
			a.allNotify[i].Unread = false
			break
		}
	}
	return nil
}

// OpenTasks — открытые задачи (не 5/6/7), чаты задач апендятся списком.
// Без дата-фильтров: корзины просрочка/сегодня считает клиент по DEADLINE
// в серверной TZ — naive-фильтры портала больше не участвуют вовсе.
func (a *API) OpenTasks(ctx context.Context, force bool) ([]Task, error) {
	if !force && time.Since(a.tasksAt) < 20*time.Second && a.allTasks != nil {
		return a.allTasks, nil
	}
	var raw json.RawMessage
	err := a.C.Call(ctx, "tasks.task.list", map[string]any{
		"filter": map[string]any{
			"RESPONSIBLE_ID": a.My,
			"!STATUS":        []int{5, 6, 7},
		},
		"select": []string{"ID,TITLE,STATUS,DEADLINE,CREATED_BY,CHAT_ID"},
		"order":  map[string]string{"DEADLINE": "asc"},
	}, &raw)
	if err != nil {
		return nil, err
	}
	var res struct {
		Tasks []struct {
			ID       FlexInt    `json:"id"`
			Title    string     `json:"title"`
			Status   FlexString `json:"status"`
			Deadline string     `json:"deadline"`
			ChatID   FlexInt    `json:"chatId"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	a.allTasks = a.allTasks[:0]
	for _, t := range res.Tasks {
		a.allTasks = append(a.allTasks, Task{
			ID: int64(t.ID), Title: t.Title, Status: string(t.Status),
			Deadline: t.Deadline, ChatID: int64(t.ChatID),
		})
	}
	a.tasksAt = a.now()
	return a.allTasks, nil
}

// Task — полная карточка (tasks.task.get: description, creator).
func (a *API) Task(ctx context.Context, id int64) (Task, error) {
	var raw json.RawMessage
	if err := a.C.Call(ctx, "tasks.task.get", map[string]any{"taskId": id}, &raw); err != nil {
		return Task{}, err
	}
	return TaskFrom(raw), nil
}

// SetStatus — смена статуса (живьём проверено: 2→3→2 на 12475).
func (a *API) SetStatus(ctx context.Context, id int64, status string) error {
	err := a.C.Call(ctx, "tasks.task.update", map[string]any{
		"taskId": id, "fields": map[string]any{"STATUS": status},
	}, nil)
	if err == nil {
		a.tasksAt = time.Time{}
	}
	return err
}

func (a *API) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// SetDeadline — сдвиг дедлайна: RFC3339 (TZ юзера) или "" чтобы снять.
func (a *API) SetDeadline(ctx context.Context, id int64, dl string) error {
	err := a.C.Call(ctx, "tasks.task.update", map[string]any{
		"taskId": id, "fields": map[string]any{"DEADLINE": dl},
	}, nil)
	if err == nil {
		a.tasksAt = time.Time{}
	}
	return err
}

// ParseDeadline: портал шлёт DEADLINE то RFC3339 (+05:00), то naive в серверной
// зоне — поддерживаем оба.
func ParseDeadline(dl string, loc *time.Location) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, dl); err == nil {
		return t, true
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", dl, loc); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// TaskBuckets — корзины задач по DEADLINE (серверная TZ): просрочка и сегодня.
type TaskBuckets struct {
	Overdue []Task
	Today   []Task
	Later   []Task
	NoDate  []Task
}

// Buckets раскладывает задачи; loc — серверная зона портала.
func Buckets(tasks []Task, loc *time.Location, now time.Time) TaskBuckets {
	var b TaskBuckets
	today := now.In(loc).Format("2006-01-02")
	for _, t := range tasks {
		d, ok := ParseDeadline(t.Deadline, loc)
		if !ok {
			b.NoDate = append(b.NoDate, t)
			continue
		}
		day := d.Format("2006-01-02")
		switch {
		case day < today:
			b.Overdue = append(b.Overdue, t)
		case day == today:
			b.Today = append(b.Today, t)
		default:
			b.Later = append(b.Later, t)
		}
	}
	return b
}
