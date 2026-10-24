package tui

import "testing"

func TestSplashRenders(t *testing.T) {
	s := Splash("x")
	lines := splitLines(s)
	if len(lines) < 8 {
		t.Fatalf("мало строк: %d", len(lines))
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
