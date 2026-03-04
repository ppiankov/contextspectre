package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/session"
)

// displayRow represents a row in the session list — either a project group header or a session.
type displayRow struct {
	isHeader     bool
	projectName  string
	sessionCount int
	sessionIdx   int // index into activeSessions(), -1 for headers
}

type sessionsModel struct {
	sessions      []session.Info
	displayRows   []displayRow
	cursor        int
	scrollOffset  int
	searching     bool
	searchQuery   string
	filtered      []session.Info
	width, height int
	err           error
}

type openSessionMsg struct {
	info session.Info
}

func newSessionsModel(sessions []session.Info) sessionsModel {
	m := sessionsModel{
		sessions: sessions,
	}
	m.buildDisplayRows(sessions)
	m.cursor = m.nextSelectableRow(0)
	return m
}

func (m sessionsModel) Init() tea.Cmd {
	return nil
}

func (m sessionsModel) Update(msg tea.Msg) (sessionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.searching {
			return m.handleSearchKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m sessionsModel) handleKey(msg tea.KeyMsg) (sessionsModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		prev := m.prevSelectableRow(m.cursor)
		if prev >= 0 {
			m.cursor = prev
			if m.cursor < m.scrollOffset {
				m.scrollOffset = m.cursor
				// Show group header if it's immediately above
				if m.scrollOffset > 0 && m.displayRows[m.scrollOffset-1].isHeader {
					m.scrollOffset--
				}
			}
		}
	case key.Matches(msg, keys.Down):
		next := m.nextSelectableRowAfter(m.cursor)
		if next >= 0 {
			m.cursor = next
			visible := m.visibleRows()
			if m.cursor >= m.scrollOffset+visible {
				m.scrollOffset = m.cursor - visible + 1
			}
		}
	case key.Matches(msg, keys.Enter):
		if m.cursor >= 0 && m.cursor < len(m.displayRows) && !m.displayRows[m.cursor].isHeader {
			src := m.activeSessions()
			idx := m.displayRows[m.cursor].sessionIdx
			if idx >= 0 && idx < len(src) {
				return m, func() tea.Msg {
					return openSessionMsg{info: src[idx]}
				}
			}
		}
	case key.Matches(msg, keys.Search):
		m.searching = true
		m.searchQuery = ""
		m.filterSessions()
	}
	return m, nil
}

func (m sessionsModel) handleSearchKey(msg tea.KeyMsg) (sessionsModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		m.searching = false
		m.searchQuery = ""
		m.filtered = nil
		m.buildDisplayRows(m.sessions)
		m.cursor = m.nextSelectableRow(0)
		m.scrollOffset = 0
	case msg.Type == tea.KeyBackspace:
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.filterSessions()
		}
	case key.Matches(msg, keys.Up):
		prev := m.prevSelectableRow(m.cursor)
		if prev >= 0 {
			m.cursor = prev
			if m.cursor < m.scrollOffset {
				m.scrollOffset = m.cursor
				if m.scrollOffset > 0 && m.displayRows[m.scrollOffset-1].isHeader {
					m.scrollOffset--
				}
			}
		}
	case key.Matches(msg, keys.Down):
		next := m.nextSelectableRowAfter(m.cursor)
		if next >= 0 {
			m.cursor = next
			visible := m.visibleRows()
			if m.cursor >= m.scrollOffset+visible {
				m.scrollOffset = m.cursor - visible + 1
			}
		}
	case key.Matches(msg, keys.Enter):
		if m.cursor >= 0 && m.cursor < len(m.displayRows) && !m.displayRows[m.cursor].isHeader {
			src := m.activeSessions()
			idx := m.displayRows[m.cursor].sessionIdx
			if idx >= 0 && idx < len(src) {
				return m, func() tea.Msg {
					return openSessionMsg{info: src[idx]}
				}
			}
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.searchQuery += string(msg.Runes)
			m.filterSessions()
		}
	}
	return m, nil
}

// activeSessions returns the filtered sessions (when searching) or all sessions.
func (m sessionsModel) activeSessions() []session.Info {
	if m.searching {
		return m.filtered
	}
	return m.sessions
}

// filterSessions filters sessions by the search query and rebuilds display rows.
func (m *sessionsModel) filterSessions() {
	q := strings.ToLower(m.searchQuery)
	if q == "" {
		m.filtered = m.sessions
	} else {
		m.filtered = nil
		for _, s := range m.sessions {
			if strings.Contains(strings.ToLower(s.ProjectName), q) ||
				strings.Contains(strings.ToLower(s.GitBranch), q) ||
				strings.Contains(strings.ToLower(s.SessionID), q) ||
				strings.Contains(strings.ToLower(s.Slug), q) {
				m.filtered = append(m.filtered, s)
			}
		}
	}
	m.buildDisplayRows(m.filtered)
	m.cursor = m.nextSelectableRow(0)
	m.scrollOffset = 0
}

// buildDisplayRows builds grouped display rows from the given sessions.
func (m *sessionsModel) buildDisplayRows(sessions []session.Info) {
	m.displayRows = nil
	if len(sessions) == 0 {
		return
	}

	type group struct {
		name    string
		indices []int
		newest  time.Time
	}

	groupMap := make(map[string]*group)
	var groupNames []string
	for i, s := range sessions {
		g, ok := groupMap[s.ProjectName]
		if !ok {
			g = &group{name: s.ProjectName}
			groupMap[s.ProjectName] = g
			groupNames = append(groupNames, s.ProjectName)
		}
		g.indices = append(g.indices, i)
		if g.newest.IsZero() || s.Modified.After(g.newest) {
			g.newest = s.Modified
		}
	}

	sort.SliceStable(groupNames, func(i, j int) bool {
		return groupMap[groupNames[i]].newest.After(groupMap[groupNames[j]].newest)
	})

	for _, name := range groupNames {
		g := groupMap[name]
		m.displayRows = append(m.displayRows, displayRow{
			isHeader:     true,
			projectName:  name,
			sessionCount: len(g.indices),
			sessionIdx:   -1,
		})
		for _, idx := range g.indices {
			m.displayRows = append(m.displayRows, displayRow{
				sessionIdx: idx,
			})
		}
	}
}

// nextSelectableRow returns the first selectable row at or after pos.
func (m sessionsModel) nextSelectableRow(pos int) int {
	if pos < 0 {
		pos = 0
	}
	for i := pos; i < len(m.displayRows); i++ {
		if !m.displayRows[i].isHeader {
			return i
		}
	}
	return -1
}

// nextSelectableRowAfter returns the next selectable row after pos.
func (m sessionsModel) nextSelectableRowAfter(pos int) int {
	for i := pos + 1; i < len(m.displayRows); i++ {
		if !m.displayRows[i].isHeader {
			return i
		}
	}
	return -1
}

// prevSelectableRow returns the previous selectable row before pos.
func (m sessionsModel) prevSelectableRow(pos int) int {
	for i := pos - 1; i >= 0; i-- {
		if !m.displayRows[i].isHeader {
			return i
		}
	}
	return -1
}

func (m sessionsModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}
	if m.width == 0 {
		return ""
	}
	if len(m.sessions) == 0 {
		return "No sessions found.\n"
	}

	var b strings.Builder
	src := m.activeSessions()

	// Title
	if m.searching {
		b.WriteString(styleTitle.Render(fmt.Sprintf(" contextspectre | %d matches", len(src))))
	} else {
		b.WriteString(styleTitle.Render(fmt.Sprintf(" contextspectre | %d sessions", len(m.sessions))))
	}
	b.WriteString("\n")

	// Search bar or blank line
	if m.searching {
		b.WriteString(styleMuted.Render(fmt.Sprintf(" / %s", m.searchQuery)))
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render("\u2588"))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
	}

	// Column widths
	projW := 20
	slugW := 22
	idW := 8
	branchW := 12
	msgsW := 6
	sizeW := 8
	barW := 10
	pctW := 7
	sigW := 4
	costW := 9
	modW := 10

	// Column header
	header := fmt.Sprintf("   %-*s %-*s %-*s %-*s %*s %*s %-*s %*s %*s %*s %*s",
		projW, "Project",
		slugW, "Slug",
		idW, "ID",
		branchW, "Branch",
		msgsW, "Msgs",
		sizeW, "Size",
		barW, "Context",
		pctW, "",
		sigW, "Sig",
		costW, "Cost",
		modW, "Modified",
	)
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n")
	b.WriteString(styleMuted.Render(" " + strings.Repeat("─", m.width-2)))
	b.WriteString("\n")

	// Rows
	visible := m.visibleRows()
	end := m.scrollOffset + visible
	if end > len(m.displayRows) {
		end = len(m.displayRows)
	}

	for i := m.scrollOffset; i < end; i++ {
		row := m.displayRows[i]
		if row.isHeader {
			groupLine := fmt.Sprintf(" \u25be %s (%d sessions)", row.projectName, row.sessionCount)
			b.WriteString(styleHeader.Render(groupLine))
			b.WriteString("\n")
			continue
		}

		s := src[row.sessionIdx]
		isSelected := i == m.cursor

		prefix := "   "
		if isSelected {
			prefix = " \u25b8 "
		}

		active := ""
		if s.IsActive() {
			active = "[ACTIVE] "
		}

		project := truncateStr(active+s.ProjectName, projW)

		slug := truncateStr(s.Slug, slugW)
		if slug == "" {
			slug = "\u2014"
		}

		shortID := s.ShortID()

		branch := truncateStr(s.GitBranch, branchW)
		if branch == "" {
			branch = "\u2014"
		}

		size := fmt.Sprintf("%.1f MB", s.FileSizeMB)

		bar := "\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591"
		pct := "\u2014"
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

		costStr := "—"
		if s.ContextStats != nil && s.ContextStats.EstimatedCost > 0 {
			costStr = analyzer.FormatCost(s.ContextStats.EstimatedCost)
		}

		// Signal health grade
		sigStr := "—"
		if s.ContextStats != nil && s.ContextStats.ContextTokens > 0 {
			grade := analyzer.GradeFromSignalPercent(s.ContextStats.SignalPercent)
			sigStr = gradeStyle(grade).Render(grade)
		}

		// Client type indicator
		clientStr := ""
		if s.ContextStats != nil && s.ContextStats.ClientType == "desktop" {
			clientStr = styleMuted.Render(" DTP")
		}

		line := fmt.Sprintf("%s%-*s %-*s %-*s %-*s %*d %*s %s %*s%s %*s%s %*s %*s",
			prefix,
			projW, project,
			slugW, slug,
			idW, shortID,
			branchW, branch,
			msgsW, s.MessageCount,
			sizeW, size,
			bar,
			pctW, pct,
			compactLabel,
			sigW, sigStr,
			clientStr,
			costW, costStr,
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
	if m.searching {
		b.WriteString(styleFooter.Render(fmt.Sprintf(" / %s  (Esc clear)  \u2191\u2193 navigate  Enter open", m.searchQuery)))
	} else {
		b.WriteString(styleFooter.Render(" \u2191\u2193 navigate  / search  Enter open  q quit"))
	}

	return b.String()
}

func (m sessionsModel) visibleRows() int {
	// Reserve: title (1) + search/blank (1) + header (1) + separator (1) + blank (1) + footer (1) = 6
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
	filledStr := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("\u2588", filled))
	emptyStr := styleMuted.Render(strings.Repeat("\u2591", width-filled))
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
	filledStr := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("\u2588", filled))
	emptyStr := styleMuted.Render(strings.Repeat("\u2591", width-filled))
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
		return "\u2014"
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
