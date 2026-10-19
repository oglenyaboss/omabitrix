package tui

import "charm.land/bubbles/v2/key"

// keyMap — все хоткеи TUI одной картой: биндинги и для сверки ввода
// (key.Matches), и для подсказок футера и «?».
type keyMap struct {
	Up        key.Binding // k, up
	Down      key.Binding // j, down
	Top       key.Binding // g
	Bottom    key.Binding // G
	Open      key.Binding // enter
	Back      key.Binding // esc
	Quit      key.Binding // q
	ForceQuit key.Binding // ctrl+c
	Tab1      key.Binding // 1
	Tab2      key.Binding // 2
	Tab3      key.Binding // 3
	NextTab   key.Binding // tab
	PrevTab   key.Binding // shift+tab
	Help      key.Binding // ?
	Refresh   key.Binding // R
	Filter    key.Binding // /
	Reply     key.Binding // r
	Send      key.Binding // enter в ответе
	Cancel    key.Binding // esc в ответе
	Older     key.Binding // H
	PageUp    key.Binding // pgup, b
	PageDown  key.Binding // pgdown, f
	Status    key.Binding // s
	Deadline  key.Binding // d
	TaskChat  key.Binding // c
	ReadAll   key.Binding // A
	Browser   key.Binding // o
}

var km = keyMap{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("↑/k", "вверх"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("↓/j", "вниз"),
	),
	Top: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "в начало"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "в конец"),
	),
	Open: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "открыть"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "назад"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "назад / выход"),
	),
	ForceQuit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "выход"),
	),
	Tab1: key.NewBinding(
		key.WithKeys("1"),
		key.WithHelp("1", "чаты"),
	),
	Tab2: key.NewBinding(
		key.WithKeys("2"),
		key.WithHelp("2", "задачи"),
	),
	Tab3: key.NewBinding(
		key.WithKeys("3"),
		key.WithHelp("3", "уведы"),
	),
	NextTab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "следующая"),
	),
	PrevTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "предыдущая"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "хоткеи"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "обновить"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "фильтр"),
	),
	Reply: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "ответить"),
	),
	Send: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "отправить"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "отмена"),
	),
	Older: key.NewBinding(
		key.WithKeys("H"),
		key.WithHelp("H", "раньше"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "b"),
		key.WithHelp("b", "стр. вверх"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "f"),
		key.WithHelp("f", "стр. вниз"),
	),
	Status: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "статус"),
	),
	Deadline: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "срок"),
	),
	TaskChat: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "чат задачи"),
	),
	Browser: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "в браузере"),
	),
	ReadAll: key.NewBinding(
		key.WithKeys("A"),
		key.WithHelp("A", "всё прочитано"),
	),
}

// shortHelp — контекстный набор для футера, «?» — в конце каждого набора.
func (m Model) shortHelp() []key.Binding {
	switch {
	case m.focus == focusReply:
		return []key.Binding{km.Send, km.Cancel, km.Help}
	case m.active != nil:
		return []key.Binding{km.Reply, km.Older, km.PageUp, km.PageDown, km.Back, km.Help}
	case m.tab == tabTasks && m.taskOpen:
		b := []key.Binding{km.Status, km.Deadline}
		if m.taskCard.ChatID > 0 {
			b = append(b, km.TaskChat)
		}
		return append(b, km.Browser, km.Quit, km.Help)
	case m.tab == tabTasks:
		return []key.Binding{km.Open, km.Refresh, km.Help}
	case m.tab == tabNotify:
		return []key.Binding{km.Open, km.ReadAll, km.Refresh, km.Help}
	default:
		return []key.Binding{km.Open, km.Filter, km.Help}
	}
}

// fullHelp — полная карта для «?»: группы-колонки.
func (m Model) fullHelp() [][]key.Binding {
	return [][]key.Binding{
		// навигация
		{km.Up, km.Down, km.Top, km.Bottom, km.Open, km.PageUp, km.PageDown},
		// вкладки
		{km.Tab1, km.Tab2, km.Tab3, km.NextTab, km.PrevTab},
		// чаты
		{km.Reply, km.Send, km.Cancel, km.Older},
		// задачи
		{km.Status, km.Deadline, km.TaskChat},
		// уведы
		{km.ReadAll},
		// общее
		{km.Filter, km.Refresh, km.Browser, km.Back, km.Quit, km.ForceQuit, km.Help},
	}
}
