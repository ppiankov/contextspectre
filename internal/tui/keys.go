package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up               key.Binding
	Down             key.Binding
	Enter            key.Binding
	Back             key.Binding
	Quit             key.Binding
	Space            key.Binding
	SelectAllProg    key.Binding
	ReplaceImages    key.Binding
	StripSeps        key.Binding
	SelectSnaps      key.Binding
	SelectStale      key.Binding
	TruncateOutput   key.Binding
	SelectChains     key.Binding
	SelectTangents   key.Binding
	CleanAll         key.Binding
	Epochs           key.Binding
	Delete           key.Binding
	Undo             key.Binding
	Search           key.Binding
	Escape           key.Binding
	Confirm          key.Binding
	DryRun           key.Binding
	Cancel           key.Binding
	MarkKeep         key.Binding
	MarkNoise        key.Binding
	CommitPoint      key.Binding
	PhaseExplore     key.Binding
	PhaseDecision    key.Binding
	PhaseOperational key.Binding
	PhaseClear       key.Binding
	Export           key.Binding
	ExportWipe       key.Binding
	Amputate         key.Binding
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
	StripSeps: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "strip separators"),
	),
	SelectSnaps: key.NewBinding(
		key.WithKeys("h"),
		key.WithHelp("h", "select snapshots"),
	),
	SelectStale: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "select stale reads"),
	),
	TruncateOutput: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "truncate outputs"),
	),
	SelectChains: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "select sidechains"),
	),
	SelectTangents: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "select tangents"),
	),
	CleanAll: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "clean all"),
	),
	Epochs: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "epoch timeline"),
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
	MarkKeep: key.NewBinding(
		key.WithKeys("K"),
		key.WithHelp("K", "toggle keep"),
	),
	MarkNoise: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "toggle noise"),
	),
	CommitPoint: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "commit point"),
	),
	PhaseExplore: key.NewBinding(
		key.WithKeys("1"),
		key.WithHelp("1", "exploratory"),
	),
	PhaseDecision: key.NewBinding(
		key.WithKeys("2"),
		key.WithHelp("2", "decision"),
	),
	PhaseOperational: key.NewBinding(
		key.WithKeys("3"),
		key.WithHelp("3", "operational"),
	),
	PhaseClear: key.NewBinding(
		key.WithKeys("0"),
		key.WithHelp("0", "clear phase"),
	),
	Export: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "export"),
	),
	ExportWipe: key.NewBinding(
		key.WithKeys("W"),
		key.WithHelp("W", "export+wipe"),
	),
	Amputate: key.NewBinding(
		key.WithKeys("!"),
		key.WithHelp("!", "amputate range"),
	),
}
