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

// taskRefRe — номер задачи в тексте уведа: [#15947] → #15947, до StripBB
// (иначе номер исчезает вместе с BB-скобками и остаётся двойной пробел).
var taskRefRe = regexp.MustCompile(`\[#(\d+)\]`)

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

// ShiftISO — метка портала, приведённая к часам ПК: все даты в ответах
// прошиты серверными часами, time.start того же ответа — тоже; сдвиг на
// skew (местное − серверное) убирает перекос серверных часов.
func ShiftISO(iso string, skew time.Duration) string {
	if skew == 0 || iso == "" {
		return iso
	}
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t.Add(skew).Format(time.RFC3339)
	}
	return iso
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
		Text   string  `json:"text"`
		Date   string  `json:"date"`
		ChatID FlexInt `json:"chat_id"` // под-чат, где реально лежит сообщение (коллабы)
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
// skew — перекос серверных часов (см. ShiftISO).
func ChatsFrom(raw []byte, now time.Time, skew time.Duration) []Chat {
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
		// коллаб: строка recent — системный чат коллаба (вступил/покинул проект),
		// живая переписка — в под-чате (чат задачи); message.chat_id указывает
		// на него. Открываем и отвечаем там, где реально идёт разговор.
		if it.ChatID > 0 && it.Message != nil && it.Message.ChatID > 0 &&
			int64(it.Message.ChatID) != int64(it.ChatID) {
			sub := int64(it.Message.ChatID)
			c.DialogID = "chat" + strconv.FormatInt(sub, 10)
			c.ChatID = sub
			c.ParentChatID = int64(it.ChatID)
		}
		if c.Title == "" {
			c.Title = dlg
		}
		if it.Message != nil {
			c.Preview = StripBB(it.Message.Text)
			date := ShiftISO(it.Message.Date, skew)
			c.Time = FmtTime(date, now)
			if t, err := time.Parse(time.RFC3339, date); err == nil {
				c.Stamp = t.UnixMilli()
			}
		}
		if it.User != nil && it.User.Status == "online" {
			c.Online = true
		}
		out = append(out, c)
	}
	sortChats(out)
	return out
}

// sortChats — непрочитанные вверх, дальше по свежести (после докладки
// коллаб-непрочитанных из im.counters.get вызывается повторно из API.Chats).
func sortChats(out []Chat) {
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Counter > 0) != (out[j].Counter > 0) {
			return out[i].Counter > 0
		}
		return out[i].Stamp > out[j].Stamp
	})
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
func MessagesFrom(raw []byte, myID int64, now time.Time, skew time.Duration) []Message {
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
		date := ShiftISO(m.Date, skew)
		day, label := DayInfo(date, now)
		out = append(out, Message{
			ID:         int64(m.ID),
			AuthorID:   int64(m.AuthorID),
			Name:       users[int64(m.AuthorID)],
			Text:       StripBB(text),
			Time:       FmtTime(date, now),
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
func SearchFrom(raw []byte, myID int64, now time.Time, skew time.Duration) []Message {
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
		date := ShiftISO(m.Date, skew)
		day, label := DayInfo(date, now)
		out = append(out, Message{
			ID:         int64(m.ID),
			AuthorID:   int64(m.AuthorID),
			Name:       users[int64(m.AuthorID)],
			Text:       StripBB(text),
			Time:       FmtTime(date, now),
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
	// id: живой портал шлёт числом, коробки/старые фикстуры — строкой.
	ID          FlexString `json:"id"`
	Module      string     `json:"notify_module"`
	NotifyTitle string     `json:"notify_title"`
	Tag         string     `json:"notify_tag"`
	Text        string     `json:"text"`
	Date        string     `json:"date"`
	NotifyRead  string     `json:"notify_read"`
	AuthorID    FlexInt    `json:"author_id"`
	// params: null у calendar, object у задач (шаблон темы с URL карточки)
	Params *struct {
		ComponentParams *struct {
			Subject string `json:"SUBJECT"`
		} `json:"COMPONENT_PARAMS"`
	} `json:"params"`
}

type rawNotifyResult struct {
	Notifications []rawNotify `json:"notifications"`
	TotalUnread   int         `json:"total_unread_count"`
	Users         []struct {
		ID   FlexInt `json:"id"`
		Name string  `json:"name"`
	} `json:"users"`
}

// urlRe — первая ссылка [URL=адрес]подпись[/URL] в тексте/теме уведа.
var urlRe = regexp.MustCompile(`(?i)\[URL=([^\]]+)\]`)

// NotifyPageFrom — страница im.notify.get → уведомления и счётчик
// непрочитанных (total_unread_count). Автор — из users[] ответа; ссылка —
// из текста, у задач — из params.COMPONENT_PARAMS.SUBJECT.
func NotifyPageFrom(raw []byte, now time.Time, skew time.Duration) ([]NotifyItem, int) {
	var res rawNotifyResult
	_ = json.Unmarshal(raw, &res)
	names := make(map[int64]string, len(res.Users))
	for _, u := range res.Users {
		names[int64(u.ID)] = u.Name
	}
	now = now.Local()
	out := make([]NotifyItem, 0, len(res.Notifications))
	for _, n := range res.Notifications {
		date := ShiftISO(n.Date, skew)
		it := NotifyItem{
			ID:       string(n.ID),
			Module:   n.Module,
			Title:    StripBB(taskRefRe.ReplaceAllString(n.NotifyTitle, "#$1")),
			Text:     StripBB(taskRefRe.ReplaceAllString(n.Text, "#$1")),
			Time:     FmtTime(date, now),
			Unread:   n.NotifyRead == "N",
			AuthorID: int64(n.AuthorID),
			Author:   names[int64(n.AuthorID)],
		}
		if t, err := time.Parse(time.RFC3339, date); err == nil {
			it.Stamp = t.UnixMilli()
			it.Date = t.Local().Format("02.01.2006 15:04")
		}
		if m := urlRe.FindStringSubmatch(n.Text); m != nil {
			it.URL = m[1]
		} else if n.Params != nil && n.Params.ComponentParams != nil {
			if m := urlRe.FindStringSubmatch(n.Params.ComponentParams.Subject); m != nil {
				it.URL = m[1]
			}
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
	sortNotify(out)
	return out, res.TotalUnread
}

// NotifyFrom — im.notify.get → уведомления, свежие сверху.
func NotifyFrom(raw []byte, now time.Time, skew time.Duration) []NotifyItem {
	items, _ := NotifyPageFrom(raw, now, skew)
	return items
}

// sortNotify — непрочитанные вверх, дальше свежие (Stamp, стабильно).
func sortNotify(items []NotifyItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Unread != items[j].Unread {
			return items[i].Unread
		}
		return items[i].Stamp > items[j].Stamp
	})
}

type rawTask struct {
	ID          FlexInt    `json:"id"`
	Title       string     `json:"title"`
	Status      FlexString `json:"status"`
	Deadline    string     `json:"deadline"`
	Description string     `json:"description"`
	ChatID      FlexInt    `json:"chatId"`
	GroupID     FlexInt    `json:"groupId"`
	StageID     FlexInt    `json:"stageId"`
	Creator     *struct {
		Name string `json:"name"`
	} `json:"creator"`
}

// StagesFrom — task.stages.get: объект по id (или [] у пустого канбана)
// → список по SORT.
func StagesFrom(raw []byte) []Stage {
	var res map[string]struct {
		ID    FlexInt    `json:"ID"`
		Title string     `json:"TITLE"`
		Sort  FlexInt    `json:"SORT"`
		Color FlexString `json:"COLOR"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return nil
	}
	type sorted struct {
		st   Stage
		sort int64
	}
	all := make([]sorted, 0, len(res))
	for _, v := range res {
		all = append(all, sorted{Stage{ID: int64(v.ID), Title: v.Title, Color: string(v.Color)}, int64(v.Sort)})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].sort < all[j].sort })
	out := make([]Stage, len(all))
	for i, s := range all {
		out[i] = s.st
	}
	return out
}

// oneLine — название задачи в одну строку: табы и переводы строк из портала
// ломают ширину карточек.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// TaskFrom — tasks.task.get → карточка.
func TaskFrom(raw []byte) Task {
	var res struct {
		Task rawTask `json:"task"`
	}
	_ = json.Unmarshal(raw, &res)
	t := Task{
		ID:          int64(res.Task.ID),
		Title:       oneLine(res.Task.Title),
		Status:      string(res.Task.Status),
		Deadline:    res.Task.Deadline,
		Description: StripBB(res.Task.Description),
		ChatID:      int64(res.Task.ChatID),
		GroupID:     int64(res.Task.GroupID),
		StageID:     int64(res.Task.StageID),
	}
	if res.Task.Creator != nil {
		t.CreatorName = res.Task.Creator.Name
	}
	if t.Status == "" {
		t.Status = "2"
	}
	return t
}
