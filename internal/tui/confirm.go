package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ppiankov/contextspectre/internal/analyzer"
)

type confirmModel struct {
	selected      map[int]bool
	impact        *analyzer.DeletionImpact
	width, height int
}

type confirmDeleteMsg struct {
	selected map[int]bool
}

type cancelDeleteMsg struct{}

func newConfirmModel(selected map[int]bool, impact *analyzer.DeletionImpact) confirmModel {
	return confirmModel{
		selected: selected,
		impact:   impact,
	}
}

func (m confirmModel) Init() tea.Cmd {
	return nil
}

func (m confirmModel) Update(msg tea.Msg) (confirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Confirm):
			return m, func() tea.Msg { return confirmDeleteMsg{selected: m.selected} }
		case key.Matches(msg, keys.Cancel):
			return m, func() tea.Msg { return cancelDeleteMsg{} }
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	if m.impact == nil {
		return ""
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(52)

	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("Delete %d messages?", m.impact.SelectedCount)))
	b.WriteString("\n\n")

	fmt.Fprintf(&b, "Estimated token savings: ~%s (%.1f%%)\n",
		formatTokensShort(m.impact.EstimatedTokenSaved),
		float64(m.impact.EstimatedTokenSaved)/float64(analyzer.ContextWindowSize)*100)

	fmt.Fprintf(&b, "New context usage: %.1f%%\n", m.impact.NewContextPercent)
	fmt.Fprintf(&b, "ParentUuid repairs: %d chains\n", m.impact.ChainRepairs)

	if m.impact.ProgressAutoRemoved > 0 {
		fmt.Fprintf(&b, "Progress auto-removed: %d\n", m.impact.ProgressAutoRemoved)
	}

	if m.impact.PredictedTurnsGained > 0 {
		fmt.Fprintf(&b, "Predicted turns gained: +%d\n", m.impact.PredictedTurnsGained)
	}

	for _, w := range m.impact.Warnings {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorYellow).Render("⚠ " + w))
	}

	b.WriteString("\n\n")
	b.WriteString("Backup will be created: session.jsonl.bak\n\n")
	b.WriteString(styleHeader.Render("[y] Confirm    [n] Cancel"))

	box := boxStyle.Render(b.String())

	// Center the box
	padTop := (m.height - lipgloss.Height(box)) / 2
	if padTop < 0 {
		padTop = 0
	}
	padLeft := (m.width - lipgloss.Width(box)) / 2
	if padLeft < 0 {
		padLeft = 0
	}

	return strings.Repeat("\n", padTop) +
		strings.Repeat(" ", padLeft) + box
}
