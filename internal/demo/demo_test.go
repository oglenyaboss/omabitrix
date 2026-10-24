package demo_test

import (
	"bytes"
	"context"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oglenyaboss/omabitrix/internal/b24"
	"github.com/oglenyaboss/omabitrix/internal/demo"
	"github.com/oglenyaboss/omabitrix/internal/tui"
)

func newAPI(t *testing.T, o demo.Options) *tui.API {
	t.Helper()
	srv := demo.Start(o)
	t.Cleanup(srv.Close)
	c := b24.New(srv.URL + demo.WebhookPath)
	return &tui.API{C: c, My: demo.MyID, Portal: demo.Portal}
}

func TestA1Chats(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	chats, err := api.Chats(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 7 {
		t.Fatalf("chats = %d, want 7", len(chats))
	}
	if chats[0].DialogID != "2" || chats[0].Title != "Анна Смирнова" || chats[0].Counter != 2 {
		t.Errorf("first chat: %+v", chats[0])
	}
	for _, c := range chats {
		if c.DialogID == "chat101" {
			if c.Counter != 5 {
				t.Errorf("chat101 counter = %d, want 5", c.Counter)
			}
			return
		}
	}
	t.Error("chat101 missing")
}

func TestA2OnboardingImage(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	msgs, _, err := api.History(ctx, "2", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 8 {
		t.Fatalf("history = %d, want 8", len(msgs))
	}
	var fileID int64
	for _, m := range msgs {
		if len(m.Files) > 0 && m.Files[0].Name == "onboarding-v3.png" && m.Files[0].IsImage() {
			fileID = m.Files[0].ID
		}
	}
	if fileID == 0 {
		t.Fatal("onboarding attachment missing")
	}
	link, err := api.FileURL(ctx, fileID)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 360 || b.Dy() != 240 {
		t.Errorf("bounds = %v", b)
	}
}

func TestA3CrashLog(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	msgs, _, err := api.History(ctx, "5", 0)
	if err != nil {
		t.Fatal(err)
	}
	var fileID int64
	for _, m := range msgs {
		for _, f := range m.Files {
			if f.Name == "crash-log.txt" {
				fileID = f.ID
			}
		}
	}
	if fileID == 0 {
		t.Fatal("crash-log attachment missing")
	}
	link, err := api.FileURL(ctx, fileID)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Fatal("crash log served as PNG")
	}
	if !strings.Contains(string(data), "FATAL EXCEPTION") {
		t.Errorf("unexpected crash log: %.80q", data)
	}
}

func TestA4OpenTasks(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	tasks, err := api.OpenTasks(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 10 {
		t.Fatalf("tasks = %d, want 10", len(tasks))
	}
	roles := map[string]int{}
	byID := map[int64]tui.Task{}
	for _, task := range tasks {
		roles[task.Role]++
		byID[task.ID] = task
	}
	if roles["do"] != 8 || roles["help"] != 1 || roles["watch"] != 1 {
		t.Errorf("roles: %+v", roles)
	}
	got := byID[201]
	if got.Stage != "В работе" || got.Project != "Мобильное приложение" {
		t.Errorf("task 201: %+v", got)
	}
}

func TestA5Overdue(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	tasks, err := api.OpenTasks(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	b := tui.Buckets(tasks, time.Local, time.Now())
	found := false
	for _, task := range b.Overdue {
		if task.ID == 203 {
			found = true
		}
	}
	if !found {
		t.Errorf("task 203 not overdue: %+v", b.Overdue)
	}
}

func TestA6MoveStage(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	if _, err := api.OpenTasks(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := api.MoveStage(ctx, 205, 102); err != nil {
		t.Fatal(err)
	}
	tasks, err := api.OpenTasks(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.ID == 205 {
			if task.Stage != "В работе" {
				t.Errorf("stage = %q, want В работе", task.Stage)
			}
			return
		}
	}
	t.Error("task 205 missing")
}

func TestA7Send(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	id, err := api.Send(ctx, "3", "привет из демо", 0)
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("id = %d", id)
	}
	msgs, _, err := api.History(ctx, "3", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Fatal("empty history")
	}
	newest := msgs[len(msgs)-1]
	if newest.Text != "привет из демо" || newest.AuthorID != demo.MyID {
		t.Errorf("newest: %+v", newest)
	}
}

func TestA8ReadDialog(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	if err := api.ReadDialog(ctx, "chat101", 0); err != nil {
		t.Fatal(err)
	}
	chats, err := api.Chats(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chats {
		if c.DialogID == "chat101" {
			if c.Counter != 0 {
				t.Errorf("counter = %d, want 0", c.Counter)
			}
			return
		}
	}
	t.Error("chat101 missing")
}

func TestA9Notify(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	items, err := api.Notify(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 8 {
		t.Fatalf("notifications = %d, want 8", len(items))
	}
	if api.NotifyUnread() != 3 {
		t.Fatalf("unread = %d, want 3", api.NotifyUnread())
	}
	if err := api.MarkNotify(ctx, items[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Notify(ctx, true); err != nil {
		t.Fatal(err)
	}
	if api.NotifyUnread() != 2 {
		t.Errorf("unread after mark = %d, want 2", api.NotifyUnread())
	}
	added, err := api.NotifyOlder(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("older added = %d, want 0", added)
	}
}

func TestA10Ping(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{PingAfter: 100 * time.Millisecond})
	chats, err := api.Chats(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := counterOf(chats, "2"); got != 2 {
		t.Fatalf("before ping counter = %d, want 2", got)
	}
	time.Sleep(300 * time.Millisecond)
	chats, err = api.Chats(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := counterOf(chats, "2"); got != 3 {
		t.Errorf("after ping counter = %d, want 3", got)
	}
	msgs, _, err := api.History(ctx, "2", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 || !strings.HasPrefix(msgs[len(msgs)-1].Text, "Обновила макет экрана бонусов") {
		t.Errorf("newest after ping: %+v", msgs[len(msgs)-1])
	}
}

func counterOf(chats []tui.Chat, dialogID string) int {
	for _, c := range chats {
		if c.DialogID == dialogID {
			return c.Counter
		}
	}
	return -1
}

func TestA11TaskCard(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	task, err := api.Task(ctx, 201)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Пуш-уведомления о начислении бонусов" {
		t.Errorf("title = %q", task.Title)
	}
	if task.Description == "" {
		t.Error("empty description")
	}
	if task.CreatorName != "Мария Иванова" {
		t.Errorf("creator = %q", task.CreatorName)
	}
}

func TestA12Search(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	msgs, err := api.SearchMessages(ctx, 101, "релиз")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Error("no messages found")
	}
}

func TestA13Paging(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	msgs, noMore, err := api.History(ctx, "chat101", 0)
	if err != nil {
		t.Fatal(err)
	}
	if noMore || len(msgs) == 0 {
		t.Fatalf("first page: %d noMore=%v", len(msgs), noMore)
	}
	oldest := msgs[0].ID
	older, noMore, err := api.History(ctx, "chat101", oldest)
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 0 || !noMore {
		t.Errorf("older = %d noMore=%v", len(older), noMore)
	}
}

func TestA14UnknownMethod(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	if err := api.C.Call(ctx, "im.unknown.method", nil, nil); err != nil {
		t.Fatalf("unknown method: %v", err)
	}
}

func TestMutationsPersist(t *testing.T) {
	ctx := context.Background()
	api := newAPI(t, demo.Options{})
	msgs, _, err := api.History(ctx, "3", 0)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("history: %v", err)
	}
	id := msgs[0].ID
	if err := api.UpdateMessage(ctx, id, "изменено в демо"); err != nil {
		t.Fatal(err)
	}
	if err := api.LikeMessage(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	msgs, _, _ = api.History(ctx, "3", 0)
	if msgs[0].Text != "изменено в демо" || msgs[0].Likes != 1 || !msgs[0].LikedByMe {
		t.Errorf("updated message: %+v", msgs[0])
	}
	if err := api.LikeMessage(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	msgs, _, _ = api.History(ctx, "3", 0)
	if msgs[0].Likes != 0 {
		t.Errorf("like not removed: %+v", msgs[0])
	}
	if err := api.DeleteMessage(ctx, id); err != nil {
		t.Fatal(err)
	}
	msgs, _, _ = api.History(ctx, "3", 0)
	for _, m := range msgs {
		if m.ID == id {
			t.Error("deleted message still visible")
		}
	}

	if err := api.SetStatus(ctx, 201, "5"); err != nil {
		t.Fatal(err)
	}
	tasks, err := api.OpenTasks(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.ID == 201 {
			t.Error("status 5 task still open")
		}
	}
	if err := api.SetDeadline(ctx, 202, ""); err != nil {
		t.Fatal(err)
	}
	task, err := api.Task(ctx, 202)
	if err != nil {
		t.Fatal(err)
	}
	if task.Deadline != "" {
		t.Errorf("deadline = %q, want empty", task.Deadline)
	}

	msgID, fileID, err := api.UploadFile(ctx, "3", "note.txt", []byte("demo attachment"), "подпись")
	if err != nil || msgID == 0 || fileID == 0 {
		t.Fatalf("upload msg=%d file=%d err=%v", msgID, fileID, err)
	}
	msgs, _, _ = api.History(ctx, "3", 0)
	newest := msgs[len(msgs)-1]
	if newest.ID != msgID || newest.Text != "подпись" || len(newest.Files) == 0 || newest.Files[0].Name != "note.txt" {
		t.Errorf("uploaded message: %+v", newest)
	}
	link, err := api.FileURL(ctx, fileID)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if string(data) != "demo attachment" {
		t.Errorf("uploaded bytes = %q", data)
	}
}
