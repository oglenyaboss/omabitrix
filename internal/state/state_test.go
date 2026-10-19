package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s := New()
	s.BaselineDone = true
	s.ImV2Offset = 42
	s.KnownTasks[7] = true
	if !s.ShouldOnce("overdue|7|2026-10-19") {
		t.Fatal("первый ShouldOnce должен быть true")
	}
	if s.ShouldOnce("overdue|7|2026-10-19") {
		t.Fatal("второй ShouldOnce должен быть false")
	}
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp-файл не убран")
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.ImV2Offset != 42 || !got.KnownTasks[7] || !got.Notified["overdue|7|2026-10-19"] {
		t.Errorf("roundtrip потерял данные: %+v", got)
	}
}

func TestLoadMissing(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || s == nil || s.KnownTasks == nil {
		t.Fatalf("ожидал чистый state, got %v %v", s, err)
	}
}

func TestPruneNotified(t *testing.T) {
	s := New()
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	s.Notified["overdue|1|2026-10-01"] = true
	s.Notified["overdue|2|2026-10-18"] = true
	s.Notified["msg|99|2026-10-01"] = true
	s.Notified["badkey"] = true
	s.PruneNotified(now, 7)
	if s.Notified["overdue|1|2026-10-01"] || s.Notified["msg|99|2026-10-01"] {
		t.Error("старые ключи не удалены")
	}
	if !s.Notified["overdue|2|2026-10-18"] || !s.Notified["badkey"] {
		t.Error("свежие ключи удалены зря")
	}
}

func TestSnapshotSave(t *testing.T) {
	p := filepath.Join(t.TempDir(), "snap.json")
	snap := &Snapshot{Chats: 3, Overdue: 2, UpdatedAt: "now"}
	if err := snap.Save(p); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	var back Snapshot
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Chats != 3 || back.Overdue != 2 || back.Error != "" {
		t.Errorf("snapshot: %+v", back)
	}
}
