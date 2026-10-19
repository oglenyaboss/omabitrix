package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// boardNow — фиксированное «сейчас» доски: пн 19 окт 2026, полдень UTC.
var boardNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// boardModel — доска: фиксированное время и канбан проекта 1 (3 стадии).
func boardModel(tasks []Task) Model {
	m := viewModel(120, 36, nil, tasks, nil, nil, nil)
	m.tab = tabTasks
	m.api = &API{Now: func() time.Time { return boardNow }}
	m.api.stages = map[int64][]Stage{
		1: {
			{ID: 10, Title: "INBOX", Color: "00c4fb"},
			{ID: 20, Title: "TODO", Color: "47d1e2"},
			{ID: 30, Title: "DOING", Color: "75d900"},
		},
	}
	return m
}

// Группировка по стадиям: порядок колонок по StageSort, пустая стадия —
// последней, внутри колонки — по дедлайну (без срока — в конце).
func TestBoardColumnsOrder(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "без стадии", Role: "do"},
		{ID: 2, Title: "инбокс без срока", Role: "do", GroupID: 1, Project: "ПРО",
			StageID: 10, Stage: "INBOX", StageColor: "00c4fb", StageSort: 0},
		{ID: 3, Title: "инбокс со сроком", Role: "do", GroupID: 1,
			StageID: 10, Stage: "INBOX", StageColor: "00c4fb", StageSort: 0, Deadline: "2026-10-19 14:00:00"},
		{ID: 4, Title: "туду", Role: "do", GroupID: 1, StageID: 20, Stage: "TODO", StageColor: "47d1e2", StageSort: 1},
		{ID: 5, Title: "дуинг", Role: "do", GroupID: 1, StageID: 30, Stage: "DOING", StageColor: "75d900", StageSort: 2},
	}
	m := boardModel(tasks)
	cols := m.boardColumns()
	want := []string{"INBOX", "TODO", "DOING", "—"}
	if len(cols) != len(want) {
		t.Fatalf("колонок %d, хочу %d", len(cols), len(want))
	}
	for i, w := range want {
		if cols[i].title != w {
			t.Fatalf("колонка %d: %q, хочу %q", i, cols[i].title, w)
		}
	}
	if cols[0].color != "00c4fb" {
		t.Fatalf("цвет колонки %q, хочу 00c4fb", cols[0].color)
	}
	// внутри INBOX: с дедлайном раньше бездедлайнного
	if cols[0].rows[0].task.ID != 3 || cols[0].rows[1].task.ID != 2 {
		t.Fatalf("порядок в INBOX: %d,%d, хочу 3,2",
			cols[0].rows[0].task.ID, cols[0].rows[1].task.ID)
	}
	// flatTasks — колонка за колонкой, col проставлен
	rows := m.flatTasks()
	for i, wantID := range []int64{3, 2, 4, 5, 1} {
		if rows[i].task.ID != wantID {
			t.Fatalf("flatTasks[%d]: задача %d, хочу %d", i, rows[i].task.ID, wantID)
		}
	}
	if rows[0].col != 0 || rows[2].col != 1 || rows[4].col != 3 {
		t.Fatalf("колонки строк: %d,%d,%d, хочу 0,1,3", rows[0].col, rows[2].col, rows[4].col)
	}
}

// Фильтры роли (t) и проекта (p): цикл ролей и проектов, курсор на 0.
func TestBoardRoleAndProjectFilter(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "делаю альфа", Role: "do", GroupID: 1, Project: "Альфа", Stage: "INBOX", StageSort: 0},
		{ID: 2, Title: "помогаю", Role: "help", GroupID: 1, Project: "Альфа", Stage: "INBOX", StageSort: 0},
		{ID: 3, Title: "наблюдаю", Role: "watch", GroupID: 2, Project: "Бета", Stage: "INBOX", StageSort: 0},
		{ID: 4, Title: "делаю бета", Role: "do", GroupID: 2, Project: "Бета", Stage: "INBOX", StageSort: 0},
	}
	m := boardModel(tasks)
	ids := func(mm Model) []int64 {
		var out []int64
		for _, r := range mm.flatTasks() {
			out = append(out, r.task.ID)
		}
		return out
	}
	eq := func(got []int64, want ...int64) {
		if len(got) != len(want) {
			t.Fatalf("задачи %v, хочу %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("задачи %v, хочу %v", got, want)
			}
		}
	}
	eq(ids(m), 1, 4)

	m2, _ := keyPress(m, "t") // → помогаю
	if m2.taskRole != "help" || m2.cursor != 0 {
		t.Fatalf("t: role=%q cursor=%d", m2.taskRole, m2.cursor)
	}
	eq(ids(m2), 2)
	m3, _ := keyPress(m2, "t") // → наблюдаю
	eq(ids(m3), 3)
	m4, _ := keyPress(m3, "t") // → снова делаю
	if m4.taskRole != "do" {
		t.Fatalf("цикл ролей сломан: %q", m4.taskRole)
	}
	eq(ids(m4), 1, 4)

	// p: проекты роли do — Альфа (1), Бета (2); 0 → 1 → 2 → 0
	m5, _ := keyPress(m4, "p")
	if m5.taskProject != 1 {
		t.Fatalf("p: taskProject=%d, хочу 1 (Альфа)", m5.taskProject)
	}
	eq(ids(m5), 1)
	m6, _ := keyPress(m5, "p")
	if m6.taskProject != 2 {
		t.Fatalf("p: taskProject=%d, хочу 2 (Бета)", m6.taskProject)
	}
	eq(ids(m6), 4)
	m7, _ := keyPress(m6, "p")
	if m7.taskProject != 0 {
		t.Fatalf("p: taskProject=%d, хочу 0 (все)", m7.taskProject)
	}
	eq(ids(m7), 1, 4)
}

// h/l — переход между колонками на ту же строку с клампом; j/k не выходят
// за пределы колонки.
func TestBoardColNav(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "a1", Role: "do", Stage: "A", StageSort: 0},
		{ID: 2, Title: "a2", Role: "do", Stage: "A", StageSort: 0},
		{ID: 3, Title: "b1", Role: "do", Stage: "B", StageSort: 1},
		{ID: 4, Title: "b2", Role: "do", Stage: "B", StageSort: 1},
		{ID: 5, Title: "b3", Role: "do", Stage: "B", StageSort: 1},
	}
	m := boardModel(tasks)
	if got := m.flatTasks(); len(got) != 5 || got[0].task.ID != 1 || got[2].task.ID != 3 {
		t.Fatalf("плоский список: %+v", got)
	}
	m2, _ := keyPress(m, "l") // колонка B, строка 0 → задача 3
	if m2.cursor != 2 {
		t.Fatalf("l: cursor=%d, хочу 2", m2.cursor)
	}
	m3, _ := keyPress(m2, "l") // край — ничего
	if m3.cursor != 2 {
		t.Fatalf("l на краю: cursor=%d", m3.cursor)
	}
	// кламп: из строки 1 колонки A в B — строка 1 (в A строк нет длиннее)
	m4 := m
	m4.cursor = 1
	m5, _ := keyPress(m4, "l")
	if m5.cursor != 3 {
		t.Fatalf("l с клампом: cursor=%d, хочу 3", m5.cursor)
	}
	m6, _ := keyPress(m5, "h")
	if m6.cursor != 1 {
		t.Fatalf("h: cursor=%d, хочу 1", m6.cursor)
	}
	m7, _ := keyPress(m6, "h") // край слева — ничего
	if m7.cursor != 1 {
		t.Fatalf("h на краю: cursor=%d", m7.cursor)
	}
	// j не выходит из колонки: строка 0 колонки A, j — задача 2, ещё j — на месте
	m8, _ := keyPress(m, "j")
	if m8.cursor != 1 {
		t.Fatalf("j: cursor=%d, хочу 1", m8.cursor)
	}
	m9, _ := keyPress(m8, "j")
	if m9.cursor != 1 {
		t.Fatalf("j вышел из колонки: cursor=%d", m9.cursor)
	}
	// k симметрично
	m10, _ := keyPress(m9, "k")
	if m10.cursor != 0 {
		t.Fatalf("k: cursor=%d, хочу 0", m10.cursor)
	}
}

// L — перенос в следующую стадию (оптимистично), курсор на той же задаче;
// на последней стадии — ничего, без канбана — флеш.
func TestBoardMoveStage(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "a", Role: "do", GroupID: 1, StageID: 10, Stage: "INBOX", StageColor: "00c4fb", StageSort: 0},
		{ID: 2, Title: "b", Role: "do", GroupID: 1, StageID: 30, Stage: "DOING", StageColor: "75d900", StageSort: 2},
		{ID: 3, Title: "c", Role: "do", GroupID: 2, StageID: 99, Stage: "SIDE", StageColor: "101010", StageSort: 5},
	}
	m := boardModel(tasks)
	m2, cmd := keyPress(m, "L")
	if cmd == nil {
		t.Fatal("L не дал команду moveStage")
	}
	got := m2.tasks[0]
	if got.StageID != 20 || got.Stage != "TODO" || got.StageColor != "47d1e2" || got.StageSort != 1 {
		t.Fatalf("L не перенёс стадию: %+v", got)
	}
	if rows := m2.flatTasks(); m2.cursor >= len(rows) || rows[m2.cursor].task.ID != 1 {
		t.Fatalf("курсор ушёл с задачи: cursor=%d", m2.cursor)
	}
	if m2.flash != "→ TODO" {
		t.Fatalf("flash: %q", m2.flash)
	}
	// L на последней стадии — ничего
	m3 := m
	m3.cursor = 1
	m4, cmd2 := keyPress(m3, "L")
	if cmd2 != nil || m4.tasks[1].StageID != 30 {
		t.Fatalf("L на последней стадии: cmd=%v stage=%d", cmd2, m4.tasks[1].StageID)
	}
	// H — назад
	m5, cmd3 := keyPress(m3, "H")
	if cmd3 == nil || m5.tasks[1].StageID != 20 {
		t.Fatalf("H: cmd=%v stage=%d", cmd3, m5.tasks[1].StageID)
	}
	// у задачи нет канбана проекта — флеш
	m6 := m
	m6.cursor = 2
	m7, _ := keyPress(m6, "L")
	if m7.flash != "у задачи нет канбана проекта" {
		t.Fatalf("flash без стадий: %q", m7.flash)
	}
}

// stageMsg: ошибка — loadErr и перечитка, успех — перечитка.
func TestStageMsgHandling(t *testing.T) {
	m := boardModel(nil)
	mm, cmd := m.Update(stageMsg{err: errors.New("boom")})
	got := mm.(Model)
	if got.loadErr != "стадия не изменена: boom" || cmd == nil {
		t.Fatalf("ошибка стадии: loadErr=%q cmd=%v", got.loadErr, cmd)
	}
	mm2, cmd2 := m.Update(stageMsg{})
	if got2 := mm2.(Model); got2.loadErr != "" || cmd2 == nil {
		t.Fatalf("успех стадии: loadErr=%q cmd=%v", got2.loadErr, cmd2)
	}
}

// Рендер 120×36: колонки, карточки, выбранная карточка, габариты.
func TestBoardView(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "Разработать раздел Дашборд", Role: "do", GroupID: 1,
			Project: "DEV. Портал", StageID: 10, Stage: "INBOX", StageColor: "00c4fb", StageSort: 0},
		{ID: 2, Title: "Проверить отработку коллтрекинга", Role: "do", GroupID: 1,
			Project: "SRV. baden-apart", StageID: 20, Stage: "TODO", StageColor: "47d1e2", StageSort: 1,
			Deadline: "2026-10-19 18:00:00", NewComments: 2},
	}
	m := boardModel(tasks)
	v := m.View().Content
	s := ansi.Strip(v)
	for _, want := range []string{"INBOX", "TODO", "Разработать раздел Дашборд", "делаю 2", "все", "✎ 2", "сегодня 18:00"} {
		if !strings.Contains(s, want) {
			t.Fatalf("в доске нет %q:\n%s", want, s)
		}
	}
	if lipgloss.Height(v) > 36 {
		t.Fatalf("высота %d > 36", lipgloss.Height(v))
	}
	for i, line := range strings.Split(v, "\n") {
		if lw := lipgloss.Width(line); lw > 120 {
			t.Fatalf("строка %d шире 120: %d", i, lw)
		}
	}
	// пустая роль — заглушка «задач нет»
	m2, _ := keyPress(m, "t") // помогаю — задач нет
	if v := ansi.Strip(m2.View().Content); !strings.Contains(v, " — задач нет — ") {
		t.Fatalf("пустая роль без заглушки:\n%s", v)
	}
}

// Колонки тянутся на всю высоту доски: viewBoard ровно m.bodyH строк.
func TestBoardViewHeight(t *testing.T) {
	tasks := []Task{
		{ID: 1, Title: "a", Role: "do", Stage: "INBOX", StageSort: 0},
		{ID: 2, Title: "b", Role: "do", Stage: "TODO", StageSort: 1},
	}
	m := boardModel(tasks)
	v := m.viewBoard()
	if lipgloss.Height(v) != m.bodyH {
		t.Fatalf("высота доски %d, хочу ровно %d", lipgloss.Height(v), m.bodyH)
	}
	// каждая колонка — внешней высотой m.bodyH-2 (нижняя грань на последней строке)
	n := len(m.boardColumns())
	colW := (m.width - (n - 1)) / n
	colH := m.bodyH - 2
	for _, c := range m.boardColumns() {
		if h := lipgloss.Height(m.boardColView(c, -1, colW, colH)); h != colH {
			t.Fatalf("колонка %q: высота %d, хочу %d", c.title, h, colH)
		}
	}
	// пустая доска — той же высоты
	empty, _ := keyPress(m, "t") // помогаю — задач нет
	if h := lipgloss.Height(empty.viewBoard()); h != m.bodyH {
		t.Fatalf("пустая доска: высота %d, хочу %d", h, m.bodyH)
	}
}

// Окно колонок: не влезающие скрываются, счётчик справа, окно следует за курсором.
func TestBoardHiddenColumns(t *testing.T) {
	tasks := make([]Task, 0, 5)
	for i := 0; i < 5; i++ {
		tasks = append(tasks, Task{ID: int64(i + 1), Title: fmt.Sprintf("задача %d", i),
			Role: "do", Stage: fmt.Sprintf("S%d", i), StageSort: i})
	}
	m := boardModel(tasks)
	v := ansi.Strip(m.View().Content)
	// влезают 4 из 5: S0..S3 в кадре, справа счётчик «1 ›»
	if !strings.Contains(v, "1 ›") || !strings.Contains(v, "S0") {
		t.Fatalf("окно колонок: нет счётчика или S0:\n%s", v)
	}
	if strings.Contains(v, "S4") {
		t.Fatal("S4 не скрыта окном")
	}
	// доходим до последней колонки — окно сдвигается, слева скрытая
	for i := 0; i < 4; i++ {
		m, _ = keyPress(m, "l")
	}
	v = ansi.Strip(m.View().Content)
	if !strings.Contains(v, "‹ 1") || !strings.Contains(v, "S4") || strings.Contains(v, "S0") {
		t.Fatalf("окно не следует за курсором:\n%s", v)
	}
}

// Сегодняшний срок, прошедший по времени, — просрочка (не «сегодня»),
// и плашка помещается в карточку.
func TestBoardOverdueToday(t *testing.T) {
	tasks := []Task{{ID: 1, Title: "Обновить фид", Role: "do", GroupID: 1,
		StageID: 10, Stage: "INBOX", Deadline: "2026-10-19 10:00:00"}}
	m := boardModel(tasks) // сейчас 12:00
	s := ansi.Strip(m.View().Content)
	if !strings.Contains(s, "просрочено 10:00") || strings.Contains(s, "сегодня 10:00") {
		t.Fatalf("прошедший сегодняшний срок не просрочен:\n%s", s)
	}
}

func TestOneLineTitle(t *testing.T) {
	if got := oneLine("SRV. Сделать два\tфида\n "); got != "SRV. Сделать два фида" {
		t.Fatalf("%q", got)
	}
}
