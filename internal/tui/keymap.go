package tui

import "charm.land/bubbles/v2/key"

// keyMap — все хоткеи TUI одной картой: биндинги и для сверки ввода
// (key.Matches), и для подсказок футера и «?».
type keyMap struct {
	Up         key.Binding // k, up
	Down       key.Binding // j, down
	Top        key.Binding // g
	Bottom     key.Binding // G
	Open       key.Binding // enter
	NextDue    key.Binding // n на пульте — ближайший срок
	Back       key.Binding // esc
	Quit       key.Binding // q
	ForceQuit  key.Binding // ctrl+c
	Tab1       key.Binding // 1
	Tab2       key.Binding // 2
	Tab3       key.Binding // 3
	Tab4       key.Binding // 4
	NextTab    key.Binding // tab
	PrevTab    key.Binding // shift+tab
	Help       key.Binding // ?
	Refresh    key.Binding // R
	Filter     key.Binding // /
	Search     key.Binding // / поиск по истории диалога
	Reply      key.Binding // r
	ReplyField key.Binding // enter на карточке экрана задачи — в поле ответа
	Select     key.Binding // v
	ReplyQuote key.Binding // r на выделенном
	Edit       key.Binding // e на выделенном
	Delete     key.Binding // D на выделенном
	Like       key.Binding // l на выделенном
	Mention    key.Binding // @ на выделенном
	Send       key.Binding // enter в ответе
	Cancel     key.Binding // esc в ответе
	Older      key.Binding // H
	PageUp     key.Binding // pgup, b
	PageDown   key.Binding // pgdown, f
	Status     key.Binding // s — стадии/статус на карточке
	Deadline   key.Binding // d
	TaskChat   key.Binding // c — чат задачи во весь экран
	FocusChat  key.Binding // i/tab — фокус в чат на экране задачи
	ColLeft    key.Binding // h, left — колонка влево
	ColRight   key.Binding // l, right — колонка вправо
	StageMove  key.Binding // H/L — стадию назад/вперёд
	Role       key.Binding // t — роль доски
	Project    key.Binding // p — проект доски
	ReadAll    key.Binding // A
	Browser    key.Binding // o
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
	NextDue: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "ближайший срок"),
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
		key.WithHelp("1", "пульт"),
	),
	Tab2: key.NewBinding(
		key.WithKeys("2"),
		key.WithHelp("2", "задачи"),
	),
	Tab3: key.NewBinding(
		key.WithKeys("3"),
		key.WithHelp("3", "чаты"),
	),
	Tab4: key.NewBinding(
		key.WithKeys("4"),
		key.WithHelp("4", "уведы"),
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
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "поиск"),
	),
	Reply: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "написать"),
	),
	ReplyField: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter/r", "написать"),
	),
	Select: key.NewBinding(
		key.WithKeys("v"),
		key.WithHelp("v", "выделить"),
	),
	ReplyQuote: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "ответить"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "изменить"),
	),
	Delete: key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "удалить"),
	),
	Like: key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "лайк"),
	),
	Mention: key.NewBinding(
		key.WithKeys("@"),
		key.WithHelp("@", "упомянуть"),
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
		key.WithHelp("b", "стр.↑"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "f"),
		key.WithHelp("f", "стр.↓"),
	),
	Status: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "стадии"),
	),
	Deadline: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "срок"),
	),
	TaskChat: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "чат во весь экран"),
	),
	FocusChat: key.NewBinding(
		key.WithKeys("i", "tab"),
		key.WithHelp("i/tab", "в чат"),
	),
	ColLeft: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("h/l", "колонка"),
	),
	ColRight: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("l", "колонка"),
	),
	StageMove: key.NewBinding(
		key.WithKeys("H", "L"),
		key.WithHelp("H/L", "стадия"),
	),
	Role: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "роль"),
	),
	Project: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "проект"),
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
	case m.searchOn:
		return []key.Binding{km.Up, km.Down, km.Back, km.Help}
	case m.tab == tabTasks && m.taskOpen:
		// чат задачи открыт справа — подсказки экрана задачи важнее
		return []key.Binding{km.StageMove, km.Status, km.Deadline, km.ReplyField, km.TaskChat, km.Browser, km.Quit, km.Help}
	case m.sel >= 0:
		return []key.Binding{km.ReplyQuote, km.Edit, km.Delete, km.Like, km.Mention, km.Cancel, km.Help}
	case m.active != nil:
		return []key.Binding{km.Reply, km.Select, km.Older, km.Search, km.PageUp, km.PageDown, km.Back, km.Help}
	case m.tab == tabTasks:
		return []key.Binding{km.ColLeft, km.Down, km.StageMove, km.Open, km.Role, km.Project, km.Filter, km.Help}
	case m.tab == tabNotify:
		return []key.Binding{km.Open, km.ReadAll, km.Refresh, km.Help}
	case m.tab == tabHome:
		return []key.Binding{km.NextDue, km.Tab2, km.Tab3, km.Tab4, km.Refresh, km.Help}
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
		{km.Tab1, km.Tab2, km.Tab3, km.Tab4, km.NextTab, km.PrevTab},
		// чаты (r — ответить, на выделенном — с цитатой)
		{km.Select, km.ReplyQuote, km.Edit, km.Delete, km.Like, km.Mention, km.Send, km.Cancel, km.Older, km.Search},
		// задачи
		{km.NextDue, km.StageMove, km.Status, km.Deadline, km.TaskChat, km.Reply, km.ColLeft, km.ColRight, km.Role, km.Project},
		// уведы
		{km.ReadAll},
		// общее
		{km.Filter, km.Refresh, km.Browser, km.Back, km.Quit, km.ForceQuit, km.Help},
	}
}
