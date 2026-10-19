package tui

// viewHome — пульт дня (вкладка 1).
func (m Model) viewHome() string {
	return box(paneTitle("", "пульт", "", true), []string{stDim.Render(" собирается…")}, m.width-4, true)
}
