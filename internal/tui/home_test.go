package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// homeModel — пульт с фиксированным временем: пн 19.10.2026 16:55 (UTC).
func homeModel(w, h int, chats []Chat, tasks []Task) Model {
	m := NewModel(&API{}, nil)
	m.width, m.height = w, h
	m.ready = true
	m.tab = tabHome
	m.chats = chats
	m.tasks = tasks
	m.api.Now = func() time.Time { return time.Date(2026, 10, 19, 16, 55, 0, 0, time.UTC) }
	m.buckets = Buckets(m.tasks, time.UTC, m.api.Now())
	m.layout()
	return m
}

// homeFixture — просрочка, 2 сегодня, позже; стадии как на портале; 2 чата.
func homeFixture() ([]Chat, []Task) {
	chats := []Chat{
		{DialogID: "chat12", Title: "Смирнова Анна", Counter: 2, Preview: "ща 10-15 мин", Time: "13:48"},
		{DialogID: "1", Title: "Орлова Вера", Counter: 1, Preview: "а колибри по хаусу?", Time: "16:47"},
		{DialogID: "chat7", Title: "Команда", Counter: 0, Preview: "прочитано", Time: "12:54"},
	}
	tasks := []Task{
		{ID: 7, Title: "SRV. Проверить отработку коллтрекинга", Deadline: "2026-10-18 12:00:00",
			Role: "do", Project: "baden-apart", NewComments: 1, Stage: "DOING", StageColor: "75d900", StageSort: 3},
		{ID: 8, Title: "CLS. Обновить фид", Deadline: "2026-10-19 18:00:00",
			Role: "do", Project: "baden-apart", Stage: "REVIEW", StageColor: "ffab00", StageSort: 4},
		{ID: 9, Title: "SRV. Скорректировать отчёт по Roistat", Deadline: "2026-10-19 15:00:00",
			Role: "do", Project: "cleverpark.life", Stage: "DOING", StageColor: "75d900", StageSort: 3},
		{ID: 10, Title: "Отпустить релиз", Deadline: "2026-10-23 18:00:00",
			Role: "do", Project: "cleverpark.life", Stage: "TODO", StageColor: "47d1e2", StageSort: 2},
	}
	return chats, tasks
}

// Пульт вписывается в экран на всех вкладках размеров: высота ≤ h, строки ≤ w.
func TestHomeFitsScreen(t *testing.T) {
	chats, tasks := homeFixture()
	for _, f := range []struct {
		chats []Chat
		tasks []Task
	}{
		{chats, tasks}, {nil, nil},
	} {
		for _, w := range []int{80, 100, 120, 200} {
			for _, h := range []int{24, 30, 36, 50} {
				m := homeModel(w, h, f.chats, f.tasks)
				v := m.View().Content
				if got := lipgloss.Height(v); got > h {
					t.Errorf("w=%d h=%d: высота %d > %d", w, h, got, h)
				}
				for i, line := range strings.Split(v, "\n") {
					if lw := lipgloss.Width(line); lw > w {
						t.Errorf("w=%d h=%d: строка %d шире %d: %d", w, h, i, w, lw)
						break
					}
				}
			}
		}
	}
}

// На 120×36 в кадре все виджеты и ближайший срок — просрочка.
func TestHomeContent(t *testing.T) {
	chats, tasks := homeFixture()
	m := homeModel(120, 36, chats, tasks)
	s := ansi.Strip(m.View().Content)
	for _, want := range []string{
		"следующий срок", "октябрь", "чаты", "неделя", "канбан",
		"SRV. Проверить отработку коллтрекинга", "просрочено на",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("в пульте нет %q:\n%s", want, s)
		}
	}
}

// nextDue: просрочка раньше сегодняшних; без просрочки — ближайшая
// сегодняшняя; пусто — false.
func TestHomeNextDueOrder(t *testing.T) {
	chats, tasks := homeFixture()
	m := homeModel(120, 36, chats, tasks)
	if got, ok := m.nextDue(); !ok || got.ID != 7 {
		t.Fatalf("nextDue с просрочкой: %+v ok=%v", got, ok)
	}
	var without []Task
	for _, x := range tasks {
		if x.ID != 7 {
			without = append(without, x)
		}
	}
	m2 := homeModel(120, 36, nil, without)
	if got, ok := m2.nextDue(); !ok || got.ID != 9 { // 15:00 раньше 18:00
		t.Fatalf("nextDue без просрочки: %+v ok=%v", got, ok)
	}
	m3 := homeModel(120, 36, nil, nil)
	if _, ok := m3.nextDue(); ok {
		t.Fatal("nextDue на пустом списке вернул задачу")
	}
}

// Клетка недели сжимается под ширину: подписи дней не режутся рамкой.
func TestHomeWeekNarrow(t *testing.T) {
	chats, tasks := homeFixture()
	for _, w := range []int{100, 104, 110, 117} {
		m := homeModel(w, 36, chats, tasks)
		lines := strings.Split(m.View().Content, "\n")
		found := false
		for i, l := range lines {
			s := ansi.Strip(l)
			// строка лейблов недели: над ней базовая линия — не шапка календаря
			if strings.Contains(s, "пн") && strings.Contains(s, "вс") &&
				i > 0 && strings.Contains(ansi.Strip(lines[i-1]), "──") {
				found = true // «вс» не обрезан — иначе бы его не было в строке
			}
		}
		if !found {
			t.Fatalf("w=%d: строки лейблов недели нет в кадре", w)
		}
	}
}

// n/enter открывают ближайшую задачу; без задач — флеш.
func TestHomeOpenNextDue(t *testing.T) {
	chats, tasks := homeFixture()
	m := homeModel(120, 36, chats, tasks)
	m2, _ := keyPress(m, "n")
	if m2.tab != tabTasks || m2.cardReq != 7 || !m2.cardLoading {
		t.Fatalf("n: tab=%v cardReq=%d loading=%v", m2.tab, m2.cardReq, m2.cardLoading)
	}
	m3, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m4 := m3.(Model); m4.tab != tabTasks || m4.cardReq != 7 || !m4.cardLoading {
		t.Fatalf("enter: tab=%v cardReq=%d loading=%v", m4.tab, m4.cardReq, m4.cardLoading)
	}
	m5 := homeModel(120, 36, nil, nil)
	m6, _ := keyPress(m5, "n")
	if m6.flash != "сроков нет" || m6.tab != tabHome {
		t.Fatalf("n без задач: flash=%q tab=%v", m6.flash, m6.tab)
	}
}
