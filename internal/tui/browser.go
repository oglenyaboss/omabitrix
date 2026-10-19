package tui

import (
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"omabitrix/internal/rules"
)

// startOpener — точка подмены в тестах: реальный xdg-open оттуда не зовётся.
var startOpener = func(url string) error {
	c := exec.Command("xdg-open", url)
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait() // xdg-open выходит сразу — не оставляем зомби
	return nil
}

// browserMsg — итог попытки открыть URL (err == nil — открыто).
type browserMsg struct{ err error }

// openURLCmd — запуск браузера без Wait: браузер не должен держать TUI.
func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		return browserMsg{err: startOpener(url)}
	}
}

// browserURL — что открыть в браузере: активный чат (или чат под курсором),
// открытая карточка задачи (или задача под курсором), задача из уведомления.
func (m Model) browserURL() string {
	if m.api.Portal == "" {
		return ""
	}
	switch m.tab {
	case tabChats:
		if m.active != nil {
			return m.api.Portal + "/online/?IM_DIALOG=" + m.active.DialogID
		}
		vc := m.visibleChats()
		if m.cursor < 0 || m.cursor >= len(vc) {
			return ""
		}
		return m.api.Portal + "/online/?IM_DIALOG=" + vc[m.cursor].DialogID
	case tabTasks:
		if m.taskOpen {
			return rules.TaskURL(m.api.Portal, int(m.api.My), int(m.taskCard.ID))
		}
		rows := m.flatTasks()
		if m.cursor < 0 || m.cursor >= len(rows) {
			return ""
		}
		return rules.TaskURL(m.api.Portal, int(m.api.My), int(rows[m.cursor].task.ID))
	default:
		vn := m.visibleNotify()
		if m.cursor < 0 || m.cursor >= len(vn) {
			return ""
		}
		if id := vn[m.cursor].TaskID; id > 0 {
			return rules.TaskURL(m.api.Portal, int(m.api.My), int(id))
		}
		return ""
	}
}

// openBrowser — «открыть в браузере»: нечего открывать — флеш, иначе запуск.
func (m Model) openBrowser() (tea.Model, tea.Cmd) {
	url := m.browserURL()
	if url == "" {
		m.flash = "нечего открыть в браузере"
		m.flashAt = m.api.now()
		return m, nil
	}
	return m, openURLCmd(url)
}
