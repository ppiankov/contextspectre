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

// sortField identifies which column to sort sessions by.
type sortField int

const (
	sortModified sortField = iota
	sortCost
	sortContext
	sortSignal
	sortSize
	sortFieldCount // sentinel for cycling
)

func (f sortField) label() string {
	switch f {
	case sortModified:
		return "Modified"
	case sortCost:
		return "Cost"
	case sortContext:
		return "Context"
	case sortSignal:
		return "Sig"
	case sortSize:
		return "Size"
	}
	return ""
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
	nav           navState
	sortBy        sortField
	help          helpModel
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
	// Help overlay intercepts all keys.
	if m.help.visible {
		if key.Matches(msg, keys.Help) || key.Matches(msg, keys.Escape) {
			m.help.dismiss()
		}
		return m, nil
	}

	// Vim navigation.
	if action := m.nav.handleVimNav(msg, true); action != navNone {
		m.cursor, m.scrollOffset = applyNavAction(action, m.cursor, m.scrollOffset, len(m.displayRows), m.visibleRows())
		// Snap to nearest selectable row.
		m.cursor = m.nearestSelectableRow(m.cursor)
		return m, nil
	}

	// Space = page down (not bound in sessions otherwise).
	if key.Matches(msg, keys.Space) {
		m.cursor, m.scrollOffset = applyNavAction(navPageDown, m.cursor, m.scrollOffset, len(m.displayRows), m.visibleRows())
		m.cursor = m.nearestSelectableRow(m.cursor)
		return m, nil
	}

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
	case key.Matches(msg, keys.Help):
		m.help.width = m.width
		m.help.height = m.height
		m.help.toggle("Session Browser", sessionsHelp())
	case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 's':
		m.cycleSortField()
	case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'v':
		if m.cursor >= 0 && m.cursor < len(m.displayRows) && !m.displayRows[m.cursor].isHeader {
			src := m.activeSessions()
			idx := m.displayRows[m.cursor].sessionIdx
			if idx >= 0 && idx < len(src) {
				return m, func() tea.Msg {
					return openVectorMsg{info: src[idx]}
				}
			}
		}
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
				strings.Contains(strings.ToLower(s.DisplayName()), q) ||
				strings.Contains(strings.ToLower(s.CustomTitle), q) ||
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

// nearestSelectableRow finds the closest selectable row to pos (forward first, then backward).
func (m sessionsModel) nearestSelectableRow(pos int) int {
	if pos < 0 {
		pos = 0
	}
	if pos >= len(m.displayRows) {
		pos = len(m.displayRows) - 1
	}
	if pos >= 0 && !m.displayRows[pos].isHeader {
		return pos
	}
	fwd := m.nextSelectableRow(pos)
	bwd := m.prevSelectableRow(pos)
	if fwd >= 0 {
		return fwd
	}
	if bwd >= 0 {
		return bwd
	}
	return 0
}

// cycleSortField advances to the next sort field and re-sorts.
func (m *sessionsModel) cycleSortField() {
	m.sortBy = sortField((int(m.sortBy) + 1) % int(sortFieldCount))
	m.sortSessions()
}

// sortSessions sorts the active sessions by the current sort field and rebuilds display rows.
func (m *sessionsModel) sortSessions() {
	src := m.activeSessions()
	sorted := make([]session.Info, len(src))
	copy(sorted, src)

	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		switch m.sortBy {
		case sortCost:
			ac, bc := 0.0, 0.0
			if a.ContextStats != nil {
				ac = a.ContextStats.EstimatedCost
			}
			if b.ContextStats != nil {
				bc = b.ContextStats.EstimatedCost
			}
			return ac > bc // descending
		case sortContext:
			ap, bp := 0.0, 0.0
			if a.ContextStats != nil {
				ap = a.ContextStats.ContextPct
			}
			if b.ContextStats != nil {
				bp = b.ContextStats.ContextPct
			}
			return ap > bp
		case sortSignal:
			as, bs := 0, 0
			if a.ContextStats != nil {
				as = a.ContextStats.SignalPercent
			}
			if b.ContextStats != nil {
				bs = b.ContextStats.SignalPercent
			}
			return as > bs
		case sortSize:
			return a.FileSizeMB > b.FileSizeMB
		default: // sortModified
			return a.Modified.After(b.Modified)
		}
	})

	if m.searching {
		m.filtered = sorted
	} else {
		m.sessions = sorted
	}
	m.buildDisplayRows(sorted)
	m.cursor = m.nextSelectableRow(0)
	m.scrollOffset = 0
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

	// Detect if any session has a non-trivial branch.
	// Check ALL sessions (not just filtered) so column doesn't jump during search.
	hasBranch := false
	for _, s := range m.sessions {
		if s.GitBranch != "" && strings.TrimSpace(s.GitBranch) != "" {
			hasBranch = true
			break
		}
	}

	// Layout: compute column widths based on terminal width
	cols := computeColumns(m.width, hasBranch)

	// Column header with sort indicator
	sortInd := " \u25bc" // ▼ descending indicator
	var hdr strings.Builder
	hdr.WriteString("     ") // prefix: active char + client type + selector + space
	fmt.Fprintf(&hdr, "%-*s ", cols.projW, "Project")
	fmt.Fprintf(&hdr, "%-*s ", cols.slugW, "Slug")
	fmt.Fprintf(&hdr, "%-*s ", cols.idW, "ID")
	if cols.showBranch {
		fmt.Fprintf(&hdr, "%-*s ", cols.branchW, "Branch")
	}
	fmt.Fprintf(&hdr, "%*s ", cols.msgsW, "Msgs")
	if cols.showSize {
		sizeLabel := "Size"
		if m.sortBy == sortSize {
			sizeLabel += sortInd
		}
		fmt.Fprintf(&hdr, "%*s ", cols.sizeW, sizeLabel)
	}
	ctxLabel := "Context"
	if m.sortBy == sortContext {
		ctxLabel += sortInd
	}
	fmt.Fprintf(&hdr, "%-*s ", cols.barW, ctxLabel)
	fmt.Fprintf(&hdr, "%*s ", cols.pctW, "")
	hdr.WriteString("     ") // compaction count column (4 chars + space)
	if !cols.mergeSignal {
		sigLabel := "Sig/Ent"
		if m.sortBy == sortSignal {
			sigLabel += sortInd
		}
		fmt.Fprintf(&hdr, "%*s ", cols.sigW, sigLabel)
	}
	costLabel := "Cost"
	if m.sortBy == sortCost {
		costLabel += sortInd
	}
	fmt.Fprintf(&hdr, "%*s ", cols.costW, costLabel)
	modLabel := "Modified"
	if m.sortBy == sortModified {
		modLabel += sortInd
	}
	fmt.Fprintf(&hdr, "%*s", cols.modW, modLabel)
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

		// Active/zombie indicator: single char prefix
		isZombie := isSessionZombie(s)
		activeChar := " "
		if isZombie {
			activeChar = lipgloss.NewStyle().Foreground(colorRed).Render("\u2717")
		} else if s.IsActive() {
			activeChar = lipgloss.NewStyle().Foreground(colorYellow).Render("\u25cf")
		}
		selector := "  "
		if isSelected {
			selector = "\u25b8 "
		}
		clientChar := clientTypeChar(s)
		prefix := " " + activeChar + clientChar + selector

		project := truncateStr(s.ProjectName, cols.projW)

		slug := middleTruncate(s.DisplayName(), cols.slugW)
		if slug == s.ShortID() {
			slug = "\u2014"
		}

		shortID := s.ShortID()

		bar := "\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591\u2591"
		pct := "\u2014"
		compactLabel := "    " // fixed 4-char field for alignment
		if s.ContextStats != nil && s.ContextStats.ContextTokens > 0 {
			pctVal := s.ContextStats.ContextPct
			if s.ContextStats.CompactionCount > 0 {
				bar = contextBarStrCompacted(pctVal, cols.barW)
				compactLabel = styleCompacted.Render(fmt.Sprintf("%3dx", s.ContextStats.CompactionCount))
			} else {
				bar = contextBarStr(pctVal, cols.barW)
			}
			pct = fmt.Sprintf("%5.1f%%", pctVal)
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
			ent := entropyShort(s.ContextStats.EntropyLevel)
			cleanDot := cleanupStatusDot(s.ContextStats.CleanupStatus)
			sigStr = fmt.Sprintf("%s/%s%s",
				gradeStyle(grade).Render(grade),
				entropyStyle(s.ContextStats.EntropyLevel).Render(ent),
				cleanDot)
		}

		// Cost alert indicator
		costAlertStr := ""
		if costAlert {
			costAlertStr = lipgloss.NewStyle().Foreground(colorRed).Render("!!")
		}

		var line strings.Builder
		line.WriteString(prefix)
		fmt.Fprintf(&line, "%-*s ", cols.projW, project)
		fmt.Fprintf(&line, "%-*s ", cols.slugW, slug)
		fmt.Fprintf(&line, "%-*s ", cols.idW, shortID)
		if cols.showBranch {
			branch := truncateStr(s.GitBranch, cols.branchW)
			if branch == "" {
				branch = "\u2014"
			}
			fmt.Fprintf(&line, "%-*s ", cols.branchW, branch)
		}
		fmt.Fprintf(&line, "%*d ", cols.msgsW, s.MessageCount)
		if cols.showSize {
			size := fmt.Sprintf("%.1f MB", s.FileSizeMB)
			fmt.Fprintf(&line, "%*s ", cols.sizeW, size)
		}
		line.WriteString(bar)
		fmt.Fprintf(&line, " %*s", cols.pctW, pct)
		fmt.Fprintf(&line, " %s", compactLabel)
		if !cols.mergeSignal {
			fmt.Fprintf(&line, " %*s", cols.sigW, sigStr)
		}
		fmt.Fprintf(&line, " %*s%s", cols.costW, costStr, costAlertStr)
		fmt.Fprintf(&line, " %*s", cols.modW, mod)

		lineStr := line.String()
		if isSelected {
			b.WriteString(styleSelected.Render(lineStr))
		} else if isZombie {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(lineStr))
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
		matchInfo := fmt.Sprintf("%d matches", len(src))
		b.WriteString(styleFooter.Render(fmt.Sprintf(" / %s  (%s)  Esc clear  \u2191\u2193 navigate  Enter open", m.searchQuery, matchInfo)))
	} else {
		sortLabel := m.sortBy.label()
		b.WriteString(styleFooter.Render(fmt.Sprintf(" \u2191\u2193/G/gg navigate  / search  s sort (%s)  v vector  ? help  Enter open  q quit", sortLabel)))
	}

	// Help overlay on top if visible.
	view := b.String()
	if m.help.visible {
		return m.help.View()
	}
	return view
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
		sigW:  7,
		costW: 8,
	}

	// Fixed overhead: prefix (5) + spaces between columns
	const prefixW = 5

	const compactW = 5 // fixed compaction count column (4 chars + space)

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
		fixed := prefixW + c.idW + c.msgsW + c.sizeW + c.barW + c.pctW + compactW + c.sigW + c.costW + c.modW + 10 // spaces
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
		fixed := prefixW + c.idW + c.msgsW + c.barW + c.pctW + compactW + c.sigW + c.costW + c.modW + 8
		remaining := width - fixed
		if remaining < 20 {
			remaining = 20
		}
		c.projW = remaining * 45 / 100
		c.slugW = remaining - c.projW

	default: // Narrow (<120)
		c.showBranch = false
		c.showSize = false
		c.mergeSignal = false
		c.modW = 5
		c.costW = 7
		c.pctW = 5
		fixed := prefixW + c.idW + c.msgsW + c.barW + c.pctW + compactW + c.sigW + c.costW + c.modW + 8
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

func entropyShort(level string) string {
	switch level {
	case "LOW":
		return "L"
	case "MEDIUM":
		return "M"
	case "HIGH":
		return "H"
	case "CRITICAL":
		return "C"
	default:
		return "\u2014"
	}
}

func clientTypeChar(s session.Info) string {
	if s.ContextStats == nil {
		return styleMuted.Render("?")
	}
	switch s.ContextStats.ClientType {
	case "cli":
		return styleMuted.Render("C")
	case "desktop":
		return styleMuted.Render("M")
	default:
		return styleMuted.Render("?")
	}
}

// isSessionZombie returns the cached zombie state from session discovery.
func isSessionZombie(s session.Info) bool {
	return s.Zombie
}
