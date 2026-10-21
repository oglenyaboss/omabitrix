// Package daemon — цикл опроса портала: один batch, правила, тосты, снапшот для бара.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"omabitrix/internal/b24"
	"omabitrix/internal/config"
	"omabitrix/internal/notify"
	"omabitrix/internal/rules"
	"omabitrix/internal/state"
)

const taskFields = "ID,TITLE,STATUS,subStatus,DEADLINE,CHANGED_DATE,CREATED_DATE,NEW_COMMENTS_COUNT"

type Daemon struct {
	Cfg    config.Config
	Client *b24.Client
	State  *state.State
	Send   notify.Sender
	Now    func() time.Time
	Loc    *time.Location
}

func (d *Daemon) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Daemon) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(d.Cfg.PollIntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		if err := d.Cycle(ctx); err != nil {
			log.Printf("cycle: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type roleNames struct {
	key   string // ключ в state.LastChanged
	field string // поле фильтра
}

var roles = []roleNames{
	{"resp", "RESPONSIBLE_ID"},
	{"acc", "ACCOMPLICE"},
	{"aud", "AUDITOR"},
}

// cycleCmds собирает все запросы цикла в один batch (лимит 2 req/s соблюдён).
func (d *Daemon) cycleCmds(now time.Time) map[string]b24.Cmd {
	cmds := map[string]b24.Cmd{
		"counters": {Method: "im.counters.get"},
		"im":       {Method: "im.v2.Event.get", Params: map[string]any{"offset": d.State.ImV2Offset}},
		"recent":   {Method: "im.recent.list", Params: map[string]any{"UNREAD_ONLY": "Y", "LIMIT": 200}},
	}
	today := now.In(d.filterLoc()).Format("2006-01-02")
	for _, r := range roles {
		cmds["ov_"+r.key] = b24.Cmd{Method: "tasks.task.list", Params: taskParams(
			map[string]any{r.field: d.Cfg.UserID, "STATUS": -1}, nil)}
		cmds["today_"+r.key] = b24.Cmd{Method: "tasks.task.list", Params: taskParams(
			map[string]any{
				r.field:      d.Cfg.UserID,
				"!STATUS":    []int{5, 6, 7},
				">=DEADLINE": today,
				"<=DEADLINE": today + " 23:59:59",
			}, nil)}
		last := d.State.LastChanged[r.key]
		if last == "" {
			last = now.In(d.filterLoc()).Format("2006-01-02 15:04:05") // первый цикл: baseline
		}
		cmds["delta_"+r.key] = b24.Cmd{Method: "tasks.task.list", Params: taskParams(
			map[string]any{r.field: d.Cfg.UserID, ">CHANGED_DATE": last},
			map[string]string{"CHANGED_DATE": "desc"})}
	}
	return cmds
}

func taskParams(filter map[string]any, order map[string]string) map[string]any {
	p := map[string]any{
		"filter": filter,
		"select": []string{taskFields},
	}
	if order != nil {
		p["order"] = order
	}
	return p
}

func (d *Daemon) loc() *time.Location {
	if d.Loc != nil {
		return d.Loc
	}
	return d.Cfg.Location()
}

// filterLoc — таймзона, в которой портал трактует наивные даты фильтров:
// серверная (+03 на portal.example.com), НЕ пользовательская (+05)
func (d *Daemon) filterLoc() *time.Location {
	return d.Client.ServerLoc()
}

func decodeInto(raw json.RawMessage, v any) {
	if len(raw) == 0 {
		return
	}
	if err := json.Unmarshal(raw, v); err != nil {
		log.Printf("decode: %v", err)
	}
}

// Cycle — один polling-цикл. Ошибки портала -> снапшот с Error (бар покажет ⚠️).
func (d *Daemon) Cycle(ctx context.Context) error {
	now := d.now()
	loc := d.loc()

	if !d.State.Subscribed {
		if err := d.Client.ImV2Subscribe(ctx); err != nil {
			return d.fail(err)
		}
		d.State.Subscribed = true
	}

	res, errs, err := d.Client.Batch(ctx, d.cycleCmds(now))
	if err != nil {
		return d.fail(err)
	}
	// тихая смерть запрещена: если все ключевые команды упали с ошибками —
	// это мёртвый вебхук/портал, бар должен показать ⚠️, а не нули
	if len(errs) > 0 {
		critical := 0
		for _, k := range []string{"counters", "im", "recent", "ov_resp"} {
			if _, ok := errs[k]; ok {
				critical++
			}
		}
		if critical == 4 {
			var first error
			for _, e := range errs {
				first = e
				break
			}
			return d.fail(fmt.Errorf("все ключевые команды упали: %v", first))
		}
	}
	for k, e := range errs {
		log.Printf("batch[%s]: %v", k, e)
	}

	var counters b24.CountersResult
	decodeInto(res["counters"], &counters)

	// envelope декодируем всегда: offset обязан двигаться даже мимо битых
	// событий, иначе демон вечно перечитывает отравленную страницу.
	// События — по одному: одно уродливое не теряет всю страницу.
	var imRaw struct {
		Events     []json.RawMessage `json:"events"`
		HasMore    bool              `json:"hasMore"`
		NextOffset int               `json:"nextOffset"`
	}
	decodeInto(res["im"], &imRaw)
	var im struct {
		Events     []b24.Event
		HasMore    bool
		NextOffset int
	}
	im.HasMore, im.NextOffset = imRaw.HasMore, imRaw.NextOffset
	for _, raw := range imRaw.Events {
		var ev b24.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			log.Printf("im event: %v: %.200s", err, raw)
			continue
		}
		im.Events = append(im.Events, ev)
	}

	var recent struct {
		Recent []b24.Recent `json:"items"` // v2-формат: items, не recent (живая проверка)
	}
	decodeInto(res["recent"], &recent)

	tasks := map[string][]b24.Task{}
	for _, r := range roles {
		for _, kind := range []string{"ov", "today", "delta"} {
			var page struct {
				Tasks []b24.Task `json:"tasks"`
			}
			decodeInto(res[kind+"_"+r.key], &page)
			tasks[kind+"_"+r.key] = page.Tasks
		}
	}

	dnd := rules.DNDActive(now.In(loc), d.Cfg.DND.Start, d.Cfg.DND.End)
	wasBaseline := d.State.BaselineDone
	var toasts []rules.Toast

	// 1) события сообщений/комментариев
	for _, ev := range im.Events {
		t := rules.Classify(ev, d.Cfg.UserID, d.Cfg.Portal)
		if t == nil || dnd {
			continue
		}
		if d.State.ShouldOnce(rules.MsgKey(int(ev.EventID), now)) {
			toasts = append(toasts, *t)
		}
	}
	if im.NextOffset > 0 {
		d.State.ImV2Offset = im.NextOffset
	}

	// 2) дельта задач + периодический полный проход (catch-up комментариев)
	delta := mergeUnique(tasks["delta_resp"], tasks["delta_acc"], tasks["delta_aud"])
	if len(delta) > 0 {
		log.Printf("дельта: %d задач", len(delta))
	}
	// NEW_COMMENTS_COUNT не фильтруется порталом (проверено живьём), а комментарии
	// не бампают CHANGED_DATE — раз в час полный проход по ролям с пагинацией
	d.State.Cycles++
	fullPassEvery := 3600 / d.Cfg.PollIntervalSec
	if fullPassEvery < 1 {
		fullPassEvery = 1
	}
	if wasBaseline && d.State.Cycles%fullPassEvery == 0 {
		for _, r := range roles {
			ts, err := d.Client.Tasks(ctx, b24.TaskQuery{
				Filter: map[string]any{r.field: d.Cfg.UserID, "!STATUS": []int{5, 6, 7}},
				Fields: []string{taskFields},
			})
			if err != nil {
				log.Printf("полный проход %s: %v", r.key, err)
				continue
			}
			delta = mergeUnique(delta, ts)
		}
	}
	for _, task := range delta {
		if !d.State.KnownTasks[int(task.ID)] {
			d.State.KnownTasks[int(task.ID)] = true
			// тост только для действительно свежих: baseline сеет KnownTasks неполно,
			// старая задача при первом попадании запоминается молча
			if wasBaseline && !dnd && rules.IsRecentlyCreated(task, now) &&
				d.State.ShouldOnce(rules.DayKey(rules.KindTaskNew, strconv.Itoa(int(task.ID)), now)) {
				toasts = append(toasts, rules.TaskNewToast(d.Cfg.Portal, d.Cfg.UserID, task))
			}
		}
		// catch-up комментариев после простоя (im.v2-очередь живёт 24 ч)
		if wasBaseline && int(task.NewCommentsCount) > d.State.NewComments[int(task.ID)] && int(task.NewCommentsCount) > 0 && !dnd {
			if d.State.ShouldOnce(rules.DayKey(rules.KindComment, strconv.Itoa(int(task.ID)), now)) {
				toasts = append(toasts, rules.Toast{
					Kind:    rules.KindComment,
					Title:   "Комментарии: " + task.Title,
					Body:    strconv.Itoa(int(task.NewCommentsCount)) + " новых",
					Urgency: "normal",
					URL:     rules.TaskURL(d.Cfg.Portal, d.Cfg.UserID, int(task.ID)),
				})
			}
		}
		d.State.NewComments[int(task.ID)] = int(task.NewCommentsCount)
	}
	// watermark дельты двигаем строго по своей роли: merged-дельта двигала бы
	// watermark чужих ролей и теряла их изменения в окне
	for _, r := range roles {
		for _, task := range tasks["delta_"+r.key] {
			if ts := task.ChangedDate.In(d.filterLoc()).Format("2006-01-02 15:04:05"); ts > d.State.LastChanged[r.key] {
				d.State.LastChanged[r.key] = ts
			}
		}
	}

	// 3) просрочки и дедлайны
	overdue := mergeUnique(tasks["ov_resp"], tasks["ov_acc"], tasks["ov_aud"])
	today := mergeUnique(tasks["today_resp"], tasks["today_acc"], tasks["today_aud"])
	for _, task := range overdue {
		d.State.KnownTasks[int(task.ID)] = true
		if wasBaseline && !dnd && d.State.ShouldOnce(rules.DayKey(rules.KindOverdue, strconv.Itoa(int(task.ID)), now)) {
			toasts = append(toasts, rules.OverdueToast(d.Cfg.Portal, d.Cfg.UserID, task))
		}
	}
	for _, task := range today {
		d.State.KnownTasks[int(task.ID)] = true
		if wasBaseline && rules.DeadlineSoon(task, now) && !dnd &&
			d.State.ShouldOnce(rules.DayKey(rules.KindDeadlineSoon, strconv.Itoa(int(task.ID)), now)) {
			toasts = append(toasts, rules.DeadlineSoonToast(d.Cfg.Portal, d.Cfg.UserID, task))
		}
	}

	// 4) отправка (после Collapse — буря превращается в summary)
	for _, t := range rules.Collapse(toasts) {
		if err := d.Send.Send(ctx, t); err != nil {
			log.Printf("send %s: %v", t.Kind, err)
		}
	}

	// 5) снапшот для бара
	// MESSENGER — весь непрочитанный мессенджер (лички + чаты + комменты
	// коллабов в CHANNEL_COMMENT); CHAT+DIALOG их не видят (живой факт
	// portal.example.com: ALL=2 при CHAT=0/DIALOG=0). max — страховка от порталов
	// без MESSENGER в ответе.
	chats := counters.TYPE.Messenger.Total
	if old := counters.TYPE.Chat.Total + counters.TYPE.Dialog.Total; old > chats {
		chats = old
	}
	snap := state.Snapshot{
		Chats:     chats,
		Notify:    counters.TYPE.Notify,
		Overdue:   len(overdue),
		Today:     len(today),
		UpdatedAt: now.Format(time.RFC3339),
	}
	if d.Cfg.CountLines {
		snap.Chats += counters.TYPE.Lines.Total
	}
	ignore := map[string]bool{}
	for _, id := range d.Cfg.IgnoreChats {
		ignore[id] = true
	}
	for _, r := range recent.Recent {
		if ignore[string(r.ID)] {
			continue
		}
		snap.Unread = append(snap.Unread, state.UnreadChat{DialogID: string(r.ID), Title: r.Title, Counter: r.Counter})
	}
	snap.OverdueTasks = taskRefs(overdue, 15)
	snap.TodayTasks = taskRefs(today, 15)

	d.State.BaselineDone = true
	// инициализация watermark дельты: без этого первый цикл оставляет LastChanged
	// пустым и каждый следующий цикл фильтрует «> своего now», теряя ВСЕ изменения
	// между циклами (нашёл оракул)
	for _, r := range roles {
		if d.State.LastChanged[r.key] == "" {
			d.State.LastChanged[r.key] = now.In(d.filterLoc()).Format("2006-01-02 15:04:05")
		}
	}
	d.State.PruneNotified(now, 14)

	if err := snap.Save(d.Cfg.SnapshotPath); err != nil {
		log.Printf("snapshot: %v", err)
	}
	if err := d.State.Save(d.Cfg.StatePath); err != nil {
		return err
	}
	return nil
}

// fail — портал недоступен/вебхук истёк: бар обязан показать ⚠️, молчание запрещено.
func (d *Daemon) fail(err error) error {
	snap := state.Snapshot{Error: fmt.Sprintf("%v", err), UpdatedAt: d.now().Format(time.RFC3339)}
	if serr := snap.Save(d.Cfg.SnapshotPath); serr != nil {
		log.Printf("snapshot(fail): %v", serr)
	}
	return err
}

func mergeUnique(lists ...[]b24.Task) []b24.Task {
	seen := map[int]bool{}
	var out []b24.Task
	for _, list := range lists {
		for _, t := range list {
			if seen[int(t.ID)] {
				continue
			}
			seen[int(t.ID)] = true
			out = append(out, t)
		}
	}
	return out
}

func taskRefs(tasks []b24.Task, limit int) []state.TaskRef {
	var out []state.TaskRef
	for i, t := range tasks {
		if i >= limit {
			break
		}
		ref := state.TaskRef{ID: int(t.ID), Title: t.Title}
		if t.Deadline != nil {
			ref.Deadline = t.Deadline.Format("15:04 02.01")
		}
		out = append(out, ref)
	}
	return out
}
