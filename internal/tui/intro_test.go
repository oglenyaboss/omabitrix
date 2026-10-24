package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func introModel(w, h int) Model {
	m := NewModel(&API{Portal: "https://portal.example.com"}, nil)
	m.PlayIntro()
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return mm.(Model)
}

func introStep(m Model, msg tea.Msg) Model {
	mm, _ := m.Update(msg)
	return mm.(Model)
}

func introFrames(m Model, n int) Model {
	for i := 0; i < n; i++ {
		m = introStep(m, introMsg{})
	}
	return m
}

func introLoadAll(m Model) Model {
	m = introStep(m, reloadMsg{chats: true})
	m = introStep(m, reloadMsg{tasks: true})
	return introStep(m, reloadMsg{notify: true})
}

func assertFits(t *testing.T, m Model, w, h int, at string) {
	t.Helper()
	lines := strings.Split(m.View().Content, "\n")
	if len(lines) > h {
		t.Fatalf("%dx%d %s: %d строк > %d", w, h, at, len(lines), h)
	}
	for i, l := range lines {
		if lw := lipgloss.Width(ansi.Strip(l)); lw > w {
			t.Fatalf("%dx%d %s: строка %d шириной %d > %d: %q", w, h, at, i, lw, w, ansi.Strip(l))
		}
	}
}

// Каждый кадр заставки, включая шторку, влезает в экран любого размера.
func TestIntroFitsScreen(t *testing.T) {
	for _, sz := range [][2]int{{120, 36}, {101, 22}, {100, 30}, {60, 24}, {40, 12}, {20, 5}, {200, 60}} {
		w, h := sz[0], sz[1]
		m := introModel(w, h)
		for f := 0; f < introHold+introOut+10 && m.intro != nil; f++ {
			if f == 10 {
				m = introLoadAll(m)
			}
			assertFits(t, m, w, h, "кадр")
			m = introStep(m, introMsg{})
		}
		if m.intro != nil {
			t.Fatalf("%dx%d: заставка не закончилась", w, h)
		}
	}
}

// Данные пришли сразу — заставка всё равно доигрывает анимацию.
func TestIntroWaitsForAnimation(t *testing.T) {
	m := introLoadAll(introModel(120, 36))
	m = introFrames(m, introHold-1)
	if m.intro == nil {
		t.Fatal("заставка ушла до конца анимации")
	}
	m = introFrames(m, introOut+2)
	if m.intro != nil {
		t.Fatal("заставка не ушла после анимации и загрузки")
	}
}

// Пока не пришли все три ветки — держим заставку; пришли — уходим шторкой.
func TestIntroWaitsForData(t *testing.T) {
	m := introModel(120, 36)
	m = introStep(m, reloadMsg{chats: true})
	m = introStep(m, reloadMsg{tasks: true})
	m = introFrames(m, 200)
	if m.intro == nil {
		t.Fatal("заставка ушла без уведов")
	}
	m = introStep(m, reloadMsg{notify: true})
	m = introFrames(m, introOut+2)
	if m.intro != nil {
		t.Fatal("заставка не ушла после загрузки")
	}
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "пульт") {
		t.Fatalf("после заставки нет основного экрана:\n%s", v)
	}
}

// Ошибка ветки не держит заставку: уходим в основной экран, там красная точка.
func TestIntroErrorBranch(t *testing.T) {
	m := introModel(120, 36)
	m = introStep(m, reloadMsg{chats: true})
	m = introStep(m, reloadMsg{tasks: true, errs: []string{"tasks.task.list: 503"}})
	m = introFrames(m, introHold)
	v := ansi.Strip(m.View().Content)
	if !strings.Contains(v, "✗") || !strings.Contains(v, "tasks.task.list: 503") {
		t.Fatalf("в чеклисте нет ошибки задач:\n%s", v)
	}
	m = introStep(m, reloadMsg{notify: true})
	m = introFrames(m, introOut+2)
	if m.intro != nil {
		t.Fatal("ошибка ветки застопорила заставку")
	}
}

// Любая клавиша пропускает заставку и применяется (как у старого сплэша).
func TestIntroKeySkips(t *testing.T) {
	m := introFrames(introModel(120, 36), 5)
	m, _ = keyPress(m, "2")
	if m.intro != nil {
		t.Fatal("клавиша не пропустила заставку")
	}
	if !m.ready || m.tab != tabTasks {
		t.Fatalf("клавиша не применилась: ready=%v tab=%v", m.ready, m.tab)
	}
}

// Мышь по заставке не кликает по невидимому экрану под ней.
func TestIntroIgnoresMouse(t *testing.T) {
	m := introLoadAll(introFrames(introModel(120, 36), 5))
	tab := m.tab
	m = introStep(m, tea.MouseClickMsg{X: 30, Y: 0, Button: tea.MouseLeft})
	if m.intro == nil || m.tab != tab {
		t.Fatalf("клик прошёл сквозь заставку: intro=%v tab=%v", m.intro != nil, m.tab)
	}
}

// Финальный кадр широкого экрана: кольцо брайлем, «24», вордмарк,
// подзаголовок и чеклист с цифрами.
func TestIntroLockup(t *testing.T) {
	m := introModel(120, 36)
	m = introStep(m, reloadMsg{chats: true})
	m = introStep(m, reloadMsg{tasks: true})
	m.chats = []Chat{{DialogID: "1", Counter: 2}, {DialogID: "2"}}
	m = introFrames(m, introHold+5) // уведы ещё грузятся — кадр держится
	v := ansi.Strip(m.View().Content)
	for _, want := range []string{Logo("omabitrix")[0], logoSub, "portal.example.com", "чаты", "непрочитанных: 1", "задачи", "уведы", "✓", "██"} {
		if !strings.Contains(v, want) {
			t.Fatalf("в кадре нет %q:\n%s", want, v)
		}
	}
	dots := 0
	for _, r := range v {
		if r > 0x2800 && r <= 0x28ff {
			dots++
		}
	}
	if dots < 20 {
		t.Fatalf("нет кольца брайлем (%d клеток):\n%s", dots, v)
	}
}

// Без PlayIntro (deep-link из тоста) — старый путь, заставки нет.
func TestNoIntroByDefault(t *testing.T) {
	m := NewModel(&API{}, nil)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	if mm.(Model).intro != nil {
		t.Fatal("заставка включилась без PlayIntro")
	}
}
