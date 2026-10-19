package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State — всё, что должно переживать рестарт демона.
type State struct {
	Version      int               `json:"version"`
	Subscribed   bool              `json:"subscribed"`    // im.v2.Event.subscribe сделан
	BaselineDone bool              `json:"baseline_done"` // первый цикл прошёл без тостов
	ImV2Offset   int               `json:"im_v2_offset"`
	Cycles       int               `json:"cycles"`       // счётчик циклов для периодических полных проходов
	LastChanged  map[string]string `json:"last_changed"` // роль -> время последней дельты
	KnownTasks   map[int]bool      `json:"known_tasks"`
	NewComments  map[int]int       `json:"new_comments"` // task_id -> последний увиденный newCommentsCount
	Notified     map[string]bool   `json:"notified"`     // ключи идемпотентности тостов
}

func New() *State {
	return &State{
		Version:     1,
		LastChanged: map[string]string{},
		KnownTasks:  map[int]bool{},
		NewComments: map[int]int{},
		Notified:    map[string]bool{},
	}
}

func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	s := New()
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("state %s: %w", path, err)
	}
	if s.LastChanged == nil {
		s.LastChanged = map[string]string{}
	}
	if s.KnownTasks == nil {
		s.KnownTasks = map[int]bool{}
	}
	if s.NewComments == nil {
		s.NewComments = map[int]int{}
	}
	if s.Notified == nil {
		s.Notified = map[string]bool{}
	}
	return s, nil
}

// Save атомарен: tmp + rename, чтобы краш не рвал JSON.
func (s *State) Save(path string) error {
	return saveAtomic(path, s)
}

// ShouldOnce true — только в первый раз для ключа.
func (s *State) ShouldOnce(key string) bool {
	if s.Notified[key] {
		return false
	}
	s.Notified[key] = true
	return true
}

// PruneNotified чистит дневные ключи (kind|id|YYYY-MM-DD) старше keepDays.
func (s *State) PruneNotified(now time.Time, keepDays int) {
	cutoff := now.AddDate(0, 0, -keepDays).Format("2006-01-02")
	for k := range s.Notified {
		parts := strings.Split(k, "|")
		d := parts[len(parts)-1]
		if len(d) == 10 && d < cutoff {
			delete(s.Notified, k)
		}
	}
}

// TaskRef — задача для снапшота бара.
type TaskRef struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Deadline string `json:"deadline,omitempty"`
}

type UnreadChat struct {
	DialogID string `json:"dialog_id"`
	Title    string `json:"title"`
	Counter  int    `json:"counter"`
}

// Snapshot — то, что читает бар-плагин (FileView + Timer).
type Snapshot struct {
	Chats        int          `json:"chats"`
	Notify       int          `json:"notify"`
	Overdue      int          `json:"overdue"`
	Today        int          `json:"today"`
	Error        string       `json:"error,omitempty"` // непусто → плагин показывает ⚠️
	Unread       []UnreadChat `json:"unread,omitempty"`
	OverdueTasks []TaskRef    `json:"overdue_tasks,omitempty"`
	TodayTasks   []TaskRef    `json:"today_tasks,omitempty"`
	UpdatedAt    string       `json:"updated_at"`
}

func (s *Snapshot) Save(path string) error {
	return saveAtomic(path, s)
}

func saveAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
