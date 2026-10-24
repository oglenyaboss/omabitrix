// Package demo — встроенный фейковый портал Битрикс24 для демо TUI:
// всё состояние в памяти, без конфига и сети.
package demo

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	WebhookPath = "/rest/1/demo"
	MyID        = 1
	Portal      = "https://portal.example.com"
)

type Options struct {
	PingAfter time.Duration
}

type server struct {
	mu sync.Mutex

	opt     Options
	started time.Time
	loc     *time.Location
	srv     *httptest.Server

	pingAt time.Time
	pinged bool

	users    []demoUser
	groups   map[int64]*demoGroup
	tasks    map[int64]*demoTask
	taskIDs  []int64
	dialogs  map[string]*demoDialog
	chats    []*demoDialog
	msgIndex map[int64]*demoMessage
	files    map[int64]*demoFile
	notifies []*demoNotify

	nextMsgID  int64
	nextFileID int64
}

func Start(o Options) *httptest.Server {
	s := &server{
		opt:        o,
		started:    time.Now(),
		loc:        time.Local,
		groups:     map[int64]*demoGroup{},
		tasks:      map[int64]*demoTask{},
		dialogs:    map[string]*demoDialog{},
		msgIndex:   map[int64]*demoMessage{},
		files:      map[int64]*demoFile{},
		nextMsgID:  1000,
		nextFileID: 7000,
	}
	if o.PingAfter > 0 {
		s.pingAt = s.started.Add(o.PingAfter)
	}
	s.buildData()

	mux := http.NewServeMux()
	mux.HandleFunc(WebhookPath+"/", s.handleREST)
	mux.HandleFunc("/demo-files/", s.handleFile)
	s.srv = httptest.NewServer(mux)
	return s.srv
}

func (s *server) handleREST(w http.ResponseWriter, r *http.Request) {
	method := strings.TrimPrefix(r.URL.Path, WebhookPath+"/")
	method = strings.TrimSuffix(method, ".json")

	raw, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	var body map[string]any
	if len(strings.TrimSpace(string(raw))) > 0 {
		_ = json.Unmarshal(raw, &body)
	}

	s.mu.Lock()
	s.maybePing()
	var result any
	if method == "batch" {
		result = s.handleBatch(body)
	} else {
		result = s.dispatch(method, flattenParams(body))
	}
	now := time.Now().In(s.loc)
	env := map[string]any{
		"result": result,
		"time": map[string]any{
			"date_start": now.Format(time.RFC3339),
			"start":      now.Unix(),
		},
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(env)
}

func (s *server) handleBatch(body map[string]any) any {
	results := map[string]any{}
	cmds, _ := body["cmd"].(map[string]any)
	for key, v := range cmds {
		cmd, _ := v.(string)
		method, query, _ := strings.Cut(cmd, "?")
		vals, _ := url.ParseQuery(query)
		params := flat{}
		for k, vs := range vals {
			if len(vs) > 0 {
				params[k] = vs[0]
			}
		}
		results[key] = s.dispatch(method, params)
	}
	return map[string]any{"result": results, "result_error": []any{}}
}

func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/demo-files/")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	s.mu.Lock()
	f := s.files[id]
	s.mu.Unlock()
	if f == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(f.Data))
	_, _ = w.Write(f.Data)
}

func (s *server) maybePing() {
	if s.pingAt.IsZero() || s.pinged || time.Now().Before(s.pingAt) {
		return
	}
	s.pinged = true
	d := s.dialogs["2"]
	if d == nil {
		return
	}
	s.appendMessage(d, 2, time.Now(), "Обновила макет экрана бонусов — там теперь конфетти 🎉")
	d.Counter++
	s.bumpChat(d)
}

type flat map[string]string

func (f flat) str(k string) string { return f[k] }

func (f flat) has(k string) bool {
	_, ok := f[k]
	return ok
}

func (f flat) int(k string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(f[k]), 10, 64)
	return n
}

func flattenParams(v any) flat {
	out := flat{}
	flattenValue(v, "", out)
	return out
}

func flattenValue(v any, prefix string, out flat) {
	switch t := v.(type) {
	case nil:
		return
	case map[string]any:
		for k, e := range t {
			p := k
			if prefix != "" {
				p = prefix + "[" + k + "]"
			}
			flattenValue(e, p, out)
		}
	case []any:
		for i, e := range t {
			flattenValue(e, fmt.Sprintf("%s[%d]", prefix, i), out)
		}
	case []string:
		for i, e := range t {
			flattenValue(e, fmt.Sprintf("%s[%d]", prefix, i), out)
		}
	case []int:
		for i, e := range t {
			flattenValue(e, fmt.Sprintf("%s[%d]", prefix, i), out)
		}
	case float64:
		out[prefix] = strconv.FormatInt(int64(t), 10)
	case string:
		out[prefix] = t
	case bool:
		out[prefix] = strconv.FormatBool(t)
	default:
		out[prefix] = fmt.Sprint(t)
	}
}

func (s *server) dispatch(method string, q flat) any {
	switch method {
	case "im.recent.list":
		return s.recentResult()
	case "im.counters.get":
		return s.countersResult()
	case "im.dialog.messages.get":
		return s.historyResult(q)
	case "im.dialog.messages.search":
		return s.searchResult(q)
	case "im.dialog.read":
		if d := s.dialogs[q.str("DIALOG_ID")]; d != nil {
			d.Counter = 0
		}
		return true
	case "im.message.add":
		return s.messageAdd(q)
	case "im.message.update":
		if m := s.msgIndex[q.int("MESSAGE_ID")]; m != nil {
			m.Text = q.str("MESSAGE")
		}
		return true
	case "im.message.delete":
		if m := s.msgIndex[q.int("MESSAGE_ID")]; m != nil {
			m.Deleted = true
		}
		return true
	case "im.message.like":
		s.messageLike(q)
		return true
	case "im.notify.get":
		return s.notifyResult(q)
	case "im.notify.read":
		s.notifyRead(q.str("ID"), q.str("ACTION") == "Y")
		return true
	case "im.notify.read.all":
		for _, n := range s.notifies {
			n.Read = true
		}
		return true
	case "im.v2.File.download":
		id := q.int("fileId")
		return map[string]any{"downloadUrl": s.srv.URL + "/demo-files/" + strconv.FormatInt(id, 10)}
	case "im.v2.File.upload":
		return s.fileUpload(q)
	case "tasks.task.list":
		return s.taskList(q)
	case "task.stages.get":
		return s.stagesResult(q)
	case "sonet_group.get":
		return s.groupResult(q)
	case "tasks.task.get":
		return s.taskGet(q)
	case "task.stages.movetask":
		if t := s.tasks[q.int("id")]; t != nil {
			t.StageID = q.int("stageId")
		}
		return true
	case "tasks.task.update":
		s.taskUpdate(q)
		return true
	case "profile":
		return map[string]any{"ID": strconv.FormatInt(MyID, 10), "ADMIN": true}
	}
	return true
}

func (s *server) recentResult() any {
	items := make([]any, 0, len(s.chats))
	for _, d := range s.chats {
		item := map[string]any{
			"id":       d.ID,
			"dialogId": d.ID,
			"title":    d.Title,
			"counter":  d.Counter,
			"user":     map[string]any{"status": onlineStatus(d.Online)},
		}
		if m := lastMessage(d); m != nil {
			item["message"] = map[string]any{
				"text": m.Text,
				"date": m.Date.In(s.loc).Format(time.RFC3339),
			}
		}
		items = append(items, item)
	}
	return map[string]any{"items": items}
}

func onlineStatus(online bool) string {
	if online {
		return "online"
	}
	return "offline"
}

func lastMessage(d *demoDialog) *demoMessage {
	for i := len(d.Messages) - 1; i >= 0; i-- {
		if !d.Messages[i].Deleted {
			return d.Messages[i]
		}
	}
	return nil
}

func (s *server) countersResult() any {
	var dialogCnt, chatCnt int
	for _, d := range s.chats {
		if d.Group {
			chatCnt += d.Counter
		} else {
			dialogCnt += d.Counter
		}
	}
	notify := s.unreadNotify()
	return map[string]any{
		"TYPE": map[string]any{
			"ALL":       dialogCnt + chatCnt + notify,
			"NOTIFY":    notify,
			"MESSENGER": dialogCnt + chatCnt,
			"CHAT":      chatCnt,
			"DIALOG":    dialogCnt,
			"LINES":     0,
		},
		"CHANNEL_COMMENT": []any{},
	}
}

func (s *server) historyResult(q flat) any {
	d := s.dialogs[q.str("DIALOG_ID")]
	if d == nil {
		return map[string]any{"messages": []any{}, "users": []any{}, "files": []any{}}
	}
	limit := int(q.int("LIMIT"))
	if limit <= 0 {
		limit = 30
	}
	last := q.int("LAST_ID")
	var sel []*demoMessage
	for _, m := range d.Messages {
		if last > 0 && m.ID >= last {
			continue
		}
		sel = append(sel, m)
	}
	if len(sel) > limit {
		sel = sel[len(sel)-limit:]
	}
	return s.messagesPayload(sel, false)
}

func (s *server) searchResult(q flat) any {
	chatID := q.int("CHAT_ID")
	needle := strings.ToLower(q.str("SEARCH_MESSAGE"))
	var found []*demoMessage
	for _, d := range s.dialogs {
		if d.ChatID != chatID {
			continue
		}
		for _, m := range d.Messages {
			if !m.Deleted && strings.Contains(strings.ToLower(m.Text), needle) {
				found = append(found, m)
			}
		}
	}
	return s.messagesPayload(found, true)
}

func (s *server) messagesPayload(sel []*demoMessage, search bool) any {
	users := make([]any, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, map[string]any{"id": u.ID, "name": u.Name})
	}
	fileSet := map[int64]bool{}
	msgs := make([]any, 0, len(sel))
	for i := len(sel) - 1; i >= 0; i-- {
		m := sel[i]
		params := map[string]any{}
		if m.Deleted {
			params["IS_DELETED"] = "Y"
		}
		if m.ReplyID > 0 {
			params["REPLY_ID"] = strconv.FormatInt(m.ReplyID, 10)
		}
		if len(m.Likes) > 0 {
			params["LIKE"] = m.Likes
		}
		if len(m.FileIDs) > 0 {
			params["FILE_ID"] = m.FileIDs
			for _, id := range m.FileIDs {
				fileSet[id] = true
			}
		}
		item := map[string]any{
			"id":        m.ID,
			"date":      m.Date.In(s.loc).Format(time.RFC3339),
			"text":      m.Text,
			"params":    params,
			"author_id": m.AuthorID,
		}
		if search {
			item["authorId"] = m.AuthorID
			delete(item, "author_id")
		}
		msgs = append(msgs, item)
	}
	files := make([]any, 0, len(fileSet))
	for id := range fileSet {
		f := s.files[id]
		if f == nil {
			continue
		}
		files = append(files, map[string]any{
			"id": f.ID, "type": f.Type, "name": f.Name,
			"extension": f.Ext, "size": len(f.Data),
		})
	}
	return map[string]any{"messages": msgs, "users": users, "files": files}
}

func (s *server) messageAdd(q flat) any {
	d := s.dialogs[q.str("DIALOG_ID")]
	if d == nil {
		return s.nextMsgID
	}
	m := s.appendMessage(d, MyID, time.Now(), q.str("MESSAGE"))
	s.bumpChat(d)
	return m.ID
}

func (s *server) messageLike(q flat) {
	m := s.msgIndex[q.int("MESSAGE_ID")]
	if m == nil {
		return
	}
	if q.str("ACTION") == "minus" {
		for i, id := range m.Likes {
			if id == MyID {
				m.Likes = append(m.Likes[:i], m.Likes[i+1:]...)
				return
			}
		}
		return
	}
	for _, id := range m.Likes {
		if id == MyID {
			return
		}
	}
	m.Likes = append(m.Likes, MyID)
}

func (s *server) unreadNotify() int {
	n := 0
	for _, it := range s.notifies {
		if !it.Read {
			n++
		}
	}
	return n
}

func (s *server) notifyResult(q flat) any {
	last := q.int("LAST_ID")
	limit := int(q.int("LIMIT"))
	if limit <= 0 {
		limit = 50
	}
	items := make([]any, 0, len(s.notifies))
	ids := map[int64]bool{}
	for _, n := range s.notifies {
		if last > 0 && n.ID >= last {
			continue
		}
		if len(items) >= limit {
			break
		}
		ids[n.AuthorID] = true
		items = append(items, s.notifyJSON(n))
	}
	users := make([]any, 0, len(ids))
	for _, u := range s.users {
		if ids[u.ID] {
			users = append(users, map[string]any{"id": u.ID, "name": u.Name})
		}
	}
	return map[string]any{
		"notifications":      items,
		"total_unread_count": s.unreadNotify(),
		"users":              users,
	}
}

func (s *server) notifyJSON(n *demoNotify) map[string]any {
	read := "Y"
	if !n.Read {
		read = "N"
	}
	item := map[string]any{
		"id": n.ID, "notify_module": n.Module, "notify_title": n.Title,
		"notify_tag": n.Tag, "text": n.Text,
		"date": n.Date.In(s.loc).Format(time.RFC3339), "notify_read": read,
		"author_id": n.AuthorID, "params": nil,
	}
	if n.Tag != "" {
		item["params"] = map[string]any{
			"COMPONENT_ID": "DefaultEntity",
			"COMPONENT_PARAMS": map[string]any{
				"SUBJECT": n.Text,
			},
		}
	}
	return item
}

func (s *server) notifyRead(id string, read bool) {
	nid, _ := strconv.ParseInt(id, 10, 64)
	for _, n := range s.notifies {
		if n.ID == nid {
			n.Read = read
			return
		}
	}
}

func (s *server) fileUpload(q flat) any {
	name := q.str("fields[name]")
	content := q.str("fields[content]")
	data, _ := base64.StdEncoding.DecodeString(content)
	ext := ""
	if i := strings.LastIndex(name, "."); i >= 0 {
		ext = name[i+1:]
	}
	kind := "file"
	if ext == "png" || ext == "jpg" || ext == "jpeg" || ext == "gif" || ext == "webp" {
		kind = "image"
	}
	s.nextFileID++
	f := &demoFile{ID: s.nextFileID, Type: kind, Name: name, Ext: ext, Data: data}
	s.files[f.ID] = f

	d := s.dialogs[q.str("dialogId")]
	if d == nil {
		return map[string]any{"messageId": 0, "file": map[string]any{"id": f.ID, "type": kind}}
	}
	text := q.str("fields[message]")
	if text == "" {
		text = "[File]"
	}
	m := s.appendMessage(d, MyID, time.Now(), text, f.ID)
	s.bumpChat(d)
	return map[string]any{
		"messageId": m.ID,
		"file":      map[string]any{"id": f.ID, "type": kind},
	}
}

func (s *server) taskList(q flat) any {
	role := ""
	switch {
	case q.has("filter[RESPONSIBLE_ID]"):
		role = "do"
	case q.has("filter[ACCOMPLICE]"):
		role = "help"
	case q.has("filter[AUDITOR]"):
		role = "watch"
	}
	tasks := make([]any, 0, len(s.taskIDs))
	for _, id := range s.taskIDs {
		t := s.tasks[id]
		if t == nil || t.Role != role || closedStatus(t.Status) {
			continue
		}
		tasks = append(tasks, s.taskJSON(t))
	}
	return map[string]any{"tasks": tasks}
}

func closedStatus(status int) bool { return status == 5 || status == 6 || status == 7 }

func (s *server) stagesResult(q flat) any {
	g := s.groups[q.int("entityId")]
	out := map[string]any{}
	if g == nil {
		return out
	}
	for _, st := range g.Stages {
		out[strconv.FormatInt(st.ID, 10)] = map[string]any{
			"ID":    strconv.FormatInt(st.ID, 10),
			"TITLE": st.Title,
			"SORT":  strconv.Itoa(st.Sort),
			"COLOR": st.Color,
		}
	}
	return out
}

func (s *server) groupResult(q flat) any {
	g := s.groups[q.int("FILTER[ID]")]
	if g == nil {
		return []any{}
	}
	return []any{map[string]any{
		"ID":   strconv.FormatInt(g.ID, 10),
		"NAME": g.Name,
	}}
}

func (s *server) taskGet(q flat) any {
	t := s.tasks[q.int("taskId")]
	if t == nil {
		return map[string]any{"task": map[string]any{}}
	}
	out := s.taskJSON(t)
	out["description"] = t.Description
	out["creator"] = map[string]any{"id": t.CreatorID, "name": s.userName(t.CreatorID)}
	return map[string]any{"task": out}
}

func (s *server) taskJSON(t *demoTask) map[string]any {
	dl := ""
	if t.HasDeadline {
		dl = t.Deadline.In(s.loc).Format(time.RFC3339)
	}
	return map[string]any{
		"id": t.ID, "title": t.Title, "status": strconv.Itoa(t.Status),
		"deadline": dl, "chatId": t.ChatID, "newCommentsCount": t.NewComments,
		"groupId": t.GroupID, "stageId": t.StageID,
	}
}

func (s *server) taskUpdate(q flat) {
	t := s.tasks[q.int("taskId")]
	if t == nil {
		return
	}
	if q.has("fields[STATUS]") {
		t.Status = int(q.int("fields[STATUS]"))
	}
	if q.has("fields[DEADLINE]") {
		dl := q.str("fields[DEADLINE]")
		if dl == "" {
			t.HasDeadline = false
			return
		}
		if ts, err := time.Parse(time.RFC3339, dl); err == nil {
			t.Deadline = ts
			t.HasDeadline = true
			return
		}
		if ts, err := time.ParseInLocation("2006-01-02 15:04:05", dl, s.loc); err == nil {
			t.Deadline = ts
			t.HasDeadline = true
		}
	}
}

func (s *server) userName(id int64) string {
	for _, u := range s.users {
		if u.ID == id {
			return u.Name
		}
	}
	return ""
}

func (s *server) appendMessage(d *demoDialog, author int64, at time.Time, text string, fileIDs ...int64) *demoMessage {
	s.nextMsgID++
	m := &demoMessage{
		ID: s.nextMsgID, AuthorID: author, Date: at.In(s.loc),
		Text: text, FileIDs: fileIDs,
	}
	d.Messages = append(d.Messages, m)
	s.msgIndex[m.ID] = m
	return m
}

func (s *server) bumpChat(d *demoDialog) {
	for i, c := range s.chats {
		if c != d {
			continue
		}
		copy(s.chats[1:i+1], s.chats[0:i])
		s.chats[0] = d
		return
	}
}
