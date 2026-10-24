package notify

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oglenyaboss/omabitrix/internal/rules"
)

func TestArgs(t *testing.T) {
	tst := rules.Toast{Kind: rules.KindComment, Title: "Комментарий: Задача X", Body: "fix ready", Urgency: "normal", URL: "https://portal.example.com/company/personal/user/75/tasks/task/view/5/"}
	got := Args(tst, "omarchy-launch-webapp", "web")
	want := []string{"--app-name", "Bitrix24", "-g", "󰭹", "-u", "normal", "-t", "8000", "Комментарий: Задача X", "fix ready", "--exec", "omarchy-launch-webapp", "https://portal.example.com/company/personal/user/75/tasks/task/view/5/"}
	if len(got) != len(want) {
		t.Fatalf("args = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %q, хочу %q (все: %v)", i, got[i], want[i], got)
		}
	}
}

func TestArgsDefaults(t *testing.T) {
	got := Args(rules.Toast{Kind: rules.KindSummary, Title: "omabitrix"}, "opener", "web")
	// urgency по умолчанию normal, без body и url
	want := []string{"--app-name", "Bitrix24", "-g", "󰭹", "-u", "normal", "-t", "8000", "omabitrix"}
	if len(got) != len(want) {
		t.Fatalf("args = %v, хочу %v", got, want)
	}
}

func TestArgsTui(t *testing.T) {
	// tui-режим: клик открывает omabitrix в контексте
	chat := Args(rules.Toast{Kind: rules.KindDialog, Title: "x", DialogID: "chat10839"}, "opener", "tui")
	wantChat := []string{"--app-name", "Bitrix24", "-g", "󰭹", "-u", "normal", "-t", "8000", "x", "--exec", "foot", "-a", "omabitrix", "omabitrix", "--chat", "chat10839"}
	if len(chat) != len(wantChat) {
		t.Fatalf("chat args = %v", chat)
	}
	for i := range wantChat {
		if chat[i] != wantChat[i] {
			t.Fatalf("chat[%d] = %q, хочу %q", i, chat[i], wantChat[i])
		}
	}
	task := Args(rules.Toast{Kind: rules.KindOverdue, Title: "y", TaskID: 12475}, "opener", "tui")
	foundTask := false
	for i := range task {
		if task[i] == "--task" && task[i+1] == "12475" {
			foundTask = true
		}
	}
	if !foundTask {
		t.Fatalf("task args = %v", task)
	}
	// без привязки — как web
	fallback := Args(rules.Toast{Kind: rules.KindSummary, Title: "z", URL: "https://x"}, "omarchy-launch-webapp", "tui")
	last := fallback[len(fallback)-1]
	if last != "https://x" {
		t.Fatalf("fallback = %v", fallback)
	}
}

// waitLines ждёт появления в файле n строк (плеер асинхронный), затем даёт
// 100мс на опоздавшие дублирующие запуски и требует ровно n.
func waitLines(t *testing.T, path string, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil && bytes.Count(data, []byte("\n")) >= n {
			break
		}
		if time.Now().After(deadline) {
			got := 0
			if data, err := os.ReadFile(path); err == nil {
				got = bytes.Count(data, []byte("\n"))
			}
			t.Fatalf("в %s строк %d, ждали >=%d", path, got, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if got := bytes.Count(data, []byte("\n")); got != n {
		t.Fatalf("в %s строк %d, хочу ровно %d", path, got, n)
	}
}

func TestSendPlaysSound(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sound.log")
	o := &Omarchy{Command: "true", Sound: []string{"sh", "-c", "echo x >> " + file}}
	if err := o.Send(context.Background(), rules.Toast{Kind: rules.KindSummary, Title: "тест"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitLines(t, file, 1)
}

func TestSendSoundThrottle(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sound.log")
	T := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := T
	o := &Omarchy{Command: "true", Sound: []string{"sh", "-c", "echo x >> " + file}, now: func() time.Time { return now }}
	for i := 0; i < 2; i++ {
		if err := o.Send(context.Background(), rules.Toast{Kind: rules.KindSummary, Title: "тест"}); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}
	waitLines(t, file, 1)
	now = T.Add(3 * time.Second)
	if err := o.Send(context.Background(), rules.Toast{Kind: rules.KindSummary, Title: "тест"}); err != nil {
		t.Fatalf("Send после 3с: %v", err)
	}
	waitLines(t, file, 2)
}

func TestSendSoundPlayerMissing(t *testing.T) {
	o := &Omarchy{Command: "true", Sound: []string{"/nonexistent-player"}}
	if err := o.Send(context.Background(), rules.Toast{Kind: rules.KindSummary, Title: "тест"}); err != nil {
		t.Fatalf("Send: %v, хочу nil при нерабочем плеере", err)
	}
}

func TestSendToastErrorStillPlaysSound(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sound.log")
	o := &Omarchy{Command: "false", Sound: []string{"sh", "-c", "echo x >> " + file}}
	err := o.Send(context.Background(), rules.Toast{Kind: rules.KindSummary, Title: "тест"})
	if err == nil {
		t.Fatal("хочу ошибку от тоста (false)")
	}
	waitLines(t, file, 1)
}

func TestSendNoSound(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sound.log")
	o := &Omarchy{Command: "true"}
	if err := o.Send(context.Background(), rules.Toast{Kind: rules.KindSummary, Title: "тест"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("файл %s не должен создаваться при Sound=nil", file)
	}
}
