package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var trailerT0 = time.Date(2026, 10, 24, 18, 0, 0, 0, time.Local)

func trailerModel(w, h int) Trailer {
	mm, _ := NewTrailer(&API{Portal: "https://portal.example.com"}, nil).Update(tea.WindowSizeMsg{Width: w, Height: h})
	mm, _ = mm.Update(trailerMsg(trailerT0))
	return mm.(Trailer)
}

func trailerAt(tr Trailer, sec float64) Trailer {
	mm, _ := tr.Update(trailerMsg(trailerT0.Add(time.Duration(sec * float64(time.Second)))))
	return mm.(Trailer)
}

// Каждый кадр ролика, включая вклейку живого приложения, влезает в экран.
func TestTrailerFitsScreen(t *testing.T) {
	for _, sz := range [][2]int{{154, 39}, {200, 50}, {100, 30}} {
		w, h := sz[0], sz[1]
		tr := trailerModel(w, h)
		for sec := 0.0; sec < atOutro+7; sec += 0.1 {
			tr = trailerAt(tr, sec)
			lines := strings.Split(tr.View().Content, "\n")
			if len(lines) > h {
				t.Fatalf("%dx%d t=%.1f: %d строк > %d", w, h, sec, len(lines), h)
			}
			for i, l := range lines {
				if lw := lipgloss.Width(ansi.Strip(l)); lw > w {
					t.Fatalf("%dx%d t=%.1f: строка %d шириной %d > %d: %q", w, h, sec, i, lw, w, ansi.Strip(l))
				}
			}
		}
	}
}

// Сценарий жмёт клавиши живой модели и заканчивает на уведах.
func TestTrailerScriptDrivesApp(t *testing.T) {
	tr := trailerModel(154, 39)
	tr = trailerAt(tr, atOpen-0.1)
	if tr.appOn {
		t.Fatal("приложение стартовало до открытия окна")
	}
	tr = trailerAt(tr, atOpen+0.1)
	if !tr.appOn || tr.app.width != 148 || tr.app.height != 29 {
		t.Fatalf("приложение в окне: on=%v %dx%d", tr.appOn, tr.app.width, tr.app.height)
	}
	tr = trailerAt(tr, atClose)
	if tr.next != len(tr.steps) {
		t.Fatalf("сыграно шагов %d из %d", tr.next, len(tr.steps))
	}
	if tr.app.tab != tabNotify {
		t.Fatalf("сценарий закончился на вкладке %v, ждём уведы", tr.app.tab)
	}
}

// Внутри окна — кадр самого приложения, под окном — подпись сцены.
func TestTrailerSplicesApp(t *testing.T) {
	tr := trailerModel(154, 39)
	tr = trailerAt(tr, atOpen+0.1)
	tr.app.intro = nil
	tr.app.ready = true
	tr = trailerAt(tr, 13)
	v := ansi.Strip(tr.View().Content)
	for _, want := range []string{"omabitrix", "1 пульт", "весь день на одном экране", "╭"} {
		if !strings.Contains(v, want) {
			t.Fatalf("в кадре нет %q:\n%s", want, v)
		}
	}
}

// Финал: кольцо брайлем, вордмарк, ссылка на репо и команда демо.
func TestTrailerOutro(t *testing.T) {
	tr := trailerAt(trailerModel(154, 39), atOutro+6)
	v := ansi.Strip(tr.View().Content)
	for _, want := range []string{Logo("omabitrix")[0], "github.com/oglenyaboss/omabitrix", "@latest -demo"} {
		if !strings.Contains(v, want) {
			t.Fatalf("в финале нет %q:\n%s", want, v)
		}
	}
	dots := 0
	for _, r := range v {
		if r > 0x2800 && r <= 0x28ff {
			dots++
		}
	}
	if dots < 20 {
		t.Fatalf("нет кольца брайлем (%d клеток)", dots)
	}
}

// Крупный текст: брайлевая маска строки непустая и шире для длинной строки.
func TestTextMask(t *testing.T) {
	a, b := textMask("чаты", capPx), textMask("картинки", capPx)
	if len(a.pts) == 0 || b.w <= a.w || a.h != b.h {
		t.Fatalf("маски: %d точек, ширины %d/%d, высоты %d/%d", len(a.pts), a.w, b.w, a.h, b.h)
	}
}
