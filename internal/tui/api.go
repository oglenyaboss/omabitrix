package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
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

	stages   map[int64][]Stage // канбан проекта по SORT; кэш на сессию
	projects map[int64]string  // имя проекта по id группы
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

// Send отправляет сообщение (replyID>0 — ответ-цитата) и сразу читает диалог
// (optimistic). Портал шлёт result то числом, то {"message_id": N} (живая
// проверка) — парсим оба.
func (a *API) Send(ctx context.Context, dialogID, text string, replyID int64) (int64, error) {
	params := map[string]any{"DIALOG_ID": dialogID, "MESSAGE": text}
	if replyID > 0 {
		params["REPLY_ID"] = replyID
	}
	var raw json.RawMessage
	if err := a.C.Call(ctx, "im.message.add", params, &raw); err != nil {
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

// UpdateMessage правит текст. ПУСТОЙ MESSAGE у портала удалил бы
// сообщение — ловим на своей стороне и запрос не шлём.
func (a *API) UpdateMessage(ctx context.Context, id int64, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("пустой текст удалил бы сообщение")
	}
	return a.C.Call(ctx, "im.message.update", map[string]any{
		"MESSAGE_ID": id, "MESSAGE": text,
	}, nil)
}

// DeleteMessage удаляет сообщение (автор или админ общего чата).
func (a *API) DeleteMessage(ctx context.Context, id int64) error {
	return a.C.Call(ctx, "im.message.delete", map[string]any{
		"MESSAGE_ID": id,
	}, nil)
}

// LikeMessage ставит/снимает «нравится». WITHOUT_CHANGES — реакция уже
// в нужном состоянии, это не ошибка.
func (a *API) LikeMessage(ctx context.Context, id int64, like bool) error {
	action := "minus"
	if like {
		action = "plus"
	}
	err := a.C.Call(ctx, "im.message.like", map[string]any{
		"MESSAGE_ID": id, "ACTION": action,
	}, nil)
	var berr *b24.Error
	if errors.As(err, &berr) && berr.Code == "WITHOUT_CHANGES" {
		return nil
	}
	return err
}

// SearchMessages — поиск по истории чата. Порталу нужен числовой CHAT_ID
// (не DIALOG_ID), строка от 3 символов, LIMIT до 200 (берём 50).
func (a *API) SearchMessages(ctx context.Context, chatID int64, query string) ([]Message, error) {
	if chatID <= 0 {
		return nil, errors.New("поиск недоступен для этого диалога")
	}
	q := strings.TrimSpace(query)
	if len([]rune(q)) < 3 {
		return nil, errors.New("минимум 3 символа")
	}
	var raw json.RawMessage
	if err := a.C.Call(ctx, "im.dialog.messages.search", map[string]any{
		"CHAT_ID": chatID, "SEARCH_MESSAGE": q, "LIMIT": 50,
	}, &raw); err != nil {
		return nil, err
	}
	return SearchFrom(raw, a.My, a.now()), nil
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

// OpenTasks — открытые задачи (не 5/6/7) всех моих ролей, чаты задач
// апендятся списком. Один batch: «делаю» — RESPONSIBLE_ID, «помогаю» —
// ACCOMPLICE, «наблюдаю» — AUDITOR (OR в одном запросе не документирован).
// Без дата-фильтров: корзины просрочка/сегодня считает клиент по DEADLINE
// в серверной TZ — naive-фильтры портала больше не участвуют вовсе.
func (a *API) OpenTasks(ctx context.Context, force bool) ([]Task, error) {
	if !force && time.Since(a.tasksAt) < 20*time.Second && a.allTasks != nil {
		return a.allTasks, nil
	}
	sel := []string{"ID,TITLE,STATUS,DEADLINE,CREATED_BY,CHAT_ID,NEW_COMMENTS_COUNT,GROUP_ID,STAGE_ID"}
	order := map[string]string{"DEADLINE": "asc"}
	taskCmd := func(field string) b24.Cmd {
		return b24.Cmd{Method: "tasks.task.list", Params: map[string]any{
			"filter": map[string]any{field: a.My, "!STATUS": []int{5, 6, 7}},
			"select": sel,
			"order":  order,
		}}
	}
	// ponytail: по 50 задач на роль (одна страница); больше — пагинация start
	res, errs, err := a.C.Batch(ctx, map[string]b24.Cmd{
		"do":    taskCmd("RESPONSIBLE_ID"),
		"help":  taskCmd("ACCOMPLICE"),
		"watch": taskCmd("AUDITOR"),
	})
	if err != nil {
		return nil, err
	}
	// упала любая команда — список был бы неполным, отдаём ошибку как при Call
	for _, role := range []string{"do", "help", "watch"} {
		if e, ok := errs[role]; ok {
			return nil, e
		}
	}
	a.allTasks = a.allTasks[:0]
	seen := map[int64]bool{}
	for _, role := range []string{"do", "help", "watch"} {
		if len(res[role]) == 0 {
			continue
		}
		var page taskListPage
		if err := json.Unmarshal(res[role], &page); err != nil {
			return nil, err
		}
		for _, t := range page.Tasks {
			id := int64(t.ID)
			if seen[id] {
				continue // дубль в нескольких ролях: приоритет do > help > watch
			}
			seen[id] = true
			a.allTasks = append(a.allTasks, Task{
				ID: id, Title: t.Title, Status: string(t.Status),
				Deadline: t.Deadline, ChatID: int64(t.ChatID),
				Role: role, NewComments: int(t.NewComments),
				GroupID: int64(t.GroupID), StageID: int64(t.StageID),
			})
		}
	}
	a.loadMeta(ctx)
	for i := range a.allTasks {
		a.annotate(&a.allTasks[i])
	}
	a.tasksAt = a.now()
	return a.allTasks, nil
}

// loadMeta — стадии и имена проектов для групп, которых ещё нет в кэше:
// один batch, по две команды на группу. Ошибка не роняет список задач —
// доска покажет их без стадии, мета перезапросится при следующей загрузке.
func (a *API) loadMeta(ctx context.Context) {
	cmds := map[string]b24.Cmd{}
	for _, t := range a.allTasks {
		gid := t.GroupID
		if gid <= 0 {
			continue
		}
		if _, ok := a.stages[gid]; ok {
			continue
		}
		// ponytail: batch держит 50 команд — до 25 новых проектов за раз,
		// остальные догрузятся следующим вызовом
		if len(cmds) >= 50 {
			break
		}
		key := strconv.FormatInt(gid, 10)
		cmds["st"+key] = b24.Cmd{Method: "task.stages.get", Params: map[string]any{"entityId": gid}}
		cmds["g"+key] = b24.Cmd{Method: "sonet_group.get", Params: map[string]any{"FILTER": map[string]any{"ID": gid}}}
	}
	if len(cmds) == 0 {
		return
	}
	res, _, err := a.C.Batch(ctx, cmds)
	if err != nil {
		return
	}
	if a.stages == nil {
		a.stages = map[int64][]Stage{}
		a.projects = map[int64]string{}
	}
	for k, raw := range res {
		gid, err := strconv.ParseInt(strings.TrimLeft(k, "stg"), 10, 64)
		if err != nil {
			continue
		}
		if strings.HasPrefix(k, "st") {
			a.stages[gid] = StagesFrom(raw)
			continue
		}
		var groups []struct {
			Name string `json:"NAME"`
		}
		if json.Unmarshal(raw, &groups) == nil && len(groups) > 0 {
			a.projects[gid] = groups[0].Name
		}
	}
}

// annotate — проект и стадия задачи из кэша; стадия 0 у портала значит
// первую стадию канбана проекта.
func (a *API) annotate(t *Task) {
	t.Project = a.projects[t.GroupID]
	st := a.stages[t.GroupID]
	if len(st) == 0 {
		return
	}
	idx := 0
	for i, s := range st {
		if s.ID == t.StageID {
			idx = i
			break
		}
	}
	t.StageID, t.Stage, t.StageColor, t.StageSort = st[idx].ID, st[idx].Title, st[idx].Color, idx
}

// StagesOf — канбан проекта по порядку (для переноса карточки в соседнюю стадию).
func (a *API) StagesOf(groupID int64) []Stage { return a.stages[groupID] }

// MoveStage — перенос задачи в стадию её проекта.
func (a *API) MoveStage(ctx context.Context, taskID, stageID int64) error {
	err := a.C.Call(ctx, "task.stages.movetask", map[string]any{
		"id": taskID, "stageId": stageID,
	}, nil)
	if err == nil {
		a.tasksAt = time.Time{}
	}
	return err
}

// taskListPage — страница tasks.task.list; результат команды batch той же формы.
type taskListPage struct {
	Tasks []struct {
		ID          FlexInt    `json:"id"`
		Title       string     `json:"title"`
		Status      FlexString `json:"status"`
		Deadline    string     `json:"deadline"`
		ChatID      FlexInt    `json:"chatId"`
		NewComments FlexInt    `json:"newCommentsCount"`
		GroupID     FlexInt    `json:"groupId"`
		StageID     FlexInt    `json:"stageId"`
	} `json:"tasks"`
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

// TaskBuckets — корзины задач по DEADLINE (серверная TZ): просрочка и сегодня;
// Help/Watch — роли «помогаю»/«наблюдаю» (без дедлайн-раскладки).
type TaskBuckets struct {
	Overdue []Task
	Today   []Task
	Later   []Task
	NoDate  []Task
	Help    []Task
	Watch   []Task
}

// Buckets раскладывает задачи; loc — серверная зона портала.
func Buckets(tasks []Task, loc *time.Location, now time.Time) TaskBuckets {
	var b TaskBuckets
	today := now.In(loc).Format("2006-01-02")
	for _, t := range tasks {
		switch t.Role {
		case "help":
			b.Help = append(b.Help, t)
			continue
		case "watch":
			b.Watch = append(b.Watch, t)
			continue
		}
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
