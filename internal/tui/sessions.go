package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ppiankov/contextspectre/internal/session"
)

type sessionsModel struct {
	sessions      []session.Info
	cursor        int
	scrollOffset  int
	width, height int
	err           error
}

type openSessionMsg struct {
	info session.Info
}

func newSessionsModel(sessions []session.Info) sessionsModel {
	return sessionsModel{
		sessions: sessions,
	}
}

func (m sessionsModel) Init() tea.Cmd {
	return nil
}

func (m sessionsModel) Update(msg tea.Msg) (sessionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scrollOffset {
					m.scrollOffset = m.cursor
				}
			}
		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
				visible := m.visibleRows()
				if m.cursor >= m.scrollOffset+visible {
					m.scrollOffset = m.cursor - visible + 1
				}
			}
		case key.Matches(msg, keys.Enter):
			if m.cursor < len(m.sessions) {
				return m, func() tea.Msg {
					return openSessionMsg{info: m.sessions[m.cursor]}
				}
			}
		}
	}
	return m, nil
}

func (m sessionsModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}
	if len(m.sessions) == 0 {
		return "No sessions found.\n"
	}

	var b strings.Builder

	// Column widths
	projW := 30
	branchW := 14
	msgsW := 6
	sizeW := 8
	barW := 10
	pctW := 7
	modW := 10

	// Header
	header := fmt.Sprintf(" %-*s %-*s %*s %*s %-*s %*s %*s",
		projW, "Project",
		branchW, "Branch",
		msgsW, "Msgs",
		sizeW, "Size",
		barW, "Context",
		pctW, "",
		modW, "Modified",
	)
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n")
	b.WriteString(styleMuted.Render(" " + strings.Repeat("─", m.width-2)))
	b.WriteString("\n")

	// Rows
	visible := m.visibleRows()
	end := m.scrollOffset + visible
	if end > len(m.sessions) {
		end = len(m.sessions)
	}

	for i := m.scrollOffset; i < end; i++ {
		s := m.sessions[i]
		isSelected := i == m.cursor

		prefix := " "
		if isSelected {
			prefix = "▸"
		}

		active := ""
		if s.IsActive() {
			active = "[ACTIVE] "
		}

		project := truncateStr(active+s.ProjectName, projW)
		branch := truncateStr(s.GitBranch, branchW)
		if branch == "" {
			branch = "—"
		}

		size := fmt.Sprintf("%.1f MB", s.FileSizeMB)

		bar := "░░░░░░░░░░"
		pct := "—"
		compactLabel := ""
		if s.ContextStats != nil && s.ContextStats.ContextTokens > 0 {
			pctVal := s.ContextStats.ContextPct
			if s.ContextStats.CompactionCount > 0 {
				bar = contextBarStrCompacted(pctVal, barW)
				compactLabel = styleCompacted.Render(fmt.Sprintf(" %dx", s.ContextStats.CompactionCount))
			} else {
				bar = contextBarStr(pctVal, barW)
			}
			pct = fmt.Sprintf("%.1f%%", pctVal)
		}

		mod := timeAgoStr(s.Modified)

		line := fmt.Sprintf("%s%-*s %-*s %*d %*s %s %*s%s %*s",
			prefix,
			projW, project,
			branchW, branch,
			msgsW, s.MessageCount,
			sizeW, size,
			bar,
			pctW, pct,
			compactLabel,
			modW, mod,
		)

		if isSelected {
			b.WriteString(styleSelected.Render(line))
		} else if s.IsActive() {
			b.WriteString(styleActive.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(styleFooter.Render(" ↑↓ navigate  Enter open  q quit"))

	return b.String()
}

func (m sessionsModel) visibleRows() int {
	// Reserve lines for header (2), footer (2), title (2)
	avail := m.height - 6
	if avail < 3 {
		return 3
	}
	return avail
}

func contextBarStr(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	color := contextColor(pct)
	filledStr := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled))
	emptyStr := styleMuted.Render(strings.Repeat("░", width-filled))
	return filledStr + emptyStr
}

func contextBarStrCompacted(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	color := contextColorCompacted(pct)
	filledStr := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled))
	emptyStr := styleMuted.Render(strings.Repeat("░", width-filled))
	return filledStr + emptyStr
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func timeAgoStr(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
