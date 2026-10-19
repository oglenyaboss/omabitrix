// Package tui — терминальный клиент Bitrix24 (bubbletea).
package tui

// Chat — элемент im.recent.list.
type Chat struct {
	DialogID string
	Title    string
	Counter  int
	Preview  string
	Time     string // готовая метка HH:MM / DD.MM
	Stamp    int64  // unix ms последнего сообщения, для сортировки
	Online   bool
}

// Message — строка истории диалога (регулярная или системная).
type Message struct {
	ID       int64
	AuthorID int64 // 0 = системное; ключ цвета автора и группировки
	Name     string
	Text     string
	Time     string
	DayKey   string
	DayLabel string
	Mine     bool
	System   bool
}

// NotifyItem — системное уведомление im.notify.get.
type NotifyItem struct {
	ID     string
	Module string
	Title  string
	Text   string
	Time   string
	Unread bool
	Stamp  int64
	TaskID int64 // увед-тег TASKS|TASK|<id>: enter откроет карточку
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
}
