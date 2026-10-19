package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// taskChatPrefixes — конвенция команды: чаты проектов задач.
// Новый префикс замусорит список чатов — правится в одном месте.
// ponytail: вынести в config.toml, когда префиксов станет больше пяти.
var taskChatPrefixes = []string{"SRV.", "ADS.", "CLS."}

var bbRe = regexp.MustCompile(`\[[^\]]*\]`)

// mentionRe — упоминание в тексте: [USER=75]Имя[/USER], [user=all]Все[/user].
var mentionRe = regexp.MustCompile(`(?i)\[USER=(\d+|all)\](.*?)\[/USER\]`)

// applyMentions: упоминания → @Имя (до StripBB); true — упомянут я (мой id или all).
func applyMentions(text string, myID int64) (string, bool) {
	me := false
	out := mentionRe.ReplaceAllStringFunc(text, func(m string) string {
		g := mentionRe.FindStringSubmatch(m)
		if strings.EqualFold(g[1], "all") || g[1] == strconv.FormatInt(myID, 10) {
			me = true
		}
		return "@" + g[2]
	})
	return out, me
}

// StripBB убирает BB-коды ([b]текст[/b], [USER=1]Имя[/USER]) и тримит.
func StripBB(s string) string {
	return strings.TrimSpace(bbRe.ReplaceAllString(s, ""))
}

// FmtTime: HH:MM если сегодня (локально), иначе DD.MM.
func FmtTime(iso string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return ""
	}
	t = t.Local()
	if t.Format("2006-01-02") == now.Format("2006-01-02") {
		return t.Format("15:04")
	}
	return t.Format("02.01")
}

// DayInfo: ключ дня и подпись-разделитель (сегодня/вчера/DD.MM).
func DayInfo(iso string, now time.Time) (key, label string) {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return "", ""
	}
	t = t.Local()
	key = t.Format("2006-01-02")
	switch key {
	case now.Format("2006-01-02"):
		return key, "сегодня"
	case now.AddDate(0, 0, -1).Format("2006-01-02"):
		return key, "вчера"
	}
	return key, t.Format("02.01")
}

// IsTaskChat — чат задачи по префиксу проекта.
func IsTaskChat(title string) bool {
	for _, p := range taskChatPrefixes {
		if len(title) >= len(p) && title[:len(p)] == p {
			return true
		}
	}
	return false
}

// parse-структуры под сырые ответы портала.

type rawRecentItem struct {
	// id: у групповых "chatNNNN" (строка), у личных — число user_id,
	// у битых — null. dialogId дублирует не всегда (живой портал).
	ID      FlexString `json:"id"`
	Dial    FlexString `json:"dialogId"`
	ChatID  FlexInt    `json:"chat_id"`
	Title   string     `json:"title"`
	Counter int        `json:"counter"`
	Message *struct {
		Text string `json:"text"`
		Date string `json:"date"`
	} `json:"message"`
	User *struct {
		Status string `json:"status"`
	} `json:"user"`
}

type rawRecentResult struct {
	Items []rawRecentItem `json:"items"`
}

// FlexInt — число, которое портал иногда присылает строкой (message_id).
type FlexInt int64

func (f *FlexInt) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	s := strings.Trim(string(b), `"`)
	var v int64
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return err
	}
	*f = FlexInt(v)
	return nil
}

// FlexString — строка, которую портал иногда присылает числом (status).
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	s := string(b)
	if len(s) >= 2 && s[0] == '"' {
		s = s[1 : len(s)-1]
	}
	*f = FlexString(s)
	return nil
}

// ChatsFrom — im.recent.list → чаты, отсортированные: непрочитанные вверх, дальше по свежести.
func ChatsFrom(raw []byte, now time.Time) []Chat {
	var res rawRecentResult
	_ = json.Unmarshal(raw, &res)
	now = now.Local()
	out := make([]Chat, 0, len(res.Items))
	for _, it := range res.Items {
		dlg := string(it.Dial)
		if dlg == "" {
			dlg = string(it.ID)
		}
		if dlg == "" || dlg == "null" {
			continue // битый элемент портала
		}
		c := Chat{DialogID: dlg, ChatID: int64(it.ChatID), Title: it.Title, Counter: it.Counter}
		if c.Title == "" {
			c.Title = dlg
		}
		if it.Message != nil {
			c.Preview = StripBB(it.Message.Text)
			c.Time = FmtTime(it.Message.Date, now)
			if t, err := time.Parse(time.RFC3339, it.Message.Date); err == nil {
				c.Stamp = t.UnixMilli()
			}
		}
		if it.User != nil && it.User.Status == "online" {
			c.Online = true
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Counter > 0) != (out[j].Counter > 0) {
			return out[i].Counter > 0
		}
		return out[i].Stamp > out[j].Stamp
	})
	return out
}

type rawMsg struct {
	ID       FlexInt         `json:"id"`
	Date     string          `json:"date"`
	AuthorID FlexInt         `json:"author_id"`
	Text     string          `json:"text"`
	Params   json.RawMessage `json:"params"`
}

// msgParams — интересующие поля params сообщения.
type msgParams struct {
	IsDeleted bool
	ReplyID   int64
	Like      []int64
}

// parseMsgParams: params бывает объектом, а бывает пустым массивом []
// (живой факт) — тогда пустой результат, остальные поля не страдают.
func parseMsgParams(raw json.RawMessage) msgParams {
	var p msgParams
	if len(raw) == 0 || raw[0] != '{' {
		return p
	}
	var wire struct {
		IsDeleted string    `json:"IS_DELETED"`
		ReplyID   FlexInt   `json:"REPLY_ID"`
		Like      []FlexInt `json:"LIKE"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return msgParams{}
	}
	p.IsDeleted = wire.IsDeleted == "Y"
	p.ReplyID = int64(wire.ReplyID)
	for _, v := range wire.Like {
		p.Like = append(p.Like, int64(v))
	}
	return p
}

func likedByMe(likes []int64, myID int64) bool {
	for _, id := range likes {
		if id == myID {
			return true
		}
	}
	return false
}

type rawDialogResult struct {
	Messages []rawMsg    `json:"messages"`
	Users    []rawUserV2 `json:"users"`
}

type rawUserV2 struct {
	ID   FlexInt `json:"id"`
	Name string  `json:"name"`
}

// MessagesFrom — im.dialog.messages.get → строки от свежих к старым (reverse ответа),
// системные (author_id=0) помечены, удалённые выброшены.
func MessagesFrom(raw []byte, myID int64, now time.Time) []Message {
	var res rawDialogResult
	_ = json.Unmarshal(raw, &res)
	users := map[int64]string{}
	for _, u := range res.Users {
		users[int64(u.ID)] = u.Name
	}
	now = now.Local()
	out := make([]Message, 0, len(res.Messages))
	for _, m := range res.Messages {
		ps := parseMsgParams(m.Params)
		if ps.IsDeleted {
			continue
		}
		text, mentionsMe := applyMentions(m.Text, myID)
		day, label := DayInfo(m.Date, now)
		out = append(out, Message{
			ID:         int64(m.ID),
			AuthorID:   int64(m.AuthorID),
			Name:       users[int64(m.AuthorID)],
			Text:       StripBB(text),
			Time:       FmtTime(m.Date, now),
			DayKey:     day,
			DayLabel:   label,
			Mine:       int64(m.AuthorID) == myID,
			System:     int64(m.AuthorID) == 0,
			ReplyID:    ps.ReplyID,
			Likes:      len(ps.Like),
			LikedByMe:  likedByMe(ps.Like, myID),
			MentionsMe: mentionsMe,
		})
	}
	// портал отдаёт от старых к свежим; переворачиваем
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

type rawSearchMsg struct {
	ID       FlexInt         `json:"id"`
	AuthorID FlexInt         `json:"authorId"` // у search — camelCase
	Date     string          `json:"date"`
	Text     string          `json:"text"`
	Params   json.RawMessage `json:"params"`
}

type rawSearchResult struct {
	Messages []rawSearchMsg `json:"messages"`
	Users    []rawUserV2    `json:"users"`
}

// SearchFrom — im.dialog.messages.search → найденные, как пришли (свежие первыми).
func SearchFrom(raw []byte, myID int64, now time.Time) []Message {
	var res rawSearchResult
	_ = json.Unmarshal(raw, &res)
	users := map[int64]string{}
	for _, u := range res.Users {
		users[int64(u.ID)] = u.Name
	}
	now = now.Local()
	out := make([]Message, 0, len(res.Messages))
	for _, m := range res.Messages {
		ps := parseMsgParams(m.Params)
		text, mentionsMe := applyMentions(m.Text, myID)
		day, label := DayInfo(m.Date, now)
		out = append(out, Message{
			ID:         int64(m.ID),
			AuthorID:   int64(m.AuthorID),
			Name:       users[int64(m.AuthorID)],
			Text:       StripBB(text),
			Time:       FmtTime(m.Date, now),
			DayKey:     day,
			DayLabel:   label,
			Mine:       int64(m.AuthorID) == myID,
			System:     int64(m.AuthorID) == 0,
			ReplyID:    ps.ReplyID,
			Likes:      len(ps.Like),
			LikedByMe:  likedByMe(ps.Like, myID),
			MentionsMe: mentionsMe,
		})
	}
	return out
}

type rawNotify struct {
	ID          string `json:"id"`
	Module      string `json:"notify_module"`
	NotifyTitle string `json:"notify_title"`
	Tag         string `json:"notify_tag"`
	Text        string `json:"text"`
	Date        string `json:"date"`
	NotifyRead  string `json:"notify_read"`
}

type rawNotifyResult struct {
	Notifications []rawNotify `json:"notifications"`
}

// NotifyFrom — im.notify.get → уведомления, свежие сверху.
func NotifyFrom(raw []byte, now time.Time) []NotifyItem {
	var res rawNotifyResult
	_ = json.Unmarshal(raw, &res)
	now = now.Local()
	out := make([]NotifyItem, 0, len(res.Notifications))
	for _, n := range res.Notifications {
		it := NotifyItem{
			ID:     n.ID,
			Module: n.Module,
			Title:  StripBB(n.NotifyTitle),
			Text:   StripBB(n.Text),
			Time:   FmtTime(n.Date, now),
			Unread: n.NotifyRead == "N",
		}
		if t, err := time.Parse(time.RFC3339, n.Date); err == nil {
			it.Stamp = t.UnixMilli()
		}
		if parts := strings.Split(n.Tag, "|"); len(parts) >= 3 &&
			parts[0] == "TASKS" && parts[1] == "TASK" {
			var v int64
			if _, err := fmt.Sscanf(parts[2], "%d", &v); err == nil {
				it.TaskID = v
			}
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Stamp > out[j].Stamp })
	return out
}

type rawTask struct {
	ID          FlexInt    `json:"id"`
	Title       string     `json:"title"`
	Status      FlexString `json:"status"`
	Deadline    string     `json:"deadline"`
	Description string     `json:"description"`
	ChatID      FlexInt    `json:"chatId"`
	Creator     *struct {
		Name string `json:"name"`
	} `json:"creator"`
}

// TaskFrom — tasks.task.get → карточка.
func TaskFrom(raw []byte) Task {
	var res struct {
		Task rawTask `json:"task"`
	}
	_ = json.Unmarshal(raw, &res)
	t := Task{
		ID:          int64(res.Task.ID),
		Title:       res.Task.Title,
		Status:      string(res.Task.Status),
		Deadline:    res.Task.Deadline,
		Description: StripBB(res.Task.Description),
		ChatID:      int64(res.Task.ChatID),
	}
	if res.Task.Creator != nil {
		t.CreatorName = res.Task.Creator.Name
	}
	if t.Status == "" {
		t.Status = "2"
	}
	return t
}
