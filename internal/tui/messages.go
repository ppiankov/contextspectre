package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
	"github.com/ppiankov/contextspectre/internal/session"
)

type messagesModel struct {
	session        session.Info
	entries        []jsonl.Entry
	stats          *analyzer.ContextStats
	issues         map[int][]analyzer.Issue
	dupResult      *analyzer.DuplicateReadResult
	staleIndices   map[int]bool
	retryResult    *analyzer.RetryResult
	failedIndices  map[int]bool
	tangentResult  *analyzer.TangentResult
	tangentIndices map[int]bool
	cursor         int
	scrollOffset   int
	selected       map[int]bool
	impact         *analyzer.DeletionImpact
	isActive       bool
	statusMsg      string
	width, height  int
}

type backToSessionsMsg struct{}

type showConfirmMsg struct {
	selected map[int]bool
	impact   *analyzer.DeletionImpact
}

func newMessagesModel(info session.Info) messagesModel {
	entries, err := jsonl.Parse(info.FullPath)
	if err != nil {
		return messagesModel{
			session:   info,
			statusMsg: fmt.Sprintf("Error: %v", err),
		}
	}

	stats := analyzer.Analyze(entries)
	diagnosis := analyzer.Diagnose(entries)
	dupResult := analyzer.FindDuplicateReads(entries)
	retryResult := analyzer.FindFailedRetries(entries)
	tangentResult := analyzer.FindTangents(entries)

	return messagesModel{
		session:        info,
		entries:        entries,
		stats:          stats,
		issues:         diagnosis.IssuesByIndex(),
		dupResult:      dupResult,
		staleIndices:   dupResult.AllStaleIndices(),
		retryResult:    retryResult,
		failedIndices:  retryResult.AllFailedIndices(),
		tangentResult:  tangentResult,
		tangentIndices: tangentResult.AllTangentIndices(),
		selected:       make(map[int]bool),
		isActive:       info.IsActive(),
	}
}

func (m messagesModel) Init() tea.Cmd {
	return nil
}

func (m messagesModel) Update(msg tea.Msg) (messagesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m messagesModel) handleKey(msg tea.KeyMsg) (messagesModel, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.scrollOffset {
				m.scrollOffset = m.cursor
			}
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			visible := m.visibleRows()
			if m.cursor >= m.scrollOffset+visible {
				m.scrollOffset = m.cursor - visible + 1
			}
		}
	case key.Matches(msg, keys.Space):
		if !m.isActive && m.cursor < len(m.entries) {
			if m.selected[m.cursor] {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = true
			}
			m.updateImpact()
		}
	case key.Matches(msg, keys.SelectAllProg):
		if !m.isActive {
			m.selectAllProgress()
			m.updateImpact()
		}
	case key.Matches(msg, keys.ReplaceImages):
		if !m.isActive {
			return m.replaceImages()
		}
	case key.Matches(msg, keys.StripSeps):
		if !m.isActive {
			return m.stripSeparators()
		}
	case key.Matches(msg, keys.SelectSnaps):
		if !m.isActive {
			m.selectAllSnapshots()
			m.updateImpact()
		}
	case key.Matches(msg, keys.SelectStale):
		if !m.isActive {
			m.selectAllStaleReads()
			m.updateImpact()
		}
	case key.Matches(msg, keys.TruncateOutput):
		if !m.isActive {
			return m.truncateOutputs()
		}
	case key.Matches(msg, keys.SelectChains):
		if !m.isActive {
			m.selectAllSidechains()
			m.updateImpact()
		}
	case key.Matches(msg, keys.SelectTangents):
		if !m.isActive {
			m.selectAllTangents()
			m.updateImpact()
		}
	case key.Matches(msg, keys.CleanAll):
		if !m.isActive {
			return m.cleanAll()
		}
	case key.Matches(msg, keys.Delete):
		if !m.isActive && len(m.selected) > 0 {
			m.updateImpact()
			return m, func() tea.Msg {
				return showConfirmMsg{
					selected: m.selected,
					impact:   m.impact,
				}
			}
		}
	case key.Matches(msg, keys.Undo):
		if !m.isActive {
			return m.undoLastChange()
		}
	case key.Matches(msg, keys.Back):
		return m, func() tea.Msg { return backToSessionsMsg{} }
	}
	return m, nil
}

func (m messagesModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Context meter
	b.WriteString(m.renderContextMeter())
	b.WriteString("\n")

	// Separator
	b.WriteString(styleMuted.Render(" " + strings.Repeat("─", m.width-2)))
	b.WriteString("\n")

	// Column header
	header := fmt.Sprintf("   %-12s %8s  %-8s  %s", "Type", "~Tokens", "Time", "Preview")
	b.WriteString(styleHeader.Render(header))
	b.WriteString("\n")

	// Message rows
	visible := m.visibleRows()
	end := m.scrollOffset + visible
	if end > len(m.entries) {
		end = len(m.entries)
	}

	previewWidth := m.width - 38
	if previewWidth < 20 {
		previewWidth = 20
	}

	for i := m.scrollOffset; i < end; i++ {
		e := m.entries[i]
		isSelected := i == m.cursor
		isMarked := m.selected[i]

		// Marker
		marker := "  "
		if isMarked {
			marker = styleMarked.Render("✕ ")
		} else if isSelected {
			marker = "▸ "
		}

		// Type with issue marker
		typeStr := typeIcon(e.Type)
		if _, hasIssue := m.issues[i]; hasIssue {
			typeStr = styleWarning.Render("!") + typeStr
		}

		// Tokens
		tokenStr := "—"
		if e.IsConversational() {
			tokens := analyzer.EstimateTokens(&e)
			if tokens > 0 {
				tokenStr = formatTokensShort(tokens)
			}
		}

		// Stale/failed/tangent indicators
		staleLabel := ""
		if m.tangentIndices[i] {
			staleLabel = styleMuted.Render(" tangent")
		} else if m.staleIndices[i] {
			staleLabel = styleMuted.Render(" stale")
		} else if m.failedIndices[i] {
			staleLabel = styleMuted.Render(" failed")
		}

		// Image cost indicator
		imgLabel := ""
		if e.HasImages() {
			imgTok := analyzer.EstimateImageTokens(&e)
			if imgTok > 0 {
				imgLabel = styleWarning.Render(fmt.Sprintf(" img:%s", formatTokensShort(imgTok)))
			}
		}

		// Time
		timeStr := e.Timestamp.Format("15:04")

		// Preview
		preview := e.ContentPreview(previewWidth)

		line := fmt.Sprintf("%s%-12s %8s  %-8s  %s%s%s",
			marker, typeStr, tokenStr, timeStr, truncateStr(preview, previewWidth), staleLabel, imgLabel)

		if isSelected && isMarked {
			b.WriteString(lipgloss.NewStyle().Background(colorRed).Foreground(colorWhite).Render(line))
		} else if isSelected {
			b.WriteString(styleSelected.Render(line))
		} else if isMarked {
			b.WriteString(styleMarked.Render(line))
		} else if e.Type == jsonl.TypeProgress || e.IsSidechain || m.tangentIndices[i] {
			b.WriteString(styleMuted.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// Impact bar
	b.WriteString("\n")
	b.WriteString(m.renderImpactBar())
	b.WriteString("\n")

	// Footer
	if m.isActive {
		b.WriteString(styleActive.Render(" [ACTIVE SESSION — READ ONLY]"))
	} else {
		b.WriteString(styleFooter.Render(" Space sel  x prog  h snap  r stale  c chain  g tang  a all  i img  s sep  t trunc  d del  u undo  q back"))
	}

	if m.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString(styleMuted.Render(" " + m.statusMsg))
	}

	return b.String()
}

func (m messagesModel) renderContextMeter() string {
	if m.stats == nil {
		return ""
	}

	var b strings.Builder

	// Title line
	b.WriteString(styleTitle.Render(fmt.Sprintf(" contextspectre | %s/%s",
		m.session.ProjectName, shortUUID(m.session.SessionID))))
	b.WriteString("\n\n")

	// Context bar
	pct := m.stats.UsagePercent
	isCompacted := m.stats.CompactionCount > 0
	barWidth := 20
	filled := int(pct / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	var color lipgloss.Color
	if isCompacted {
		color = contextColorCompacted(pct)
	} else {
		color = contextColor(pct)
	}

	filledStr := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled))
	emptyStr := styleMuted.Render(strings.Repeat("░", barWidth-filled))

	fmt.Fprintf(&b, " Context: %s%s  %.1f%% (%s / %s)",
		filledStr, emptyStr,
		pct,
		formatTokensFull(m.stats.CurrentContextTokens),
		formatTokensFull(analyzer.ContextWindowSize))

	// Post-compaction label
	if isCompacted {
		last := m.stats.Compactions[len(m.stats.Compactions)-1]
		b.WriteString(styleCompacted.Render(fmt.Sprintf("  compacted from %.0f%%",
			float64(last.BeforeTokens)/float64(analyzer.ContextWindowSize)*100)))
	}

	// Compaction imminent warning
	if pct >= 90 && m.stats.EstimatedTurnsLeft >= 0 {
		b.WriteString(styleWarning.Render(fmt.Sprintf("  !! ~%d turns left", m.stats.EstimatedTurnsLeft)))
	} else if pct >= 85 {
		b.WriteString(styleWarning.Render("  !! COMPACTION IMMINENT"))
	}
	b.WriteString("\n")

	// Ghost bar showing pre-compaction level
	if isCompacted {
		last := m.stats.Compactions[len(m.stats.Compactions)-1]
		ghostPct := float64(last.BeforeTokens) / float64(analyzer.ContextWindowSize) * 100
		ghostFilled := int(ghostPct / 100 * float64(barWidth))
		if ghostFilled > barWidth {
			ghostFilled = barWidth
		}
		ghostBar := styleGhost.Render(strings.Repeat("▓", ghostFilled) + strings.Repeat("░", barWidth-ghostFilled))
		b.WriteString(styleMuted.Render(fmt.Sprintf(" Before:  %s  %.0f%% (%s)",
			ghostBar,
			ghostPct,
			formatTokensFull(last.BeforeTokens))))
		b.WriteString("\n")
	}

	// Stats line
	turnsStr := "unknown"
	turnsLabel := "turns until next"
	if m.stats.EstimatedTurnsLeft >= 0 {
		turnsStr = fmt.Sprintf("~%d", m.stats.EstimatedTurnsLeft)
	}
	if isCompacted {
		turnsLabel = "turns until next (since last compaction)"
	}
	// Cost attribution
	costStr := ""
	if m.stats.Cost != nil && m.stats.Cost.TurnCount > 0 {
		costStr = fmt.Sprintf("  |  Cost: %s (%s/t)",
			analyzer.FormatCost(m.stats.Cost.TotalCost),
			analyzer.FormatCost(m.stats.Cost.CostPerTurn))
	}

	b.WriteString(styleMuted.Render(fmt.Sprintf(" Compactions: %d  |  %s %s%s  |  Images: %d",
		m.stats.CompactionCount,
		turnsStr,
		turnsLabel,
		costStr,
		m.stats.ImageCount)))

	if m.stats.ImageBytesTotal > 0 {
		b.WriteString(styleMuted.Render(fmt.Sprintf(" (%.1f MB)", float64(m.stats.ImageBytesTotal)/1024/1024)))
	}

	if m.stats.SnapshotCount > 0 {
		b.WriteString(styleMuted.Render(fmt.Sprintf("  |  Snapshots: %d (%.1f MB)",
			m.stats.SnapshotCount, float64(m.stats.SnapshotBytesTotal)/1024/1024)))
	}

	if m.dupResult != nil && m.dupResult.TotalStale > 0 {
		b.WriteString(styleMuted.Render(fmt.Sprintf("  |  Stale reads: %d across %d files (~%s tok)",
			m.dupResult.TotalStale, m.dupResult.UniqueFiles, formatTokensShort(m.dupResult.TotalTokens))))
	}

	if m.stats.LargeOutputCount > 0 {
		b.WriteString(styleMuted.Render(fmt.Sprintf("  |  Large outputs: %d (~%s tok)",
			m.stats.LargeOutputCount, formatTokensShort(m.stats.LargeOutputTokens))))
	}

	if m.retryResult != nil && m.retryResult.TotalFailed > 0 {
		b.WriteString(styleMuted.Render(fmt.Sprintf("  |  Failed retries: %d (~%s tok)",
			m.retryResult.TotalFailed, formatTokensShort(m.retryResult.TotalTokens))))
	}

	if m.stats.SidechainCount > 0 {
		b.WriteString(styleMuted.Render(fmt.Sprintf("  |  Sidechains: %d entries, %d groups (~%s tok)",
			m.stats.SidechainCount, m.stats.SidechainGroups, formatTokensShort(m.stats.SidechainTokens))))
	}

	if m.tangentResult != nil && m.tangentResult.TotalEntries > 0 {
		b.WriteString(styleMuted.Render(fmt.Sprintf("  |  Tangents: %d entries, %d groups (~%s tok)",
			m.tangentResult.TotalEntries, len(m.tangentResult.Groups), formatTokensShort(m.tangentResult.TotalTokens))))
	}

	// Image weight warning when images are >10% of context
	if m.stats.CurrentContextTokens > 0 && m.stats.ImageCount > 0 {
		imgTokens := m.estimateTotalImageTokens()
		imgPct := float64(imgTokens) / float64(m.stats.CurrentContextTokens) * 100
		if imgPct > 10 {
			b.WriteString("\n")
			b.WriteString(styleWarning.Render(fmt.Sprintf(" !! Images: %.1f%% of context (%d images, %.1f MB) — press i to replace",
				imgPct, m.stats.ImageCount, float64(m.stats.ImageBytesTotal)/1024/1024)))
		}
	}

	return b.String()
}

func (m messagesModel) renderImpactBar() string {
	count := len(m.selected)
	if count == 0 {
		return styleMuted.Render(" Selected: 0 | Savings: 0 tokens (0.0%)")
	}

	if m.impact == nil {
		return styleMuted.Render(fmt.Sprintf(" Selected: %d | calculating...", count))
	}

	line := fmt.Sprintf(" Selected: %d | Savings: ~%s tokens (%.1f%%) | New: %.1f%% | +%d turns",
		m.impact.SelectedCount,
		formatTokensShort(m.impact.EstimatedTokenSaved),
		float64(m.impact.EstimatedTokenSaved)/float64(analyzer.ContextWindowSize)*100,
		m.impact.NewContextPercent,
		m.impact.PredictedTurnsGained,
	)

	return lipgloss.NewStyle().Foreground(colorGreen).Render(line)
}

func (m *messagesModel) updateImpact() {
	if len(m.selected) == 0 {
		m.impact = nil
		return
	}
	m.impact = analyzer.PredictImpact(m.entries, m.selected, m.stats)
}

func (m *messagesModel) selectAllProgress() {
	for i, e := range m.entries {
		if e.Type == jsonl.TypeProgress {
			m.selected[i] = true
		}
	}
}

func (m *messagesModel) selectAllSnapshots() {
	for i, e := range m.entries {
		if e.Type == jsonl.TypeFileHistorySnapshot {
			m.selected[i] = true
		}
	}
}

func (m *messagesModel) selectAllStaleReads() {
	for idx := range m.staleIndices {
		m.selected[idx] = true
	}
}

func (m *messagesModel) selectAllSidechains() {
	for i, e := range m.entries {
		if e.IsSidechain {
			m.selected[i] = true
		}
	}
}

func (m *messagesModel) selectAllTangents() {
	for idx := range m.tangentIndices {
		m.selected[idx] = true
	}
}

func (m messagesModel) replaceImages() (messagesModel, tea.Cmd) {
	// Preview savings before executing
	imgTokens := m.estimateTotalImageTokens()
	if imgTokens == 0 {
		m.statusMsg = "No images to replace."
		return m, nil
	}

	result, err := editor.ReplaceImages(m.session.FullPath)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Error: %v", err)
		return m, nil
	}
	if result.ImagesReplaced == 0 {
		m.statusMsg = "No images to replace."
		return m, nil
	}

	savedPct := float64(imgTokens) / float64(analyzer.ContextWindowSize) * 100
	m.statusMsg = fmt.Sprintf("Replaced %d images, saved ~%s tokens (%.1f%%), %.1f KB on disk",
		result.ImagesReplaced, formatTokensShort(imgTokens), savedPct, float64(result.BytesSaved)/1024)

	// Reload session data
	return m.reload(), nil
}

func (m messagesModel) cleanAll() (messagesModel, tea.Cmd) {
	result, err := editor.CleanAll(m.session.FullPath)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Error: %v", err)
		return m, nil
	}
	m.statusMsg = fmt.Sprintf("Cleaned: %d prog, %d snap, %d chain, %d tang, %d retry, %d stale, %d img, %d sep, %d trunc — saved ~%d tokens",
		result.ProgressRemoved, result.SnapshotsRemoved, result.SidechainsRemoved,
		result.TangentsRemoved, result.FailedRetries, result.StaleReadsRemoved,
		result.ImagesReplaced, result.SeparatorsStripped, result.OutputsTruncated,
		result.TotalTokensSaved)
	return m.reload(), nil
}

func (m messagesModel) truncateOutputs() (messagesModel, tea.Cmd) {
	result, err := editor.TruncateOutputs(m.session.FullPath, analyzer.LargeOutputThreshold, 10)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Error: %v", err)
		return m, nil
	}
	if result.OutputsTruncated == 0 {
		m.statusMsg = "No large outputs to truncate."
		return m, nil
	}
	m.statusMsg = fmt.Sprintf("Truncated %d outputs, saved ~%d tokens",
		result.OutputsTruncated, result.TokensSaved)
	return m.reload(), nil
}

func (m messagesModel) stripSeparators() (messagesModel, tea.Cmd) {
	result, err := editor.StripSeparators(m.session.FullPath)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Error: %v", err)
		return m, nil
	}
	if result.LinesStripped == 0 {
		m.statusMsg = "No decorative separators found."
		return m, nil
	}
	m.statusMsg = fmt.Sprintf("Stripped %d separator lines from %d messages, saved ~%d tokens",
		result.LinesStripped, result.MessagesModified, result.CharsSaved/4)
	return m.reload(), nil
}

func (m messagesModel) estimateTotalImageTokens() int {
	total := 0
	for i := range m.entries {
		total += analyzer.EstimateImageTokens(&m.entries[i])
	}
	return total
}

func (m messagesModel) undoLastChange() (messagesModel, tea.Cmd) {
	if !safecopy.Exists(m.session.FullPath) {
		m.statusMsg = "No backup to restore."
		return m, nil
	}
	if err := safecopy.Restore(m.session.FullPath); err != nil {
		m.statusMsg = fmt.Sprintf("Restore error: %v", err)
		return m, nil
	}
	m.statusMsg = "Restored from safecopy."
	return m.reload(), nil
}

func (m messagesModel) reload() messagesModel {
	entries, err := jsonl.Parse(m.session.FullPath)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Reload error: %v", err)
		return m
	}
	m.entries = entries
	m.stats = analyzer.Analyze(entries)
	m.issues = analyzer.Diagnose(entries).IssuesByIndex()
	m.dupResult = analyzer.FindDuplicateReads(entries)
	m.staleIndices = m.dupResult.AllStaleIndices()
	m.retryResult = analyzer.FindFailedRetries(entries)
	m.failedIndices = m.retryResult.AllFailedIndices()
	m.tangentResult = analyzer.FindTangents(entries)
	m.tangentIndices = m.tangentResult.AllTangentIndices()
	m.selected = make(map[int]bool)
	m.impact = nil
	if m.cursor >= len(m.entries) {
		m.cursor = len(m.entries) - 1
	}
	return m
}

func (m messagesModel) visibleRows() int {
	// Reserve: title(3) + context meter(4) + separator(1) + header(1) + impact(2) + footer(2) = 13
	reserved := 13
	// Extra line for ghost bar when session has compacted
	if m.stats != nil && m.stats.CompactionCount > 0 {
		reserved++
	}
	// Extra line for image weight warning
	if m.stats != nil && m.stats.CurrentContextTokens > 0 && m.stats.ImageCount > 0 {
		imgTokens := m.estimateTotalImageTokens()
		imgPct := float64(imgTokens) / float64(m.stats.CurrentContextTokens) * 100
		if imgPct > 10 {
			reserved++
		}
	}
	avail := m.height - reserved
	if avail < 3 {
		return 3
	}
	return avail
}

func typeIcon(t jsonl.MessageType) string {
	switch t {
	case jsonl.TypeUser:
		return "user"
	case jsonl.TypeAssistant:
		return "assistant"
	case jsonl.TypeProgress:
		return "progress"
	case jsonl.TypeFileHistorySnapshot:
		return "snapshot"
	case jsonl.TypeSystem:
		return "system"
	default:
		return string(t)
	}
}

func formatTokensShort(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("~%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("~%d", n)
}

func formatTokensFull(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%d,%03d,%03d", n/1000000, (n/1000)%1000, n%1000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d", n)
}

func shortUUID(uuid string) string {
	if len(uuid) > 8 {
		return uuid[:8]
	}
	return uuid
}
