package tui

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/mosaic"
	_ "golang.org/x/image/webp"
)

// Вложения: кэш на диске, инлайн-превью картинок (half-blocks, mosaic —
// работает в любом truecolor-терминале, скролл-безопасно), буфер картинок
// Wayland. Прямые urlDownload из files[] сессионные — байты берём
// im.v2.File.download → GET подписанной ссылки (живая проверка).

// userCacheDir — точка подмены в тестах.
var userCacheDir = os.UserCacheDir

// imgRenderCache — строки рендера по «fileID:ширина»: рендер дороже декода,
// история перерисовывается на каждый тик.
var imgRenderCache sync.Map

// wlPaste — картинка из буфера Wayland (Omarchy); точка подмены в тестах.
var wlPaste = func() ([]byte, error) {
	out, err := exec.Command("wl-paste", "-t", "image/png").Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// fileMsg — итог загрузки файла: path при ok, иначе err.
type fileMsg struct {
	att  Attachment
	path string
	err  error
}

// cacheDir — ~/.cache/omabitrix/files.
func cacheDir() (string, error) {
	root, err := userCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "omabitrix", "files"), nil
}

// cachedPath — путь в кэше, true — файл уже скачан. Диск-файл считаем
// неизменяемым (версии Диска дают новый id).
func cachedPath(att Attachment) (string, bool) {
	dir, err := cacheDir()
	if err != nil {
		return "", false
	}
	ext := att.Ext
	if ext == "" {
		ext = strings.TrimPrefix(filepath.Ext(att.Name), ".")
	}
	p := filepath.Join(dir, fmt.Sprintf("%d.%s", att.ID, ext))
	if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Size() > 0 {
		return p, true
	}
	return p, false
}

// ensureFile — ссылка через REST, GET байтов, запись tmp+rename.
func ensureFile(ctx context.Context, a *API, att Attachment) (string, error) {
	if a == nil || a.C == nil {
		return "", errors.New("нет клиента портала")
	}
	p, ok := cachedPath(att)
	if ok {
		return p, nil
	}
	url, err := a.FileURL(ctx, att.ID)
	if err != nil {
		return "", err
	}
	hc := a.C.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("файл: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 100<<20))
	if err != nil {
		return "", err
	}
	if dir := filepath.Dir(p); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return "", err
	}
	return p, os.Rename(tmp, p)
}

// fetchFileCmd — скачать файл молча: ответ приходит fileMsg-ом.
func fetchFileCmd(a *API, att Attachment) tea.Cmd {
	return func() tea.Msg {
		p, err := ensureFile(context.Background(), a, att)
		return fileMsg{att: att, path: p, err: err}
	}
}

// renderImage — строки half-block-превью картинки из кэша: ширина w ячеек,
// высота не выше maxH (аспект сохраняем, символ ячейки вдвое выше пиксельного
// ряда — потому /2). Кэш по «id:w:maxH».
// out — строки mosaic без хвостовых пустых (strings.Split даёт их после
// завершающего \n).
func renderImage(att Attachment, w, maxH int) []string {
	if w < 2 {
		w = 2
	}
	key := fmt.Sprintf("%d:%d:%d", att.ID, w, maxH)
	if v, ok := imgRenderCache.Load(key); ok {
		return v.([]string)
	}
	p, ok := cachedPath(att)
	if !ok {
		return nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW < 1 || srcH < 1 {
		return nil
	}
	cw := w
	ch := srcH * w / srcW / 2
	if ch < 1 {
		ch = 1
	}
	if ch > maxH {
		ch = maxH
		cw = srcW * ch * 2 / srcH
		if cw < 1 {
			cw = 1
		}
		if cw > w {
			cw = w
		}
	}
	// mosaic считает w/h пикселями целевого буфера, а ячейки — половина —
	// удваиваем, чтобы получить cw колонок и ch строк.
	out := strings.Split(mosaic.Render(img, cw*2, ch*2), "\n")
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return nil
	}
	imgRenderCache.Store(key, out)
	return out
}

// humanSize — «12,3 КБ» / «2,1 МБ» / «48 Б».
func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f МБ", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f КБ", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d Б", n)
	}
}

// clipboardImageName — «clipboard-20261024-120000.png».
func clipboardImageName(now time.Time) string {
	return "clipboard-" + now.Format("20060102-150405") + ".png"
}

// writeCached — положить байты в кэш под id файла (своя отправка:
// превью рисуется сразу, без повторной загрузки).
func writeCached(id int64, ext string, content []byte) string {
	p, _ := cachedPath(Attachment{ID: id, Ext: ext})
	if p == "" {
		return ""
	}
	if dir := filepath.Dir(p); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return ""
	}
	if err := os.Rename(tmp, p); err != nil {
		return ""
	}
	return p
}

// openFileCmd — скачать в кэш и открыть системным вьювером: итог — fileMsg.
func openFileCmd(a *API, att Attachment) tea.Cmd {
	return func() tea.Msg {
		p, err := ensureFile(context.Background(), a, att)
		if err == nil {
			err = startOpener(p)
		}
		return fileMsg{att: att, path: p, err: err}
	}
}

// isImageName — расширение картинки (оптимистичная история своей отправки).
func isImageName(name string) bool {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")) {
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "heic", "avif":
		return true
	}
	return false
}

// fileTypeName — «image»/«file» для оптимистичной истории.
func fileTypeName(image bool) string {
	if image {
		return "image"
	}
	return "file"
}

// thumbMaxW/thumbMaxH — потолок инлайн-превью в истории: компактно,
// как миниатюры в телеге; полный размер — enter (системный вьювер)
// или space (ovImage).
const (
	thumbMaxW = 44
	thumbMaxH = 10
)

// attachLines — блок вложений сообщения для истории: картинка — инлайн
// half-blocks (пока не скачалась — плейсхолдер), остальное — строка с именем.
func attachLines(msg Message, w int) []string {
	var out []string
	tw := min(w-4, thumbMaxW)
	if tw < 2 {
		tw = 2
	}
	for _, f := range msg.Files {
		switch {
		case f.IsImage() && f.ID > 0:
			img := renderImage(f, tw, thumbMaxH)
			_, cached := cachedPath(f)
			switch {
			case len(img) > 0:
				for _, l := range img {
					out = append(out, "  "+l)
				}
			case cached:
				// скачано, но не декодируется (живой факт гейта): без вечной «загрузки»
				out = append(out, stDim.Render("  📎 "+truncate(f.Name, max(w-2, 4))+" · enter — открыть"))
			default:
				out = append(out, stDim.Render("  [изображение] "+f.Name+" — загрузка…"))
			}
		default:
			label := f.Name
			if f.Size > 0 {
				label += " (" + humanSize(f.Size) + ")"
			}
			out = append(out, stDim.Render("  📎 "+truncate(label, max(w-2, 4))))
		}
	}
	return out
}
