package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/session"
)

type openEpochsMsg struct {
	epochs []analyzer.Epoch
	info   session.Info
	drift  *analyzer.ScopeDrift
}

type backFromEpochsMsg struct{}

type epochsModel struct {
	epochs       []analyzer.Epoch
	session      session.Info
	driftResult  *analyzer.ScopeDrift
	cursor       int
	scrollOffset int
	detailOpen   bool
	width        int
	height       int
}

func newEpochsModel(epochs []analyzer.Epoch, info session.Info) epochsModel {
	return epochsModel{
		epochs:  epochs,
		session: info,
	}
}

func (m epochsModel) Init() tea.Cmd {
	return nil
}

func (m epochsModel) Update(msg tea.Msg) (epochsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m epochsModel) handleKey(msg tea.KeyMsg) (epochsModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.detailOpen = false
			if m.cursor < m.scrollOffset {
				m.scrollOffset = m.cursor
			}
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.epochs)-1 {
			m.cursor++
			m.detailOpen = false
			visible := m.visibleRows()
			if m.cursor >= m.scrollOffset+visible {
				m.scrollOffset = m.cursor - visible + 1
			}
		}
	case key.Matches(msg, keys.Enter):
		if m.cursor < len(m.epochs) && m.epochs[m.cursor].Archaeology != nil {
			m.detailOpen = !m.detailOpen
		}
	case key.Matches(msg, keys.Back):
		return m, func() tea.Msg { return backFromEpochsMsg{} }
	}
	return m, nil
}

func (m epochsModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Title
	totalCost := 0.0
	for _, ep := range m.epochs {
		totalCost += ep.Cost
	}
	b.WriteString(styleTitle.Render(fmt.Sprintf(" Epoch Timeline | %s/%s | %d epochs | %s",
		m.session.ProjectName, shortUUID(m.session.SessionID),
		len(m.epochs), analyzer.FormatCost(totalCost))))
	b.WriteString("\n\n")

	// Column header
	header := fmt.Sprintf("   %-7s %6s %10s %9s %7s  %-28s %10s",
		"Epoch", "Turns", "Peak", "Cost", "Drift", "Topic", "Survived")
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n")
	sepWidth := m.width - 2
	if sepWidth < 1 {
		sepWidth = 1
	}
	b.WriteString(styleMuted.Render(" " + strings.Repeat("─", sepWidth)))
	b.WriteString("\n")

	// Find costliest epoch
	costliestIdx := 0
	for i, ep := range m.epochs {
		if ep.Cost > m.epochs[costliestIdx].Cost {
			costliestIdx = i
		}
	}

	// Epoch rows
	visible := m.visibleRows()
	end := m.scrollOffset + visible
	if end > len(m.epochs) {
		end = len(m.epochs)
	}

	for i := m.scrollOffset; i < end; i++ {
		ep := m.epochs[i]
		isSelected := i == m.cursor

		prefix := "   "
		if isSelected {
			prefix = " ▸ "
		} else if i == costliestIdx && !ep.IsActive {
			prefix = " * "
		}

		survived := fmt.Sprintf("%d chars", ep.SurvivedChars)
		if ep.IsActive {
			survived = "(active)"
		}

		driftStr := "    —"
		if m.driftResult != nil && ep.Index < len(m.driftResult.EpochScopes) {
			es := m.driftResult.EpochScopes[ep.Index]
			if es.InScope+es.OutScope > 0 {
				driftStr = fmt.Sprintf("%5.0f%%", es.DriftRatio*100)
			}
		}

		line := fmt.Sprintf("%s#%-6d %6d %10s %9s %7s  %-28s %10s",
			prefix,
			ep.Index,
			ep.TurnCount,
			formatTokensShort(ep.PeakTokens),
			analyzer.FormatCost(ep.Cost),
			driftStr,
			truncateStr(ep.Topic, 28),
			survived)

		if isSelected {
			b.WriteString(styleSelected.Render(line))
		} else if i == costliestIdx && !ep.IsActive {
			b.WriteString(styleWarning.Render(line))
		} else if ep.IsActive {
			b.WriteString(styleCompacted.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")

		// Inline detail panel
		if isSelected && m.detailOpen && ep.Archaeology != nil {
			b.WriteString(m.renderDetail(ep))
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(styleFooter.Render(" ↑↓ navigate  Enter detail  q back"))

	return b.String()
}

func (m epochsModel) renderDetail(ep analyzer.Epoch) string {
	arch := ep.Archaeology
	if arch == nil {
		return ""
	}

	var b strings.Builder
	indent := "     "
	detailWidth := m.width - 10
	if detailWidth < 20 {
		detailWidth = 20
	}

	// Stats line
	b.WriteString(styleMuted.Render(fmt.Sprintf(
		"%sFiles: %d  |  Tools: %d  |  Compression: %.1fx\n",
		indent,
		len(arch.Before.FilesReferenced),
		arch.Before.TotalToolCalls(),
		arch.After.CompressionRatio)))

	// Top tool calls
	if arch.Before.TotalToolCalls() > 0 {
		b.WriteString(styleMuted.Render(indent + "Tools:"))
		count := 0
		for name, c := range arch.Before.ToolCallCounts {
			if count >= 5 {
				break
			}
			b.WriteString(styleMuted.Render(fmt.Sprintf(" %s(%d)", name, c)))
			count++
		}
		b.WriteString("\n")
	}

	// User questions (up to 3)
	if len(arch.Before.UserQuestions) > 0 {
		max := 3
		if len(arch.Before.UserQuestions) < max {
			max = len(arch.Before.UserQuestions)
		}
		b.WriteString(styleMuted.Render(fmt.Sprintf(
			"%sQuestions (%d):\n", indent, len(arch.Before.UserQuestions))))
		for j := 0; j < max; j++ {
			b.WriteString(styleMuted.Render(fmt.Sprintf(
				"%s  - %s\n", indent, truncateStr(arch.Before.UserQuestions[j], detailWidth))))
		}
		if len(arch.Before.UserQuestions) > 3 {
			b.WriteString(styleMuted.Render(fmt.Sprintf(
				"%s  ... and %d more\n", indent, len(arch.Before.UserQuestions)-3)))
		}
	}

	// Decision hints (up to 3)
	if len(arch.Before.DecisionHints) > 0 {
		max := 3
		if len(arch.Before.DecisionHints) < max {
			max = len(arch.Before.DecisionHints)
		}
		b.WriteString(styleMuted.Render(fmt.Sprintf(
			"%sDecisions (%d):\n", indent, len(arch.Before.DecisionHints))))
		for j := 0; j < max; j++ {
			b.WriteString(styleMuted.Render(fmt.Sprintf(
				"%s  - %s\n", indent, truncateStr(arch.Before.DecisionHints[j], detailWidth))))
		}
	}

	// Summary preview
	if arch.After.SummaryText != "" {
		preview := truncateStr(arch.After.SummaryText, detailWidth)
		b.WriteString(styleMuted.Render(fmt.Sprintf(
			"%sSummary: \"%s\"\n", indent, preview)))
	}

	sepWidth := m.width - 10
	if sepWidth < 1 {
		sepWidth = 1
	}
	b.WriteString(styleMuted.Render(indent + strings.Repeat("─", sepWidth)))
	b.WriteString("\n")

	return b.String()
}

func (m epochsModel) visibleRows() int {
	// title(2) + header(1) + separator(1) + blank(1) + footer(1) = 6
	reserved := 6
	if m.detailOpen {
		reserved += 8
	}
	avail := m.height - reserved
	if avail < 3 {
		return 3
	}
	return avail
}
