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
	aliasLookup   map[string]string // encoded path prefix → alias name
	costThreshold float64           // cost alert threshold (0 = disabled)
	width, height int
	err           error
}

type openSessionMsg struct {
	info session.Info
}

func newSessionsModel(sessions []session.Info, aliasLookup map[string]string, costThreshold float64) sessionsModel {
	m := sessionsModel{
		sessions:      sessions,
		aliasLookup:   aliasLookup,
		costThreshold: costThreshold,
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
			aliasName := resolveAliasName(s.FullPath, m.aliasLookup)
			if strings.Contains(strings.ToLower(s.ProjectName), q) ||
				strings.Contains(strings.ToLower(s.GitBranch), q) ||
				strings.Contains(strings.ToLower(s.SessionID), q) ||
				strings.Contains(strings.ToLower(s.Slug), q) ||
				(aliasName != "" && strings.Contains(strings.ToLower(aliasName), q)) {
				m.filtered = append(m.filtered, s)
			}
		}
	}
	m.buildDisplayRows(m.filtered)
	m.cursor = m.nextSelectableRow(0)
	m.scrollOffset = 0
}

// projectGroupName returns the display group name for a session.
// If the session matches an alias, returns the alias name; otherwise the project name.
func (m sessionsModel) projectGroupName(s session.Info) string {
	if name := resolveAliasName(s.FullPath, m.aliasLookup); name != "" {
		return name
	}
	return s.ProjectName
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
		gname := m.projectGroupName(s)
		g, ok := groupMap[gname]
		if !ok {
			g = &group{name: gname}
			groupMap[gname] = g
			groupNames = append(groupNames, gname)
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

	// Detect if any session has a branch
	hasBranch := false
	for _, s := range src {
		if s.GitBranch != "" {
			hasBranch = true
			break
		}
	}

	// Layout: compute column widths based on terminal width
	cols := computeColumns(m.width, hasBranch)

	// Column header
	var hdr strings.Builder
	hdr.WriteString("    ") // prefix: active char + selector + space
	fmt.Fprintf(&hdr, "%-*s ", cols.projW, "Project")
	fmt.Fprintf(&hdr, "%-*s ", cols.slugW, "Slug")
	fmt.Fprintf(&hdr, "%-*s ", cols.idW, "ID")
	if cols.showBranch {
		fmt.Fprintf(&hdr, "%-*s ", cols.branchW, "Branch")
	}
	fmt.Fprintf(&hdr, "%*s ", cols.msgsW, "Msgs")
	if cols.showSize {
		fmt.Fprintf(&hdr, "%*s ", cols.sizeW, "Size")
	}
	fmt.Fprintf(&hdr, "%-*s ", cols.barW, "Context")
	fmt.Fprintf(&hdr, "%*s ", cols.pctW, "")
	if !cols.mergeSignal {
		fmt.Fprintf(&hdr, "%*s ", cols.sigW, "Sig")
	}
	fmt.Fprintf(&hdr, "%*s ", cols.costW, "Cost")
	fmt.Fprintf(&hdr, "%*s", cols.modW, "Modified")
	b.WriteString(styleHeader.Render(hdr.String()))
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

		// Active indicator: single char prefix
		activeChar := " "
		if s.IsActive() {
			activeChar = lipgloss.NewStyle().Foreground(colorYellow).Render("\u25cf")
		}
		selector := "  "
		if isSelected {
			selector = "\u25b8 "
		}
		prefix := " " + activeChar + selector

		project := truncateStr(s.ProjectName, cols.projW)

		slug := middleTruncate(s.Slug, cols.slugW)
		if slug == "" {
			slug = "\u2014"
		}

		shortID := s.ShortID()

		bar := "\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591"
		pct := "\u2014"
		compactLabel := ""
		if s.ContextStats != nil && s.ContextStats.ContextTokens > 0 {
			pctVal := s.ContextStats.ContextPct
			if s.ContextStats.CompactionCount > 0 {
				bar = contextBarStrCompacted(pctVal, cols.barW)
				compactLabel = styleCompacted.Render(fmt.Sprintf(" %dx", s.ContextStats.CompactionCount))
			} else {
				bar = contextBarStr(pctVal, cols.barW)
			}
			pct = fmt.Sprintf("%.1f%%", pctVal)
		}

		mod := timeAgoStr(s.Modified)

		costStr := "\u2014"
		costAlert := false
		if s.ContextStats != nil && s.ContextStats.EstimatedCost > 0 {
			costStr = analyzer.FormatCost(s.ContextStats.EstimatedCost)
			if m.costThreshold > 0 && s.ContextStats.EstimatedCost >= m.costThreshold {
				costAlert = true
			}
		}

		// Signal health grade
		sigStr := "\u2014"
		if s.ContextStats != nil && s.ContextStats.ContextTokens > 0 {
			grade := analyzer.GradeFromSignalPercent(s.ContextStats.SignalPercent)
			sigStr = gradeStyle(grade).Render(grade)
		}

		// Cost alert indicator
		costAlertStr := ""
		if costAlert {
			costAlertStr = lipgloss.NewStyle().Foreground(colorRed).Render("!!")
		}

		var line strings.Builder
		line.WriteString(prefix)
		line.WriteString(fmt.Sprintf("%-*s ", cols.projW, project))
		line.WriteString(fmt.Sprintf("%-*s ", cols.slugW, slug))
		line.WriteString(fmt.Sprintf("%-*s ", cols.idW, shortID))
		if cols.showBranch {
			branch := truncateStr(s.GitBranch, cols.branchW)
			if branch == "" {
				branch = "\u2014"
			}
			line.WriteString(fmt.Sprintf("%-*s ", cols.branchW, branch))
		}
		line.WriteString(fmt.Sprintf("%*d ", cols.msgsW, s.MessageCount))
		if cols.showSize {
			size := fmt.Sprintf("%.1f MB", s.FileSizeMB)
			line.WriteString(fmt.Sprintf("%*s ", cols.sizeW, size))
		}
		line.WriteString(bar)
		line.WriteString(fmt.Sprintf(" %*s", cols.pctW, pct))
		line.WriteString(compactLabel)
		if !cols.mergeSignal {
			line.WriteString(fmt.Sprintf(" %*s", cols.sigW, sigStr))
		}
		line.WriteString(fmt.Sprintf(" %*s%s", cols.costW, costStr, costAlertStr))
		line.WriteString(fmt.Sprintf(" %*s", cols.modW, mod))

		lineStr := line.String()
		if isSelected {
			b.WriteString(styleSelected.Render(lineStr))
		} else if s.IsActive() {
			b.WriteString(styleActive.Render(lineStr))
		} else {
			b.WriteString(lineStr)
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

// columnLayout holds computed column widths for the session browser.
type columnLayout struct {
	projW       int
	slugW       int
	idW         int
	branchW     int
	showBranch  bool
	msgsW       int
	sizeW       int
	showSize    bool
	barW        int
	pctW        int
	sigW        int
	mergeSignal bool // true when signal is shown as context bar color only
	costW       int
	modW        int
}

// computeColumns calculates responsive column widths based on terminal width.
func computeColumns(width int, hasBranch bool) columnLayout {
	c := columnLayout{
		idW:   8,
		msgsW: 6,
		barW:  10,
		pctW:  6,
		sigW:  3,
		costW: 8,
	}

	// Fixed overhead: prefix (4) + spaces between columns
	const prefixW = 4

	switch {
	case width > 160: // Wide
		c.showBranch = hasBranch
		c.showSize = true
		c.mergeSignal = false
		c.sizeW = 8
		c.modW = 8
		if hasBranch {
			c.branchW = 12
		}
		// Distribute remaining to project + slug
		fixed := prefixW + c.idW + c.msgsW + c.sizeW + c.barW + c.pctW + c.sigW + c.costW + c.modW + 10 // spaces
		if c.showBranch {
			fixed += c.branchW + 1
		}
		remaining := width - fixed
		if remaining < 20 {
			remaining = 20
		}
		c.projW = remaining * 45 / 100
		c.slugW = remaining - c.projW

	case width >= 120: // Medium
		c.showBranch = false
		c.showSize = false
		c.mergeSignal = false
		c.modW = 6
		fixed := prefixW + c.idW + c.msgsW + c.barW + c.pctW + c.sigW + c.costW + c.modW + 8
		remaining := width - fixed
		if remaining < 20 {
			remaining = 20
		}
		c.projW = remaining * 45 / 100
		c.slugW = remaining - c.projW

	default: // Narrow (<120)
		c.showBranch = false
		c.showSize = false
		c.mergeSignal = true
		c.modW = 5
		c.costW = 7
		c.pctW = 5
		fixed := prefixW + c.idW + c.msgsW + c.barW + c.pctW + c.costW + c.modW + 7
		remaining := width - fixed
		if remaining < 16 {
			remaining = 16
		}
		c.projW = remaining * 45 / 100
		c.slugW = remaining - c.projW
	}

	// Clamp minimums
	if c.projW < 8 {
		c.projW = 8
	}
	if c.slugW < 8 {
		c.slugW = 8
	}

	return c
}

// middleTruncate truncates a string from the middle with an ellipsis.
// "shimmying-twirling-charm" → "shimmying…charm"
func middleTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	// Split: keep more from start than end for readability
	half := (maxLen - 1) / 2 // -1 for the "…"
	endLen := maxLen - 1 - half
	return s[:half] + "\u2026" + s[len(s)-endLen:]
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
