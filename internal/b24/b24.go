// Package b24 — тонкий клиент REST Битрикс24 поверх входящего вебхука.
// Все факты структуры ответов — из гейта №0 на portal.example.com (docs/SPEC.md).
package b24

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	Base string
	HTTP *http.Client
	// RateLimit — минимальная пауза между HTTP-запросами (вне batch);
	// лимит портала 2 req/s, пагинация полного прохода должна в него укладываться.
	RateLimit time.Duration

	mu   sync.Mutex
	last time.Time

	srvMu     sync.Mutex
	serverLoc *time.Location // из time.date_start ответов; наивные даты фильтров портал трактует в серверной зоне

	skewMu sync.Mutex
	skew   *time.Duration // минимальная оценка: местное время − серверное (time.start)
}

// ServerLoc — таймзона сервера (проверяется живьём: portal.example.com — сервер +03,
// задачи приходят в +05 юзера; фильтры >CHANGED_DATE/DEADLINE — в серверской).
func (c *Client) ServerLoc() *time.Location {
	c.srvMu.Lock()
	defer c.srvMu.Unlock()
	if c.serverLoc != nil {
		return c.serverLoc
	}
	return time.UTC
}

// Skew — насколько часы сервера опережают местные (отрицательное — сервер
// отстаёт). Даты портала прошиты теми же часами, что и time.start ответа,
// поэтому сдвиг на Skew() приводит их к часам ПК. Храним минимум по
// наблюдениям: каждая оценка завышена на сетевую задержку, для минутной
// точности отображения этого достаточно.
func (c *Client) Skew() time.Duration {
	c.skewMu.Lock()
	defer c.skewMu.Unlock()
	if c.skew == nil {
		return 0
	}
	return *c.skew
}

func (c *Client) noteSkew(startEpoch float64) {
	if startEpoch == 0 {
		return
	}
	d := time.Since(time.Unix(int64(startEpoch), 0))
	c.skewMu.Lock()
	defer c.skewMu.Unlock()
	if c.skew == nil || d < *c.skew {
		c.skew = &d
	}
}

func New(webhookURL string) *Client {
	return &Client{
		Base: strings.TrimRight(webhookURL, "/"),
		HTTP: &http.Client{Timeout: 30 * time.Second},
	}
}

type Error struct {
	Method      string
	Code        string
	Description string
}

func (e *Error) Error() string {
	return fmt.Sprintf("b24 %s: %s %s", e.Method, e.Code, e.Description)
}

// Call — публичная обёртка для разовых методов (TUI-клиент).
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	return c.call(ctx, method, params, out)
}

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	if c.RateLimit > 0 {
		c.mu.Lock()
		if wait := time.Until(c.last.Add(c.RateLimit)); wait > 0 {
			time.Sleep(wait)
		}
		c.last = time.Now()
		c.mu.Unlock()
	}
	var body io.Reader
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/"+method+".json", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
		Descr  string          `json:"error_description"`
		Time   struct {
			DateStart string  `json:"date_start"`
			Start     float64 `json:"start"`
		} `json:"time"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("b24 %s: %w: %.200s", method, err, raw)
	}
	c.noteSkew(env.Time.Start)
	if env.Error != "" {
		return &Error{Method: method, Code: env.Error, Description: env.Descr}
	}
	if env.Time.DateStart != "" {
		if ts, err := time.Parse(time.RFC3339, env.Time.DateStart); err == nil {
			c.srvMu.Lock()
			if c.serverLoc == nil {
				c.serverLoc = ts.Location()
			}
			c.srvMu.Unlock()
		}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}

// Cmd — команда для batch.
type Cmd struct {
	Method string
	Params any
}

// errMap/resultMap терпят PHP-пустоту: {} может прийти как [].
type batchErr struct {
	Error string `json:"error"`
	Descr string `json:"error_description"`
}

type errMap map[string]batchErr

func (m *errMap) UnmarshalJSON(b []byte) error {
	t := strings.TrimSpace(string(b))
	if t == "" || t == "null" || t[0] == '[' {
		return nil
	}
	var mm map[string]batchErr
	if err := json.Unmarshal(b, &mm); err != nil {
		return err
	}
	*m = mm
	return nil
}

type resultMap map[string]json.RawMessage

func (m *resultMap) UnmarshalJSON(b []byte) error {
	t := strings.TrimSpace(string(b))
	if t == "" || t == "null" || t[0] == '[' {
		return nil
	}
	var mm map[string]json.RawMessage
	if err := json.Unmarshal(b, &mm); err != nil {
		return err
	}
	*m = mm
	return nil
}

// Batch выполняет до 50 команд одним HTTP-запросом.
// Коробка понимает только строковую форму команд: cmd[k]="method?param=value" (объектную молча игнорирует — проверено на portal.example.com).
func (c *Client) Batch(ctx context.Context, cmds map[string]Cmd) (map[string]json.RawMessage, map[string]error, error) {
	cmd := map[string]string{}
	for k, v := range cmds {
		s := v.Method
		if q := encodeParams(v.Params); q != "" {
			s += "?" + q
		}
		cmd[k] = s
	}
	// call() уже снимает верхний "result", здесь остаются result/result_error batch-обёртки
	var out struct {
		Result      resultMap `json:"result"`
		ResultError errMap    `json:"result_error"`
	}
	if err := c.call(ctx, "batch", map[string]any{"halt": 0, "cmd": cmd}, &out); err != nil {
		return nil, nil, err
	}
	errs := map[string]error{}
	for k, e := range out.ResultError {
		errs[k] = &Error{Method: cmds[k].Method, Code: e.Error, Description: e.Descr}
	}
	return out.Result, errs, nil
}

// encodeParams — PHP-стиль ключей: filter[STATUS]=-1&select[0]=ID&order[CHANGED_DATE]=desc
func encodeParams(params any) string {
	if params == nil {
		return ""
	}
	var parts []string
	encodeValue(params, "", &parts)
	return strings.Join(parts, "&")
}

func encodeValue(v any, prefix string, out *[]string) {
	switch t := v.(type) {
	case nil:
		return
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := k
			if prefix != "" {
				p = prefix + "[" + k + "]"
			}
			encodeValue(t[k], p, out)
		}
	case map[string]string:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := k
			if prefix != "" {
				p = prefix + "[" + k + "]"
			}
			encodeValue(t[k], p, out)
		}
	case []any:
		for i, e := range t {
			encodeValue(e, fmt.Sprintf("%s[%d]", prefix, i), out)
		}
	case []string:
		for i, e := range t {
			encodeValue(e, fmt.Sprintf("%s[%d]", prefix, i), out)
		}
	case []int:
		for i, e := range t {
			encodeValue(e, fmt.Sprintf("%s[%d]", prefix, i), out)
		}
	case string:
		*out = append(*out, url.QueryEscape(prefix)+"="+url.QueryEscape(t))
	case bool:
		n := "0"
		if t {
			n = "1"
		}
		*out = append(*out, url.QueryEscape(prefix)+"="+n)
	default:
		*out = append(*out, url.QueryEscape(prefix)+"="+url.QueryEscape(fmt.Sprint(v)))
	}
}

// — im.v2 события —

// FlexInt — число приходит числом или строкой (коробка отдаёт id/статусы задач строками).
type FlexInt int

func (f *FlexInt) UnmarshalJSON(b []byte) error {
	t := strings.TrimSpace(string(b))
	if t == "" || t == "null" {
		*f = 0
		return nil
	}
	if t[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("flexint %q: %w", s, err)
		}
		*f = FlexInt(n)
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = FlexInt(n)
	return nil
}

type EntityLink struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	ID   string `json:"id"`
}

type EventMessage struct {
	ID       FlexInt   `json:"id"`
	ChatID   FlexInt   `json:"chatId"`
	AuthorID FlexInt   `json:"authorId"`
	Date     time.Time `json:"date"`
	Text     string    `json:"text"`
}

type EventChat struct {
	ID         FlexInt     `json:"id"`
	DialogID   string      `json:"dialogId"`
	Type       string      `json:"type"`       // "tasksTask", "chat", ...
	EntityType string      `json:"entityType"` // "TASKS_TASK"
	EntityID   string      `json:"entityId"`
	Name       string      `json:"name"`
	EntityLink *EntityLink `json:"entityLink"`
}

// EventUser — автор сообщения; приходит прямо в payload (проверено живьём).
type EventUser struct {
	ID        FlexInt `json:"id"`
	Name      string  `json:"name"`
	FirstName string  `json:"firstName"`
	LastName  string  `json:"lastName"`
}

// UnmarshalJSON терпит PHP-пустоту: в некоторых событиях data.user приходит
// как [] вместо объекта — без этого падал декод всей страницы событий
// (живой портал, journalctl 19.10.2026).
func (u *EventUser) UnmarshalJSON(b []byte) error {
	if t := strings.TrimSpace(string(b)); len(t) > 0 && t[0] == '[' {
		*u = EventUser{}
		return nil
	}
	type plain EventUser
	return json.Unmarshal(b, (*plain)(u))
}

// DisplayName — «Имя Фамилия»; fallback на любое доступное.
func (u EventUser) DisplayName() string {
	if u.Name != "" {
		return u.Name
	}
	if u.FirstName != "" || u.LastName != "" {
		return strings.TrimSpace(u.FirstName + " " + u.LastName)
	}
	return ""
}

type Event struct {
	EventID FlexInt `json:"eventId"`
	Type    string  `json:"type"` // ONIMV2MESSAGEADD | ONIMV2MESSAGEDELETE | ...
	Data    struct {
		Message EventMessage `json:"message"`
		Chat    EventChat    `json:"chat"`
		User    EventUser    `json:"user"`
	} `json:"data"`
}

func (c *Client) ImV2Subscribe(ctx context.Context) error {
	return c.call(ctx, "im.v2.Event.subscribe", nil, nil)
}

func (c *Client) ImV2Get(ctx context.Context, offset int) ([]Event, int, bool, error) {
	var out struct {
		Events     []Event `json:"events"`
		HasMore    bool    `json:"hasMore"`
		NextOffset int     `json:"nextOffset"`
	}
	if err := c.call(ctx, "im.v2.Event.get", map[string]any{"offset": offset}, &out); err != nil {
		return nil, 0, false, err
	}
	return out.Events, out.NextOffset, out.HasMore, nil
}

// — счётчики —
// CHAT/DIALOG бывают и числом (0), и картой {chatId: n} — гейт показал оба.

type FlexCount struct {
	Total int
	ByID  map[string]int
}

func (f *FlexCount) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '{' {
		m := map[string]int{}
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		f.ByID = m
		for _, v := range m {
			f.Total += v
		}
		return nil
	}
	if len(b) > 0 && (b[0] == 'n' || b[0] == 'N') { // null
		return nil
	}
	return json.Unmarshal(b, &f.Total)
}

type Counters struct {
	All       int       `json:"ALL"`
	Notify    int       `json:"NOTIFY"`
	Messenger FlexCount `json:"MESSENGER"` // весь мессенджер: чаты + лички + комменты коллабов
	Chat      FlexCount `json:"CHAT"`
	Dialog    FlexCount `json:"DIALOG"`
	Lines     FlexCount `json:"LINES"`
	// CollabUnread — непрочитанное в под-чатах коллабов (CHANNEL_COMMENT):
	// id родительского чата → сумма по под-чатам. Портал считает их
	// отдельно от CHAT/DIALOG, поэтому бейдж без них врёт.
	CollabUnread map[string]int `json:"-"`
}

// CountersResult — весь ответ im.counters.get: CHANNEL_COMMENT лежит рядом
// с TYPE, не внутри (живой факт portal.example.com) — поэтому свой UnmarshalJSON.
func (r *CountersResult) UnmarshalJSON(b []byte) error {
	var wire struct {
		TYPE           Counters        `json:"TYPE"`
		ChannelComment json.RawMessage `json:"CHANNEL_COMMENT"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return err
	}
	r.TYPE = wire.TYPE
	if len(wire.ChannelComment) > 0 && wire.ChannelComment[0] == '{' {
		var subs map[string]map[string]int
		if err := json.Unmarshal(wire.ChannelComment, &subs); err != nil {
			return err
		}
		r.TYPE.CollabUnread = make(map[string]int, len(subs))
		for parent, m := range subs {
			n := 0
			for _, v := range m {
				n += v
			}
			r.TYPE.CollabUnread[parent] = n
		}
	}
	return nil
}

type CountersResult struct {
	TYPE Counters
}

func (c *Client) Counters(ctx context.Context) (Counters, error) {
	var out CountersResult
	if err := c.call(ctx, "im.counters.get", nil, &out); err != nil {
		return Counters{}, err
	}
	return out.TYPE, nil
}

// — недавние чаты —

// FlexString — id приходит числом (личка) или строкой ("chat123").
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = FlexString(n.String())
	return nil
}

type Recent struct {
	ID      FlexString `json:"id"`
	Title   string     `json:"title"`
	Counter int        `json:"counter"`
	Type    string     `json:"type"`
}

func (c *Client) RecentUnread(ctx context.Context, limit int) ([]Recent, error) {
	var out struct {
		Recent []Recent `json:"items"`
	}
	if err := c.call(ctx, "im.recent.list", map[string]any{"UNREAD_ONLY": "Y", "LIMIT": limit}, &out); err != nil {
		return nil, err
	}
	return out.Recent, nil
}

// — задачи —

type TaskGroup struct {
	ID   FlexInt `json:"id"`
	Name string  `json:"name"`
}

type Task struct {
	ID               FlexInt    `json:"id"`
	Title            string     `json:"title"`
	Status           FlexInt    `json:"status"`
	SubStatus        FlexInt    `json:"subStatus"`
	Deadline         *time.Time `json:"deadline"`
	ChangedDate      time.Time  `json:"changedDate"`
	CreatedDate      time.Time  `json:"createdDate"`
	NewCommentsCount FlexInt    `json:"newCommentsCount"`
	ChatID           FlexInt    `json:"chatId"`
	Group            *TaskGroup `json:"group"`
}

type TaskQuery struct {
	Filter map[string]any
	Order  map[string]string
	Fields []string
}

type taskPage struct {
	Tasks []Task `json:"tasks"`
	Total int    `json:"total"`
}

// Tasks — с пагинацией по 50 (вне batch; для дельт/дедлайнов в демоне идёт через Batch по одной странице).
func (c *Client) Tasks(ctx context.Context, q TaskQuery) ([]Task, error) {
	var all []Task
	for start := 0; ; start += 50 {
		page, err := c.tasksPage(ctx, q, start)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Tasks...)
		if len(all) >= page.Total || len(page.Tasks) == 0 {
			return all, nil
		}
	}
}

func (c *Client) tasksPage(ctx context.Context, q TaskQuery, start int) (taskPage, error) {
	params := map[string]any{"start": start}
	if q.Filter != nil {
		params["filter"] = q.Filter
	}
	if q.Order != nil {
		params["order"] = q.Order
	}
	if q.Fields != nil {
		params["select"] = q.Fields
	}
	var page taskPage
	if err := c.call(ctx, "tasks.task.list", params, &page); err != nil {
		return page, err
	}
	return page, nil
}

// Profile — для doctor (ID приходит строкой).
func (c *Client) Profile(ctx context.Context) (id int, admin bool, err error) {
	var out struct {
		ID    string `json:"ID"`
		ADMIN bool   `json:"ADMIN"`
	}
	if err := c.call(ctx, "profile", nil, &out); err != nil {
		return 0, false, err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out.ID))
	return n, out.ADMIN, nil
}
