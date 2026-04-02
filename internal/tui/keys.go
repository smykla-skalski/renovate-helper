package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Esc        key.Binding
	Merge      key.Binding
	MergeAll   key.Binding
	Approve    key.Binding
	ApproveAll key.Binding
	Label      key.Binding
	Rerun      key.Binding
	Refresh    key.Binding
	Filter     key.Binding
	Sort       key.Binding
	Open       key.Binding
	Select     key.Binding
	FixCI      key.Binding
	CopyLinks  key.Binding
	Help       key.Binding
	Quit       key.Binding
}

var keys = keyMap{
	Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
	Esc:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Merge:      key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "merge")),
	MergeAll:   key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "merge all selected")),
	Approve:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approve")),
	ApproveAll: key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "approve all selected")),
	Label:      key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "add label")),
	Rerun:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rerun checks")),
	Refresh:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "force refresh")),
	Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Sort:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
	Open:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in browser")),
	Select:     key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "select")),
	FixCI:      key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fix CI")),
	CopyLinks:  key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy review links")),
	Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
