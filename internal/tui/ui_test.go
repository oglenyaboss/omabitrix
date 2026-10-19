package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestBigText(t *testing.T) {
	l := bigText("16:55", true)
	if len(l) != 5 {
		t.Fatalf("строк %d, хочу 5", len(l))
	}
	// 4 цифры по 6 колонок + двоеточие 2 + 4 зазора по 2
	for i, s := range l {
		if w := lipgloss.Width(s); w != 4*6+2+4*2 {
			t.Errorf("строка %d ширина %d: %q", i, w, s)
		}
	}
	if !strings.Contains(l[1], "██") {
		t.Errorf("нет пикселей: %q", l[1])
	}
	off := bigText("16:55", false)
	if lipgloss.Width(off[1]) != lipgloss.Width(l[1]) || off[1] == l[1] {
		t.Errorf("погашенное двоеточие должно сохранить ширину и исчезнуть: %q / %q", off[1], l[1])
	}
	if got := bigClock("16:55", true, cAccent, cBlue); ansi.Strip(got[0]) != l[0] {
		t.Errorf("bigClock меняет глифы: %q", ansi.Strip(got[0]))
	}
}

func TestMeterWidth(t *testing.T) {
	for _, f := range []float64{-1, 0, 0.5, 1, 2} {
		s := meter(10, f, cAccent, cAmber)
		if w := lipgloss.Width(s); w != 10 {
			t.Errorf("frac %v: ширина %d", f, w)
		}
	}
	if got := strings.Count(ansi.Strip(meter(10, 0.5, cAccent, cAmber)), "▰"); got != 5 {
		t.Errorf("половина: %d заполненных", got)
	}
}

func TestUntilLabel(t *testing.T) {
	cases := map[time.Duration]string{
		65 * time.Minute: "через 1 ч 05 мин",
		2 * time.Hour:    "через 2 ч",
		25 * time.Minute: "через 25 мин",
		-2 * time.Hour:   "просрочено на 2 ч",
		50 * time.Hour:   "через 2 дн",
		10 * time.Second: "через меньше минуты",
	}
	for d, want := range cases {
		if got := untilLabel(d); got != want {
			t.Errorf("%v: %q, хочу %q", d, got, want)
		}
	}
}

func TestDueLabel(t *testing.T) {
	now := time.Date(2026, 10, 19, 16, 55, 0, 0, time.UTC) // пн
	cases := []struct {
		dl   time.Time
		want string
	}{
		{time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC), "сегодня 18:00"},
		{time.Date(2026, 10, 20, 18, 0, 0, 0, time.UTC), "завтра 18:00"},
		{time.Date(2026, 10, 21, 18, 0, 0, 0, time.UTC), "ср 21.10"},
		{time.Date(2026, 10, 18, 18, 0, 0, 0, time.UTC), "вчера"},
		{time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC), "30.10"},
	}
	for _, c := range cases {
		if got := dueLabel(c.dl, now); got != c.want {
			t.Errorf("%v: %q, хочу %q", c.dl, got, c.want)
		}
	}
}

func TestAvatar(t *testing.T) {
	if got := ansi.Strip(avatar("орлова Вера", false, 1)); got != " К " {
		t.Errorf("личный: %q", got)
	}
	if got := ansi.Strip(avatar("Команда", true, 1)); got != " # " {
		t.Errorf("групповой: %q", got)
	}
	if got := ansi.Strip(avatar("🏠 КЛЕВЕР", false, 1)); got != " К " {
		t.Errorf("эмодзи в начале: %q", got)
	}
	if seedOf("chat1") != seedOf("chat1") {
		t.Error("seed нестабилен")
	}
}

func TestPulseCycle(t *testing.T) {
	if pulseColor(0) != pulseColor(16) || pulseColor(3) != pulseColor(13) {
		t.Error("пульс не периодичен / не симметричен")
	}
	if pulseColor(0) == pulseColor(8) {
		t.Error("пульс не меняет цвет")
	}
}
