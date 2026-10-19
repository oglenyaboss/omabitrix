package tui

import (
	"errors"
	"strconv"
	"testing"
)

const browserTestPortal = "https://x.bitrix24.ru"

func withPortal(m Model) Model {
	m.api.Portal = browserTestPortal
	m.api.My = 7
	return m
}

func TestBrowserURL(t *testing.T) {
	chats := []Chat{{DialogID: "chat1", Title: "А"}, {DialogID: "chat2", Title: "Б"}}
	tasks := []Task{{ID: 5, Title: "x", Role: "do"}}
	notify := []NotifyItem{{ID: "n1", TaskID: 15947}, {ID: "n2", TaskID: 0}}
	taskURL := func(id int) string {
		return browserTestPortal + "/company/personal/user/7/tasks/task/view/" + strconv.Itoa(id) + "/"
	}

	// активный чат
	m := withPortal(viewModel(120, 36, chats, nil, nil, &chats[0], nil))
	m.tab = tabChats
	if got := m.browserURL(); got != browserTestPortal+"/online/?IM_DIALOG=chat1" {
		t.Fatalf("активный чат: %q", got)
	}
	// без активного — чат под курсором
	m.active = nil
	m.cursor = 1
	if got := m.browserURL(); got != browserTestPortal+"/online/?IM_DIALOG=chat2" {
		t.Fatalf("чат под курсором: %q", got)
	}
	// курсор за границей списка
	m.cursor = 9
	if got := m.browserURL(); got != "" {
		t.Fatalf("курсор за границей: %q", got)
	}

	// открытая карточка задачи
	m = withPortal(viewModel(120, 36, nil, tasks, nil, nil, nil))
	m.tab = tabTasks
	m.taskOpen = true
	m.taskCard = Task{ID: 5}
	if got := m.browserURL(); got != taskURL(5) {
		t.Fatalf("карточка задачи: %q", got)
	}
	// задача под курсором (карточка закрыта)
	m.taskOpen = false
	if got := m.browserURL(); got != taskURL(5) {
		t.Fatalf("задача под курсором: %q", got)
	}

	// уведомление с задачей и без
	m = withPortal(viewModel(120, 36, nil, nil, notify, nil, nil))
	m.tab = tabNotify
	if got := m.browserURL(); got != taskURL(15947) {
		t.Fatalf("задача из уведомления: %q", got)
	}
	m.cursor = 1
	if got := m.browserURL(); got != "" {
		t.Fatalf("уведомление без задачи: %q", got)
	}

	// пустой портал
	m2 := viewModel(120, 36, chats, nil, nil, &chats[0], nil)
	m2.tab = tabChats
	if got := m2.browserURL(); got != "" {
		t.Fatalf("пустой портал: %q", got)
	}
}

func TestOpenURLCmd(t *testing.T) {
	var got string
	var boom bool
	old := startOpener
	defer func() { startOpener = old }()
	startOpener = func(url string) error {
		got = url
		if boom {
			return errors.New("нет xdg-open")
		}
		return nil
	}

	want := browserTestPortal + "/online/?IM_DIALOG=chat1"
	if msg := openURLCmd(want)(); msg.(browserMsg).err != nil || got != want {
		t.Fatalf("openURLCmd: url=%q err=%v", got, msg.(browserMsg).err)
	}
	// ошибка запуска доезжает до browserMsg
	boom = true
	if msg := openURLCmd(want)(); msg.(browserMsg).err == nil {
		t.Fatal("ошибка запуска потерялась")
	}
	boom = false

	// есть что открыть — команда запуска с URL из browserURL
	chats := []Chat{{DialogID: "chat1", Title: "А"}}
	m := withPortal(viewModel(120, 36, chats, nil, nil, &chats[0], nil))
	mm, cmd := m.openBrowser()
	if cmd == nil {
		t.Fatal("openBrowser с URL без команды")
	}
	msg := cmd()
	if bm := msg.(browserMsg); bm.err != nil || got != want {
		t.Fatalf("openBrowser: url=%q err=%v", got, bm.err)
	}
	if flash := mm.(Model).flash; flash != "" {
		t.Fatalf("успех до ответа команды не должен флешить: %q", flash)
	}
}

func TestBrowserMsgAndFlash(t *testing.T) {
	chats := []Chat{{DialogID: "chat1", Title: "А"}}
	m := withPortal(viewModel(120, 36, chats, nil, nil, &chats[0], nil))
	m.tab = tabChats

	mm, _ := m.Update(browserMsg{err: errors.New("нет xdg-open")})
	if got := mm.(Model).loadErr; got != "браузер: нет xdg-open" {
		t.Fatalf("ошибка браузера: loadErr=%q", got)
	}
	mm, _ = m.Update(browserMsg{})
	if got := mm.(Model).flash; got != "открыто в браузере" {
		t.Fatalf("успех браузера: flash=%q", got)
	}

	// нечего открывать — флеш, команды нет (реальный opener не зовётся)
	m2 := viewModel(120, 36, chats, nil, nil, nil, nil)
	m2.tab = tabChats
	mm2, cmd := m2.openBrowser()
	m3 := mm2.(Model)
	if m3.flash != "нечего открыть в браузере" || cmd != nil {
		t.Fatalf("пустой URL: flash=%q cmd=%v", m3.flash, cmd)
	}
}
