package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up            key.Binding
	Down          key.Binding
	Enter         key.Binding
	Back          key.Binding
	Quit          key.Binding
	Space         key.Binding
	SelectAllProg key.Binding
	ReplaceImages key.Binding
	Delete        key.Binding
	Undo          key.Binding
	Search        key.Binding
	Escape        key.Binding
	Confirm       key.Binding
	DryRun        key.Binding
	Cancel        key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("Enter", "open/preview"),
	),
	Back: key.NewBinding(
		key.WithKeys("q", "esc"),
		key.WithHelp("q", "back/quit"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Space: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("Space", "select"),
	),
	SelectAllProg: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "select all progress"),
	),
	ReplaceImages: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "replace images"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete selected"),
	),
	Undo: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "undo"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("Esc", "cancel"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "confirm"),
	),
	DryRun: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "cancel"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("n", "esc"),
		key.WithHelp("n", "cancel"),
	),
}
