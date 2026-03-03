package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/session"
)

type drillIntoBranchMsg struct {
	info     session.Info
	startIdx int
}

type backFromBranchesMsg struct{}

type branchesModel struct {
	branches     []analyzer.Branch
	session      session.Info
	cursor       int
	scrollOffset int
	width        int
	height       int
}

func newBranchesModel(branches []analyzer.Branch, info session.Info) branchesModel {
	return branchesModel{
		branches: branches,
		session:  info,
	}
}

func (m branchesModel) Init() tea.Cmd {
	return nil
}

func (m branchesModel) Update(msg tea.Msg) (branchesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m branchesModel) handleKey(msg tea.KeyMsg) (branchesModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.scrollOffset {
				m.scrollOffset = m.cursor
			}
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.branches)-1 {
			m.cursor++
			visible := m.visibleRows()
			if m.cursor >= m.scrollOffset+visible {
				m.scrollOffset = m.cursor - visible + 1
			}
		}
	case key.Matches(msg, keys.Enter):
		if m.cursor < len(m.branches) {
			b := m.branches[m.cursor]
			return m, func() tea.Msg {
				return drillIntoBranchMsg{
					info:     m.session,
					startIdx: b.StartIdx,
				}
			}
		}
	case key.Matches(msg, keys.Back):
		return m, func() tea.Msg { return backFromBranchesMsg{} }
	}
	return m, nil
}

func (m branchesModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Title
	totalTokens := 0
	for _, br := range m.branches {
		totalTokens += br.TokenCost
	}
	b.WriteString(styleTitle.Render(fmt.Sprintf(" Branches | %s/%s | %d branches | ~%s tokens",
		m.session.ProjectName, shortUUID(m.session.SessionID),
		len(m.branches), formatTokensShort(totalTokens))))
	b.WriteString("\n\n")

	// Column header
	header := fmt.Sprintf("   %-8s %7s %6s %8s  %-15s  %-28s %5s",
		"Branch", "Entries", "Turns", "Tokens", "Time Range", "Summary", "Files")
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n")
	sepWidth := m.width - 2
	if sepWidth < 1 {
		sepWidth = 1
	}
	b.WriteString(styleMuted.Render(" " + strings.Repeat("─", sepWidth)))
	b.WriteString("\n")

	// Branch rows
	visible := m.visibleRows()
	end := m.scrollOffset + visible
	if end > len(m.branches) {
		end = len(m.branches)
	}

	for i := m.scrollOffset; i < end; i++ {
		br := m.branches[i]
		isSelected := i == m.cursor

		prefix := "   "
		if isSelected {
			prefix = " ▸ "
		}

		// Branch label: index with 'c' suffix for compaction-triggered
		label := fmt.Sprintf("#%d", br.Index)
		if br.HasCompaction {
			label += "c"
		}

		// Time range
		timeRange := "—"
		if !br.TimeStart.IsZero() {
			start := br.TimeStart.Format("15:04")
			end := br.TimeEnd.Format("15:04")
			if br.IsLast {
				end = "now"
			}
			timeRange = fmt.Sprintf("%s-%s", start, end)
		}

		line := fmt.Sprintf("%s%-8s %7d %6d %8s  %-15s  %-28s %5d",
			prefix,
			label,
			br.EntryCount,
			br.UserTurns,
			formatTokensShort(br.TokenCost),
			timeRange,
			truncateStr(br.Summary, 28),
			br.FileCount)

		if isSelected {
			b.WriteString(styleSelected.Render(line))
		} else if br.IsLast {
			b.WriteString(styleCompacted.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(styleFooter.Render(" ↑↓ navigate  Enter drill-in  q back"))

	return b.String()
}

func (m branchesModel) visibleRows() int {
	// title(2) + header(1) + separator(1) + blank(1) + footer(1) = 6
	reserved := 6
	avail := m.height - reserved
	if avail < 3 {
		return 3
	}
	return avail
}
