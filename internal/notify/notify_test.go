package notify

import (
	"testing"

	"omabitrix/internal/rules"
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
