// Package tui — терминальный клиент Bitrix24 (bubbletea).
package tui

// Chat — элемент im.recent.list.
type Chat struct {
	DialogID string
	ChatID   int64 // числовой id чата (поиск, реакции); у личных диалогов может быть 0
	// ParentChatID — системный чат коллаба, когда диалог перепривязан к под-чату:
	// по нему раскладываются непрочитанные из im.counters.get (CHANNEL_COMMENT).
	ParentChatID int64
	Title        string
	Counter      int
	Preview      string
	Time         string // готовая метка HH:MM / DD.MM
	Stamp        int64  // unix ms последнего сообщения, для сортировки
	Online       bool
}

// Attachment — файл сообщения (files[] ответа im.dialog.messages.get).
type Attachment struct {
	ID   int64  // disk-файл; fileId для im.v2.File.download (живая проверка)
	Type string // "image" | "file": поле image у портала бывает false даже у png
	Name string
	Ext  string
	Size int64
}

// IsImage — картинка для инлайн-превью в истории.
func (a Attachment) IsImage() bool { return a.Type == "image" }

// Message — строка истории диалога (регулярная или системная).
type Message struct {
	ID         int64
	AuthorID   int64 // 0 = системное; ключ цвета автора и группировки
	Name       string
	Text       string
	Files      []Attachment
	Time       string
	DayKey     string
	DayLabel   string
	Mine       bool
	System     bool
	ReplyID    int64 // ответ-цитата: params.REPLY_ID (не задокументировано — терпимый парс)
	Likes      int   // params.LIKE — сколько лайков
	LikedByMe  bool  // мой id в params.LIKE
	MentionsMe bool  // [USER=мой_id] или [USER=all] в тексте
}

// NotifyItem — системное уведомление im.notify.get.
type NotifyItem struct {
	ID       string
	Module   string
	Title    string
	Text     string
	Time     string
	Unread   bool
	Stamp    int64
	TaskID   int64  // увед-тег TASKS|TASK|<id>: enter откроет карточку
	AuthorID int64  // author_id из ответа
	Author   string // имя автора из users[] ответа; "" если автора там нет
	URL      string // первая [URL=…] из текста; у задач — из params.COMPONENT_PARAMS.SUBJECT
	Date     string // полная дата «02.01.2006 15:04» в локальной зоне
}

// Task — карточка задачи.
type Task struct {
	ID          int64
	Title       string
	Deadline    string // "2006-01-02 15:04:05" (server TZ) или ""
	Description string
	CreatorName string
	Status      string // 1..7
	ChatID      int64
	Role        string // «делаю»/«помогаю»/«наблюдаю»: do | help | watch
	NewComments int    // бейдж новых комментариев (newCommentsCount)
	GroupID     int64  // проект (рабочая группа); 0 — без проекта
	Project     string // имя проекта (sonet_group.get)
	StageID     int64  // стадия канбана проекта; 0 портала уже разрешён в первую
	Stage       string // имя стадии: INBOX/TODO/… — ключ колонки доски
	StageColor  string // hex без «#», как отдаёт портал
	StageSort   int    // позиция стадии в канбане проекта (0 — первая)
}

// Stage — стадия канбана проекта (task.stages.get), по порядку SORT.
type Stage struct {
	ID    int64
	Title string
	Color string // hex без «#»
}
