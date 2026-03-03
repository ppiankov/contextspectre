package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
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
	entries      []jsonl.Entry
	cursor       int
	scrollOffset int
	selected     map[int]bool
	statusMsg    string
	width        int
	height       int
}

func newBranchesModel(branches []analyzer.Branch, info session.Info) branchesModel {
	return branchesModel{
		branches: branches,
		session:  info,
		selected: make(map[int]bool),
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
	case key.Matches(msg, keys.Space):
		if m.cursor < len(m.branches) {
			if m.selected[m.cursor] {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = true
			}
		}
	case key.Matches(msg, keys.Export):
		return m.exportBranches(false)
	case key.Matches(msg, keys.ExportWipe):
		return m.exportBranches(true)
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

func (m branchesModel) exportBranches(wipe bool) (branchesModel, tea.Cmd) {
	// Parse entries if not loaded
	entries := m.entries
	if len(entries) == 0 {
		var err error
		entries, err = jsonl.Parse(m.session.FullPath)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", err)
			return m, nil
		}
		m.entries = entries
	}

	// Build selected indices (all if none selected)
	var indices []int
	if len(m.selected) > 0 {
		for idx := range m.selected {
			indices = append(indices, idx)
		}
	}

	// Output path
	sessionID := strings.TrimSuffix(filepath.Base(m.session.FullPath), ".jsonl")
	outputPath := fmt.Sprintf("branch-export-%s.md", time.Now().Format("2006-01-02-150405"))

	result, err := editor.ExportBranches(entries, m.branches, indices, sessionID, outputPath)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Export error: %v", err)
		return m, nil
	}

	m.statusMsg = fmt.Sprintf("Exported %d branches (~%s tokens) to %s",
		result.BranchesExported, formatTokensShort(result.TokenCost), result.OutputPath)

	if wipe {
		// Build toDelete from selected branch entry ranges
		toDelete := make(map[int]bool)
		selected := indices
		if len(selected) == 0 {
			for i := range m.branches {
				selected = append(selected, i)
			}
		}
		for _, idx := range selected {
			br := m.branches[idx]
			for i := br.StartIdx; i <= br.EndIdx; i++ {
				toDelete[i] = true
			}
		}
		wipeResult, err := editor.Delete(m.session.FullPath, toDelete)
		if err != nil {
			m.statusMsg += fmt.Sprintf(" | Wipe error: %v", err)
		} else {
			m.statusMsg += fmt.Sprintf(" | Wiped %d entries, %d chain repairs",
				wipeResult.EntriesRemoved, wipeResult.ChainRepairs)
		}
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
	header := fmt.Sprintf("      %-8s %7s %6s %8s  %-15s  %-28s %5s",
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
		isMarked := m.selected[i]

		// Checkbox
		check := "[ ] "
		if isMarked {
			check = styleMarked.Render("[x] ")
		}

		prefix := " "
		if isSelected {
			prefix = "▸"
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

		line := fmt.Sprintf("%s%s%-8s %7d %6d %8s  %-15s  %-28s %5d",
			prefix,
			check,
			label,
			br.EntryCount,
			br.UserTurns,
			formatTokensShort(br.TokenCost),
			timeRange,
			truncateStr(br.Summary, 28),
			br.FileCount)

		if isSelected && isMarked {
			b.WriteString(lipgloss.NewStyle().Background(colorAccent).Foreground(colorWhite).Render(line))
		} else if isSelected {
			b.WriteString(styleSelected.Render(line))
		} else if br.IsLast {
			b.WriteString(styleCompacted.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// Selection summary
	if len(m.selected) > 0 {
		selTokens := 0
		for idx := range m.selected {
			if idx < len(m.branches) {
				selTokens += m.branches[idx].TokenCost
			}
		}
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render(
			fmt.Sprintf(" Selected: %d branches (~%s tokens)", len(m.selected), formatTokensShort(selTokens))))
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(styleFooter.Render(" Space select  e export  W export+wipe  Enter drill  q back"))

	if m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(styleMuted.Render(" " + m.statusMsg))
	}

	return b.String()
}

func (m branchesModel) visibleRows() int {
	// title(2) + header(1) + separator(1) + selection(2) + footer(1) + status(1) = 8
	reserved := 8
	avail := m.height - reserved
	if avail < 3 {
		return 3
	}
	return avail
}
