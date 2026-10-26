package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/oglenyaboss/omabitrix/internal/b24"
)

// testCache — кэш в temp-каталоге теста.
func testCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := userCacheDir
	userCacheDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userCacheDir = old })
	return dir
}

// testPNG — 80x60 png-байты.
func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 80, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: uint8(255 - y*4), G: 40, B: 40, A: 255})
		}
	}
	var b strings.Builder
	if err := png.Encode(&nopWriter{&b}, img); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}

type nopWriter struct{ b *strings.Builder }

func (w *nopWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

// Сообщения с FILE_ID получают вложения из files[] того же ответа;
// «[Файл]» в тексте вычищается StripBB — остаётся только вложение.
func TestMessagesFromFiles(t *testing.T) {
	raw := []byte(`{"messages":[
		{"id":593865,"date":"2026-10-24T11:57:00+03:00","author_id":75,"text":"гейт: картинка",
		 "params":{"FILE_ID":[57693]}},
		{"id":593867,"date":"2026-10-24T11:57:10+03:00","author_id":75,"text":"[Файл]",
		 "params":{"FILE_ID":[57695,57696]}}
	],"users":[{"id":75,"name":"Лёня"}],"files":[
		{"id":57693,"type":"image","name":"gate-test.png","extension":"png","size":289},
		{"id":57695,"type":"file","name":"gate-test.txt","extension":"txt","size":20},
		{"id":57696,"type":"image","name":"без сообщения.png","extension":"png","size":100}
	]}`)
	rows := MessagesFrom(raw, 75, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), 0)
	if len(rows) != 2 {
		t.Fatalf("len = %d", len(rows))
	}
	cap := rows[0] // свежие сверху: id 593867
	if cap.ID != 593867 {
		t.Fatalf("порядок: %+v", rows)
	}
	if cap.Text != "" {
		t.Errorf("«[Файл]» должен уйти из текста: %q", cap.Text)
	}
	if len(cap.Files) != 2 || cap.Files[0].ID != 57695 || !cap.Files[1].IsImage() {
		t.Errorf("вложения: %+v", cap.Files)
	}
	if rows[1].Text != "гейт: картинка" || len(rows[1].Files) != 1 || !rows[1].Files[0].IsImage() {
		t.Errorf("подпись+картинка: text=%q files=%+v", rows[1].Text, rows[1].Files)
	}
}

// Превью списка чатов: «[Файл]» портала не должен оставлять пустоту.
func TestChatsFilePreview(t *testing.T) {
	raw := []byte(`{"items":[
		{"id":"75","title":"Ложкин Леонид","message":{"text":"[Файл]","date":"2026-10-24T11:57:00+03:00"}},
		{"id":"d1","title":"чат","message":{"text":"привет","date":"2026-10-24T11:50:00+03:00"}}
	]}`)
	chats := ChatsFrom(raw, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), 0)
	if chats[0].Preview != "📎 файл" {
		t.Errorf("превью файла: %q", chats[0].Preview)
	}
	if chats[1].Preview != "привет" {
		t.Errorf("обычное превью: %q", chats[1].Preview)
	}
}

// FileURL — im.v2.File.download отдаёт downloadUrl.
func TestFileURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/im.v2.File.download.json") {
			t.Errorf("path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"result":{"downloadUrl":"https://example.org/dl?token=1"}}`)
	}))
	defer srv.Close()
	a := &API{C: b24.New(srv.URL), My: 75}
	url, err := a.FileURL(context.Background(), 57693)
	if err != nil || url != "https://example.org/dl?token=1" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

// UploadFile — base64 content, message-подпись, из ответа берём оба id.
func TestUploadFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/im.v2.File.upload.json") {
			fmt.Fprint(w, `{"result":true}`)
			return
		}
		var req struct {
			DialogID string `json:"dialogId"`
			Fields   struct {
				Name    string `json:"name"`
				Content string `json:"content"`
				Message string `json:"message"`
			} `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.DialogID != "75" || req.Fields.Name != "x.png" {
			t.Errorf("запрос: %+v", req)
		}
		if req.Fields.Content != "aGk=" {
			t.Errorf("content не base64: %q", req.Fields.Content)
		}
		if req.Fields.Message != "подпись" {
			t.Errorf("подпись: %q", req.Fields.Message)
		}
		fmt.Fprint(w, `{"result":{"messageId":593865,"file":{"id":57693,"type":"image"}}}`)
	}))
	defer srv.Close()
	a := &API{C: b24.New(srv.URL), My: 75}
	msgID, fileID, err := a.UploadFile(context.Background(), "75", "x.png", []byte("hi"), "подпись")
	if err != nil || msgID != 593865 || fileID != 57693 {
		t.Fatalf("msgID=%d fileID=%d err=%v", msgID, fileID, err)
	}
}

// ensureFile: REST-ссылка → GET байтов → файл в кэше; повтор — без запросов.
func TestEnsureFile(t *testing.T) {
	testCache(t)
	pngBytes := testPNG(t)
	var rest, dl atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/im.v2.File.download.json", func(w http.ResponseWriter, r *http.Request) {
		rest.Add(1)
		fmt.Fprintf(w, `{"result":{"downloadUrl":"http://%s/file.bin"}}`, r.Host)
	})
	mux.HandleFunc("/file.bin", func(w http.ResponseWriter, r *http.Request) {
		dl.Add(1)
		w.Write(pngBytes)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	a := &API{C: b24.New(srv.URL)}
	att := Attachment{ID: 7, Type: "image", Name: "t.png", Ext: "png", Size: int64(len(pngBytes))}

	p, err := ensureFile(context.Background(), a, att)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("кэш-файла нет: %v", err)
	}
	if _, cached := cachedPath(att); !cached {
		t.Fatal("cachedPath не видит скачанное")
	}
	if _, err = ensureFile(context.Background(), a, att); err != nil {
		t.Fatalf("повтор: %v", err)
	}
	if rest.Load() != 1 || dl.Load() != 1 {
		t.Fatalf("повтор не должен качать заново: rest=%d dl=%d", rest.Load(), dl.Load())
	}
}

// renderImage: кэш есть — строки half-blocks; нет — nil.
func TestRenderImage(t *testing.T) {
	dir := testCache(t)
	pngBytes := testPNG(t)
	if err := os.MkdirAll(filepath.Join(dir, "omabitrix", "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "omabitrix", "files", "9.png"), pngBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	att := Attachment{ID: 9, Type: "image", Name: "t.png", Ext: "png"}
	img := renderImage(att, 20, 6)
	if len(img) == 0 || len(img) > 6 {
		t.Fatalf("превью: %d строк", len(img))
	}
	for _, l := range img {
		if l == "" {
			t.Fatal("пустая строка в превью")
		}
	}
	if got := renderImage(Attachment{ID: 10, Type: "image", Ext: "png"}, 20, 6); got != nil {
		t.Fatal("без кэша должно быть nil")
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{48: "48 Б", 2048: "2.0 КБ", 3 << 20: "3.0 МБ"}
	for n, want := range cases {
		if got := humanSize(n); got != want {
			t.Errorf("%d: %q != %q", n, got, want)
		}
	}
}

func TestIsImageName(t *testing.T) {
	if !isImageName("Photo.PNG") || isImageName("doc.pdf") {
		t.Fatal("расширения")
	}
}

// writeCached — байты сразу в кэш (своя отправка), превью без загрузки.
func TestWriteCached(t *testing.T) {
	dir := testCache(t)
	p := writeCached(11, "png", testPNG(t))
	if p == "" {
		t.Fatal("нет пути")
	}
	if _, cached := cachedPath(Attachment{ID: 11, Ext: "png"}); !cached {
		t.Fatal("не в кэше")
	}
	if _, err := os.Stat(filepath.Join(dir, "omabitrix", "files", "11.png")); err != nil {
		t.Fatal(err)
	}
}

// attachLines: файл — одна строка с именем и размером, картинка без кэша —
// плейсхолдер загрузки.
func TestAttachLines(t *testing.T) {
	testCache(t)
	msg := Message{Files: []Attachment{
		{ID: 1, Type: "file", Name: "doc.pdf", Size: 2048},
		{ID: 2, Type: "image", Name: "shot.png", Ext: "png"},
	}}
	lines := attachLines(msg, 60)
	if len(lines) != 2 {
		t.Fatalf("строк: %d", len(lines))
	}
	if !strings.Contains(lines[0], "doc.pdf") || !strings.Contains(lines[0], "2.0 КБ") {
		t.Errorf("строка файла: %q", lines[0])
	}
	if !strings.Contains(lines[1], "shot.png") || !strings.Contains(lines[1], "загрузка") {
		t.Errorf("плейсхолдер: %q", lines[1])
	}
}

// Отправка файла: enter в поле с прикреплённым файлом уходит sendFileCmd,
// ответ fileSentMsg дорисовывает оптимистичную историю с вложением.
func TestFileSendFlow(t *testing.T) {
	chats := []Chat{{DialogID: "d0", Title: "чат"}}
	active := chats[0]
	m := viewModel(120, 36, chats, nil, nil, &active, nil)
	m.attach = "/tmp/x.png"
	m.replyActive()

	mmI, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := mmI.(Model)
	if cmd == nil {
		t.Fatal("нет команды отправки")
	}
	if m2.attach != "" || !m2.sending {
		t.Fatalf("attach=%q sending=%v", m2.attach, m2.sending)
	}
	if _, ok := cmd().(fileSentMsg); !ok {
		t.Fatalf("команда не файловая: %T", cmd())
	}

	gotI, _ := m2.Update(fileSentMsg{dialogID: "d0", id: 593865, fileID: 57693,
		name: "x.png", size: 289, image: true})
	got := gotI.(Model)
	if got.attach != "" || got.sending {
		t.Fatalf("после ответа: attach=%q sending=%v", got.attach, got.sending)
	}
	if len(got.history) == 0 || len(got.history[len(got.history)-1].Files) != 1 {
		t.Fatal("оптимистичной записи с вложением нет")
	}
	if f := got.history[len(got.history)-1].Files[0]; f.ID != 57693 || !f.IsImage() {
		t.Errorf("вложение: %+v", f)
	}
}

// Прикрепление: a открывает ввод пути, enter с существующим файлом
// запоминает путь и возвращает фокус в поле ответа; мусорный путь — флеш.
func TestAttachByPath(t *testing.T) {
	chats := []Chat{{DialogID: "d0", Title: "чат"}}
	active := chats[0]
	m := viewModel(120, 36, chats, nil, nil, &active, nil)
	m.replyActive()

	mm, _ := keyPress(m, "a")
	if mm.focus != focusAttach {
		t.Fatalf("a не открыл ввод пути: %v", mm.focus)
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	mm.attachIn.SetValue(p)
	mm2I, _ := mm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := mm2I.(Model)
	if m2.attach != p || m2.focus != focusReply {
		t.Fatalf("attach=%q focus=%v", m2.attach, m2.focus)
	}
	if !strings.Contains(m2.dialogPane(), "f.txt") {
		t.Fatal("прикреплённый файл не показан в панели диалога")
	}

	mm4I, _ := keyPress(m2, "a")
	mm4 := mm4I
	mm4.attachIn.SetValue("/no/such/file")
	mm3I, _ := mm4.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m3 := mm3I.(Model)
	if m3.attach != p || m3.flash == "" {
		t.Fatalf("битый путь: флеш без смены файла; attach=%q flash=%q", m3.attach, m3.flash)
	}
}

// Выделенное сообщение с картинкой: enter открывает системный вьювер
// (после скачивания — «скачиваю…»), space — терминальный просмотр, esc закрывает.
func TestSelOpenImage(t *testing.T) {
	testCache(t)
	old := startOpener
	var got string
	startOpener = func(p string) error { got = p; return nil }
	defer func() { startOpener = old }()

	chats := []Chat{{DialogID: "d0", Title: "чат"}}
	active := chats[0]
	msgs := []Message{{ID: 1, Name: "А", Text: "смотри",
		Files: []Attachment{{ID: 5, Type: "image", Name: "p.png", Ext: "png"}}}}
	m := viewModel(120, 36, chats, nil, nil, &active, msgs)
	m.enterSel()

	// не скачано и нет клиента портала — флеш, не тишина
	mmI, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m1 := mmI.(Model)
	if m1.overlay != ovNone || m1.flash != "файл: нет клиента портала" {
		t.Fatalf("enter без клиента: overlay=%v flash=%q", m1.overlay, m1.flash)
	}

	writeCached(5, "png", testPNG(t))
	mm2I, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := mm2I.(Model)
	if m2.overlay != ovNone {
		t.Fatalf("enter открыл оверлей: %v", m2.overlay)
	}
	if !strings.HasSuffix(got, "5.png") {
		t.Fatalf("вьювер не открыт: %q", got)
	}

	// space — полноэкранный терминальный просмотр
	mm3I, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m3 := mm3I.(Model)
	if m3.overlay != ovImage {
		t.Fatalf("space не открыл просмотр: %v", m3.overlay)
	}
	if v := m3.View().Content; !strings.Contains(v, "p.png") {
		t.Fatal("в просмотре нет имени файла")
	}
	mm4, _ := keyPress(m3, "esc")
	if mm4.overlay != ovNone {
		t.Fatal("esc не закрыл просмотр")
	}
}

// enter по нескачанной картинке с клиентом портала: флеш «скачиваю»,
// команда скачивает и открывает системный вьювер.
func TestSelOpenImageDownloads(t *testing.T) {
	testCache(t)
	old := startOpener
	var got string
	startOpener = func(p string) error { got = p; return nil }
	defer func() { startOpener = old }()

	pngBytes := testPNG(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/im.v2.File.download.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"result":{"downloadUrl":"http://%s/file.bin"}}`, r.Host)
	})
	mux.HandleFunc("/file.bin", func(w http.ResponseWriter, r *http.Request) {
		w.Write(pngBytes)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	chats := []Chat{{DialogID: "d0", Title: "чат"}}
	active := chats[0]
	att := Attachment{ID: 9, Type: "image", Name: "t.png", Ext: "png"}
	msgs := []Message{{ID: 1, Name: "А", Text: "смотри", Files: []Attachment{att}}}
	m := viewModel(120, 36, chats, nil, nil, &active, msgs)
	m.api = &API{C: b24.New(srv.URL)}
	m.enterSel()

	m1, cmd := keyPress(m, "enter")
	if m1.flash != "скачиваю t.png…" {
		t.Fatalf("флеш: %q", m1.flash)
	}
	if cmd == nil {
		t.Fatal("нет команды скачивания")
	}
	if fm, ok := cmd().(fileMsg); !ok || fm.err != nil {
		t.Fatalf("итог команды: %T %+v", cmd(), fm)
	}
	if !strings.HasSuffix(got, "9.png") {
		t.Fatalf("вьювер не открыт: %q", got)
	}
	if _, cached := cachedPath(att); !cached {
		t.Fatal("файл не в кэше")
	}
}

// fileMsg снимает job и перерисовывает историю: плейсхолдер сменяется
// строками превью (кэш подложен через writeCached).
func TestFileMsgRendersPreview(t *testing.T) {
	dir := testCache(t)
	chats := []Chat{{DialogID: "d0", Title: "чат"}}
	active := chats[0]
	msgs := []Message{{ID: 1, Name: "А", Text: "",
		Files: []Attachment{{ID: 21, Type: "image", Name: "p.png", Ext: "png"}}}}
	m := viewModel(120, 36, chats, nil, nil, &active, msgs)
	if !strings.Contains(m.vp.View(), "загрузка") {
		t.Fatal("нет плейсхолдера до скачивания")
	}
	writeCached(21, "png", testPNG(t))
	m.fileJobs[21] = true

	m1I, _ := m.Update(fileMsg{att: msgs[0].Files[0], path: filepath.Join(dir, "omabitrix", "files", "21.png")})
	m1 := m1I.(Model)
	if m1.fileJobs[21] {
		t.Fatal("job не снят")
	}
	if !strings.Contains(m1.vp.View(), "48;2;") {
		t.Fatal("превью не отрисовалось")
	}
}

// fetchImagesCmd качает только некэшированные картинки, без дублей.
func TestFetchImagesCmd(t *testing.T) {
	dir := testCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"downloadUrl":"/f"}}`)
	}))
	defer srv.Close()
	a := &API{C: b24.New(srv.URL)}
	m := NewModel(a, nil)
	m.fileJobs = map[int64]bool{}
	m.history = []Message{
		{ID: 1, Files: []Attachment{{ID: 31, Type: "image", Ext: "png"}, {ID: 32, Type: "file"}}},
		{ID: 2, Files: []Attachment{{ID: 31, Type: "image", Ext: "png"}}},
	}
	cmd := m.fetchImagesCmd()
	if cmd == nil {
		t.Fatal("нет команды загрузки")
	}
	if m.fileJobs[31] != true {
		t.Fatal("job не отмечен")
	}
	// вторая попытка — всё в полёте, команды нет
	if m.fetchImagesCmd() != nil {
		t.Fatal("дубликат загрузки")
	}
	os.MkdirAll(filepath.Join(dir, "omabitrix", "files"), 0o755)
	os.WriteFile(filepath.Join(dir, "omabitrix", "files", "31.png"), testPNG(t), 0o644)
	delete(m.fileJobs, 31)
	if m.fetchImagesCmd() != nil {
		t.Fatal("кэшированное не должно качаться")
	}
}

// Клик ctrl+v без wl-paste в системе — ошибка возвращается, не паника.
func TestClipboardFallback(t *testing.T) {
	old := wlPaste
	wlPaste = func() ([]byte, error) { return nil, errors.New("no wayland") }
	defer func() { wlPaste = old }()

	chats := []Chat{{DialogID: "d0", Title: "чат"}}
	active := chats[0]
	m := viewModel(120, 36, chats, nil, nil, &active, nil)
	m.replyActive()
	mm, cmd := keyPress(m, "ctrl+v")
	m1 := mm
	if !m1.sending {
		t.Fatal("отправка не начата")
	}
	msg := cmd().(fileSentMsg)
	if msg.err == nil {
		t.Fatal("должна быть ошибка буфера")
	}
	m2, _ := m1.Update(msg)
	if g := m2.(Model); g.sending {
		t.Fatal("флаг sending не снят")
	}
}

// Клик мышью по блоку вложения открывает системный вьювер (как enter) —
// из любого фокуса; клик по тексту сообщения — нет.
func TestClickAttachmentOpens(t *testing.T) {
	testCache(t)
	old := startOpener
	var got string
	startOpener = func(p string) error { got = p; return nil }
	defer func() { startOpener = old }()

	writeCached(41, "png", testPNG(t))
	chats := []Chat{{DialogID: "d0", Title: "чат"}}
	active := chats[0]
	msgs := []Message{{ID: 1, Name: "А", Text: "смотри",
		Files: []Attachment{{ID: 41, Type: "image", Name: "p.png", Ext: "png"}}}}
	m := viewModel(120, 36, chats, nil, nil, &active, msgs)
	span, ok := m.attachLine[1]
	if !ok || span[0] < 0 || span[1] <= span[0] {
		t.Fatalf("диапазон вложений не записан: %+v", span)
	}
	x := m.listW + 10
	got = ""
	mmI, _ := m.Update(tea.MouseClickMsg{X: x, Y: 2 + span[0], Button: tea.MouseLeft})
	if m1 := mmI.(Model); m1.overlay != ovNone {
		t.Fatalf("клик открыл оверлей: %v", m1.overlay)
	}
	if !strings.HasSuffix(got, "41.png") {
		t.Fatalf("вьювер не открыт кликом: %q", got)
	}
	// тот же клик из поля ответа
	got = ""
	m.replyActive()
	mm2I, _ := m.Update(tea.MouseClickMsg{X: x, Y: 2 + span[0], Button: tea.MouseLeft})
	if m2 := mm2I.(Model); m2.overlay != ovNone {
		t.Fatalf("клик из поля ответа открыл оверлей: %v", m2.overlay)
	}
	if !strings.HasSuffix(got, "41.png") {
		t.Fatalf("клик из поля ответа не открыл вьювер: %q", got)
	}
	// клик по строке текста (первая строка блока — имя автора) — ничего
	mm3I, _ := m.Update(tea.MouseClickMsg{X: x, Y: 2 + m.msgLine[1], Button: tea.MouseLeft})
	if m3 := mm3I.(Model); m3.overlay != ovNone {
		t.Fatalf("клик по тексту открыл что-то: %v", m3.overlay)
	}
}

// Скачанное, но не декодируемое: строка «enter — открыть», не вечная загрузка.
func TestAttachBrokenCached(t *testing.T) {
	testCache(t)
	writeCached(51, "png", []byte("это не png"))
	msg := Message{Files: []Attachment{{ID: 51, Type: "image", Name: "bad.png", Ext: "png"}}}
	lines := attachLines(msg, 60)
	if len(lines) != 1 || !strings.Contains(lines[0], "enter — открыть") {
		t.Fatalf("битое превью: %q", lines[0])
	}
}
