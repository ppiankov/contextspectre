package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ppiankov/contextspectre/internal/editor"
)

type commitPointModel struct {
	commitPoint   editor.CommitPoint
	cursorIdx     int
	width, height int
}

type showCommitPointMsg struct {
	cursorIdx   int
	commitPoint editor.CommitPoint
}

type confirmCommitPointMsg struct {
	cursorIdx   int
	commitPoint editor.CommitPoint
}

type cancelCommitPointMsg struct{}

func newCommitPointModel(cp editor.CommitPoint, cursorIdx int) commitPointModel {
	return commitPointModel{
		commitPoint: cp,
		cursorIdx:   cursorIdx,
	}
}

func (m commitPointModel) Init() tea.Cmd {
	return nil
}

func (m commitPointModel) Update(msg tea.Msg) (commitPointModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Confirm):
			return m, func() tea.Msg {
				return confirmCommitPointMsg{
					cursorIdx:   m.cursorIdx,
					commitPoint: m.commitPoint,
				}
			}
		case key.Matches(msg, keys.Cancel):
			return m, func() tea.Msg { return cancelCommitPointMsg{} }
		}
	}
	return m, nil
}

func (m commitPointModel) View() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Padding(1, 2).
		Width(60)

	var b strings.Builder
	b.WriteString(styleHeader.Render(fmt.Sprintf("Set Commit Point at entry #%d?", m.cursorIdx)))
	b.WriteString("\n\n")

	// Goal
	if m.commitPoint.Goal != "" {
		fmt.Fprintf(&b, "Goal: %s\n", m.commitPoint.Goal)
	}

	// Decisions
	if len(m.commitPoint.Decisions) > 0 {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Decisions (%d):\n", len(m.commitPoint.Decisions))
		for _, d := range m.commitPoint.Decisions {
			fmt.Fprintf(&b, "  - %s\n", truncateStr(d, 54))
		}
	}

	// Constraints
	if len(m.commitPoint.Constraints) > 0 {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Constraints (%d):\n", len(m.commitPoint.Constraints))
		for _, c := range m.commitPoint.Constraints {
			fmt.Fprintf(&b, "  - %s\n", truncateStr(c, 54))
		}
	}

	// Questions
	if len(m.commitPoint.Questions) > 0 {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Open Questions (%d):\n", len(m.commitPoint.Questions))
		for _, q := range m.commitPoint.Questions {
			fmt.Fprintf(&b, "  - %s\n", truncateStr(q, 54))
		}
	}

	// Files
	if len(m.commitPoint.Files) > 0 {
		b.WriteString("\n")
		fmt.Fprintf(&b, "Files Referenced: %d\n", len(m.commitPoint.Files))
	}

	b.WriteString("\n")
	fmt.Fprintf(&b, "Entries above: %d -> marked CANDIDATE\n", m.cursorIdx)
	b.WriteString("\n")
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
