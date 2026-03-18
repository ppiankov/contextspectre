package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/safecopy"
	"github.com/ppiankov/contextspectre/internal/session"
)

type detailPanel int

const (
	panelOverview detailPanel = iota
	panelMessages
	panelCleanup
	panelGhost
)

var panelNames = []string{"Overview", "Messages", "Cleanup", "Ghost"}

// detailModel wraps the session detail view with tabbed panels.
type detailModel struct {
	session        session.Info
	activePanel    detailPanel
	messages       messagesModel
	stats          *analyzer.ContextStats
	rec            *analyzer.CleanupRecommendation
	health         *analyzer.HealthScore
	decEcon        *analyzer.DecisionEconomics
	gauge          *analyzer.VectorGauge
	branchOrigin   bool
	overviewScroll int
	overviewLines  int // total lines in last rendered overview (for nav)
	nav            navState
	help           helpModel
	statusMsg      string
	width, height  int
}

type backFromDetailMsg struct {
	branchOrigin bool
}

func newDetailModel(info session.Info) detailModel {
	msgs := newMessagesModel(info)
	m := detailModel{
		session:     info,
		activePanel: panelOverview,
		messages:    msgs,
		stats:       msgs.stats,
		rec:         msgs.recommendation,
		health:      msgs.health,
		decEcon:     analyzer.ComputeDecisionEconomics(msgs.stats, msgs.driftResult),
	}
	m.gauge = analyzer.ComputeGauge(msgs.stats, m.decEcon, analyzer.DefaultGaugeThresholds)
	m.overviewLines = m.countOverviewLines()
	return m
}

// countOverviewLines computes total lines in overview for navigation.
func (m detailModel) countOverviewLines() int {
	count := 3 // context meter + health + blank
	if m.gauge != nil && m.gauge.State != analyzer.VectorHealthy {
		count += 2 // gauge line + blank
	}
	if m.stats != nil && m.stats.Cost != nil {
		count += 2 // cost + blank
		if len(m.stats.Cost.PerModel) > 1 {
			count += len(m.stats.Cost.PerModel)
		}
	}
	if m.decEcon != nil && m.decEcon.HasDecisions {
		count += 4 // header + CPD + TTC + blank (CDR adds 1-2 more)
		if m.decEcon.CDR > 0 {
			count++
		}
		if m.decEcon.CDR > 0.35 {
			count++
		}
	}
	if m.stats != nil && m.stats.CompactionCount > 0 {
		count += len(m.stats.EpochCosts) + 2 // header + epochs + blank
	}
	if m.rec != nil && len(m.rec.Items) > 0 {
		count += len(m.rec.Items) + 3 // header + items + total + blank
	}
	if m.stats != nil && m.stats.GhostReport != nil && len(m.stats.GhostReport.Files) > 0 {
		count += 3
	}
	if m.stats != nil && m.stats.InputPurity != nil && m.stats.InputPurity.TotalResultTokens > 0 {
		count++
	}
	count += 2 // footer hint
	return count
}

// reload re-creates the detail model from the session, preserving panel state.
func (m detailModel) reload() detailModel {
	newDetail := newDetailModel(m.session)
	newDetail.activePanel = m.activePanel
	newDetail.branchOrigin = m.branchOrigin
	newDetail.width = m.width
	newDetail.height = m.height
	newDetail.statusMsg = m.statusMsg
	return newDetail
}

// showCleanConfirmMsg triggers a confirmation dialog for clean --all.
type showCleanConfirmMsg struct {
	rec *analyzer.CleanupRecommendation
}

// showCoalesceConfirmMsg triggers a confirmation dialog for coalesce.
type showCoalesceConfirmMsg struct{}

// showUndoConfirmMsg triggers a confirmation dialog for undo.
type showUndoConfirmMsg struct {
	hasBak bool
}

func (m detailModel) Init() tea.Cmd {
	return nil
}

func (m detailModel) Update(msg tea.Msg) (detailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Tab switching (works in all panels)
		switch {
		case msg.String() == "tab":
			m.activePanel = (m.activePanel + 1) % 4
			return m, nil
		case msg.String() == "shift+tab":
			m.activePanel = (m.activePanel + 3) % 4
			return m, nil
		case msg.String() == "1":
			if m.activePanel != panelMessages { // don't intercept '1' in messages
				m.activePanel = panelOverview
				return m, nil
			}
		case msg.String() == "2":
			if m.activePanel != panelMessages {
				m.activePanel = panelMessages
				return m, nil
			}
		case msg.String() == "3":
			if m.activePanel != panelMessages {
				m.activePanel = panelCleanup
				return m, nil
			}
		case msg.String() == "4":
			if m.activePanel != panelMessages {
				m.activePanel = panelGhost
				return m, nil
			}
		case key.Matches(msg, keys.Escape):
			if m.activePanel == panelMessages {
				if m.messages.amputateMode {
					// Let messages handle amputate cancel
					break
				}
				if m.branchOrigin {
					// From branch drill-in, go back to branches
					return m, func() tea.Msg {
						return backFromDetailMsg{branchOrigin: true}
					}
				}
				m.activePanel = panelOverview
				return m, nil
			}
			// Esc from other panels → back to sessions
			return m, func() tea.Msg {
				return backFromDetailMsg{branchOrigin: m.branchOrigin}
			}
		}

		// Delegate to active panel
		switch m.activePanel {
		case panelOverview:
			return m.updateOverview(msg)
		case panelMessages:
			// Intercept 'q' — branch origin goes back to branches, else overview
			if msg.String() == "q" {
				if m.branchOrigin {
					return m, func() tea.Msg {
						return backFromDetailMsg{branchOrigin: true}
					}
				}
				m.activePanel = panelOverview
				return m, nil
			}
			var cmd tea.Cmd
			m.messages, cmd = m.messages.Update(msg)
			return m, cmd
		case panelCleanup:
			switch {
			case key.Matches(msg, keys.Help):
				m.help.width = m.width
				m.help.height = m.height
				m.help.toggle("Cleanup", cleanupHelp())
			case msg.String() == "c":
				return m, func() tea.Msg {
					return showCleanConfirmMsg{rec: m.rec}
				}
			case msg.String() == "o":
				return m, func() tea.Msg {
					return showCoalesceConfirmMsg{}
				}
			case msg.String() == "u":
				return m, func() tea.Msg {
					return showUndoConfirmMsg{hasBak: safecopy.Exists(m.session.FullPath)}
				}
			}
			return m, nil
		case panelGhost:
			if key.Matches(msg, keys.Help) {
				m.help.width = m.width
				m.help.height = m.height
				m.help.toggle("Detail", overviewHelp())
			}
			return m, nil
		}
	}

	// Non-key messages: delegate to messages if active
	if m.activePanel == panelMessages {
		var cmd tea.Cmd
		m.messages, cmd = m.messages.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m detailModel) updateOverview(msg tea.KeyMsg) (detailModel, tea.Cmd) {
	// Help overlay.
	if m.help.visible {
		if key.Matches(msg, keys.Help) || key.Matches(msg, keys.Escape) {
			m.help.dismiss()
		}
		return m, nil
	}

	panelHeight := m.height - 4
	if panelHeight < 3 {
		panelHeight = 3
	}

	// Vim navigation for scroll-based panels (gg enabled — no g conflict in overview).
	if action := m.nav.handleVimNav(msg, true); action != navNone {
		total := m.overviewLines
		if total < 1 {
			total = 1
		}
		m.overviewScroll, _ = applyNavAction(action, m.overviewScroll, 0, total, panelHeight)
		if m.overviewScroll < 0 {
			m.overviewScroll = 0
		}
		return m, nil
	}

	// Space = page down for overview.
	if key.Matches(msg, keys.Space) {
		total := m.overviewLines
		if total < 1 {
			total = 1
		}
		m.overviewScroll, _ = applyNavAction(navPageDown, m.overviewScroll, 0, total, panelHeight)
		return m, nil
	}

	switch {
	case key.Matches(msg, keys.Down):
		m.overviewScroll++
	case key.Matches(msg, keys.Up):
		if m.overviewScroll > 0 {
			m.overviewScroll--
		}
	case key.Matches(msg, keys.Enter):
		// Enter on overview → switch to messages
		m.activePanel = panelMessages
	case key.Matches(msg, keys.Help):
		m.help.width = m.width
		m.help.height = m.height
		m.help.toggle("Overview", overviewHelp())
	}
	return m, nil
}

func (m detailModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Title bar
	title := fmt.Sprintf(" %s (%s) | %s",
		m.session.Slug, m.session.ShortID(), m.session.ProjectName)
	if m.session.IsActive() {
		title += " " + lipgloss.NewStyle().Foreground(colorYellow).Render("[ACTIVE]")
	}
	b.WriteString(styleTitle.Render(title))
	b.WriteString("\n")

	// Tab bar
	b.WriteString(m.renderTabBar())
	b.WriteString("\n")
	b.WriteString(styleMuted.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")

	// Panel content
	panelHeight := m.height - 4 // title + tab bar + separator + footer
	if panelHeight < 3 {
		panelHeight = 3
	}

	switch m.activePanel {
	case panelOverview:
		b.WriteString(m.renderOverview(panelHeight))
	case panelMessages:
		b.WriteString(m.messages.View())
	case panelCleanup:
		b.WriteString(m.renderCleanup(panelHeight))
	case panelGhost:
		b.WriteString(m.renderGhost(panelHeight))
	}

	view := b.String()
	if m.help.visible {
		return m.help.View()
	}
	return view
}

func (m detailModel) renderTabBar() string {
	var tabs []string
	for i, name := range panelNames {
		if detailPanel(i) == m.activePanel {
			tabs = append(tabs, lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent).
				Render(fmt.Sprintf(" [%s] ", name)))
		} else {
			tabs = append(tabs, styleMuted.Render(fmt.Sprintf("  %s  ", name)))
		}
	}
	return strings.Join(tabs, "")
}

func (m detailModel) renderOverview(height int) string {
	var lines []string

	stats := m.stats
	if stats == nil {
		lines = append(lines, " No analysis data available.")
		return m.scrolledContent(lines, height)
	}

	// Vector gauge (when not healthy)
	if m.gauge != nil && m.gauge.State != analyzer.VectorHealthy {
		stateColor := gaugeStateColor(m.gauge.State)
		stateStyle := lipgloss.NewStyle().Foreground(stateColor).Bold(true)
		lines = append(lines, fmt.Sprintf(" Vector: %s — %s",
			stateStyle.Render(string(m.gauge.State)),
			styleMuted.Render(string(m.gauge.Action))))
		lines = append(lines, "")
	}

	// Context meter
	pct := stats.UsagePercent
	barW := 20
	var bar string
	if stats.CompactionCount > 0 {
		bar = contextBarStrCompacted(pct, barW)
		lines = append(lines, fmt.Sprintf(" Context: %s %.1f%%  (%d compactions)",
			bar, pct, stats.CompactionCount))
	} else {
		bar = contextBarStr(pct, barW)
		lines = append(lines, fmt.Sprintf(" Context: %s %.1f%%", bar, pct))
	}

	// Health + signal
	if m.health != nil {
		grade := m.health.Grade
		gradeStr := gradeStyle(grade).Render(grade)
		turnsLeft := "—"
		if stats.EstimatedTurnsLeft >= 0 {
			turnsLeft = fmt.Sprintf("%d", stats.EstimatedTurnsLeft)
		}
		lines = append(lines, fmt.Sprintf(" Health: %s  Signal: %.0f%%  Turns: %d  Est. left: %s",
			gradeStr, m.health.SignalPercent,
			stats.ConversationalTurns, turnsLeft))
	}

	if stats.InputPurity != nil && stats.InputPurity.TotalResultTokens > 0 {
		lines = append(lines, fmt.Sprintf(" Input purity: %.0f%% (%.0f%% compressible)",
			stats.InputPurity.Score, 100-stats.InputPurity.Score))
	}

	lines = append(lines, "")

	// Cost summary
	if stats.Cost != nil {
		costLine := fmt.Sprintf(" Cost: %s", analyzer.FormatCost(stats.Cost.TotalCost))
		if stats.Cost.TurnCount > 0 {
			costLine += fmt.Sprintf("  (%s/turn)", analyzer.FormatCost(stats.Cost.CostPerTurn))
		}
		lines = append(lines, costLine)

		// Per-model breakdown
		if len(stats.Cost.PerModel) > 1 {
			for model, pm := range stats.Cost.PerModel {
				pricing := analyzer.PricingForModel(model)
				lines = append(lines, fmt.Sprintf("   %s: %s (%d turns)",
					pricing.Name, analyzer.FormatCost(pm.TotalCost), pm.TurnCount))
			}
		}
	}

	// Decision economics
	if m.decEcon != nil && m.decEcon.HasDecisions {
		lines = append(lines, styleHeader.Render(" Decision Economics"))
		lines = append(lines, fmt.Sprintf("   CPD: %s/decision (%d decisions)",
			analyzer.FormatCost(m.decEcon.CPD), m.decEcon.TotalDecisions))
		lines = append(lines, fmt.Sprintf("   TTC: %d turns/decision", m.decEcon.TTC))
		if m.decEcon.CDR > 0 {
			lines = append(lines, fmt.Sprintf("   CDR: %.0f%%", m.decEcon.CDR*100))
		}
		if m.decEcon.CDR > 0.35 {
			lines = append(lines, styleMuted.Render("   CDR > 35% — consider splitting session"))
		}
		lines = append(lines, "")
	}

	// Compaction epochs
	if stats.CompactionCount > 0 && len(stats.EpochCosts) > 0 {
		lines = append(lines, styleHeader.Render(" Compaction Epochs"))
		for _, ec := range stats.EpochCosts {
			peakPct := float64(ec.PeakTokens) / float64(analyzer.ContextWindowSize) * 100
			lines = append(lines, fmt.Sprintf("   Epoch %d: %d turns, peak %.0f%%, %s",
				ec.EpochIndex, ec.TurnCount, peakPct, analyzer.FormatCost(ec.Cost.TotalCost)))
		}
		lines = append(lines, "")
	}

	// Cleanup recommendations
	if m.rec != nil && len(m.rec.Items) > 0 {
		lines = append(lines, styleHeader.Render(" Cleanup Recoverable"))
		for _, item := range m.rec.Items {
			turnsStr := ""
			if item.TurnsGained > 0 {
				turnsStr = fmt.Sprintf(" (+%d turns)", item.TurnsGained)
			}
			lines = append(lines, fmt.Sprintf("   %-18s %d items  ~%s tokens%s",
				item.Label, item.Count, formatTokensShort(item.TokensSaved), turnsStr))
		}
		lines = append(lines, fmt.Sprintf("   Total: ~%s tokens  %.1f%% → %.1f%%",
			formatTokensShort(m.rec.TotalTokens), m.rec.CurrentPercent, m.rec.ProjectedPercent))
		lines = append(lines, "")
	}

	// Ghost context summary
	if stats.GhostReport != nil && len(stats.GhostReport.Files) > 0 {
		lines = append(lines, styleHeader.Render(fmt.Sprintf(" Ghost Context: %d files",
			len(stats.GhostReport.Files))))
		lines = append(lines, styleMuted.Render("   (Switch to Ghost tab for details)"))
		lines = append(lines, "")
	}

	// Footer hint
	lines = append(lines, "")
	lines = append(lines, styleMuted.Render(" Enter: messages  Tab: switch panels  Esc: back"))

	return m.scrolledContent(lines, height)
}

func (m detailModel) renderCleanup(height int) string {
	var lines []string

	if m.statusMsg != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(colorGreen).Render(" "+m.statusMsg))
		lines = append(lines, "")
	}

	if m.rec == nil || len(m.rec.Items) == 0 {
		lines = append(lines, " Nothing to clean.")
		lines = append(lines, "")
		lines = append(lines, styleMuted.Render(" c clean  o coalesce  u undo  Tab panels  ? help"))
		return m.scrolledContent(lines, height)
	}

	lines = append(lines, styleHeader.Render(" Noise Breakdown"))
	lines = append(lines, "")

	for _, item := range m.rec.Items {
		turnsStr := ""
		if item.TurnsGained > 0 {
			turnsStr = fmt.Sprintf(" (+%d turns)", item.TurnsGained)
		}
		lines = append(lines, fmt.Sprintf("   %-18s %3d items  ~%6s tokens%s",
			item.Label, item.Count, formatTokensShort(item.TokensSaved), turnsStr))
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("   Total recoverable: ~%s tokens", formatTokensShort(m.rec.TotalTokens)))
	lines = append(lines, fmt.Sprintf("   Projected: %.1f%% → %.1f%%", m.rec.CurrentPercent, m.rec.ProjectedPercent))

	if m.stats != nil && m.stats.EstimatedTurnsLeft > 0 && m.rec.TotalTokens > 0 {
		pricing := analyzer.PricingForModel(m.stats.Model)
		avoided := m.rec.TotalTokens * m.stats.EstimatedTurnsLeft
		cost := float64(avoided) / 1_000_000 * pricing.CacheReadPerMillion
		lines = append(lines, fmt.Sprintf("   Projected savings: ~%s cache-read tokens (~%s)",
			formatTokensShort(avoided), analyzer.FormatCost(cost)))
	}

	lines = append(lines, "")
	lines = append(lines, styleMuted.Render(" c clean  o coalesce  u undo  Tab panels  ? help"))

	return m.scrolledContent(lines, height)
}

func (m detailModel) renderGhost(height int) string {
	var lines []string

	if m.stats == nil || m.stats.GhostReport == nil || len(m.stats.GhostReport.Files) == 0 {
		lines = append(lines, " No ghost context detected.")
		lines = append(lines, "")
		lines = append(lines, styleMuted.Render(" Tab: switch panels  Esc: back"))
		return m.scrolledContent(lines, height)
	}

	gr := m.stats.GhostReport
	lines = append(lines, styleHeader.Render(fmt.Sprintf(" Ghost Context: %d files referenced but lost to compaction",
		len(gr.Files))))
	lines = append(lines, "")

	for _, f := range gr.Files {
		lines = append(lines, fmt.Sprintf("   %s", styleGhost.Render(f.Path)))
		lines = append(lines, fmt.Sprintf("     Compacted at epoch %d, modified in epoch %d",
			f.CompactionIndex, f.EpochModified))
	}

	lines = append(lines, "")
	lines = append(lines, styleMuted.Render(" Tab: switch panels  Esc: back"))

	return m.scrolledContent(lines, height)
}

// scrolledContent renders lines with scroll support.
func (m detailModel) scrolledContent(lines []string, height int) string {
	start := m.overviewScroll
	if start >= len(lines) {
		start = len(lines) - 1
	}
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

// formatTokensShort is defined in messages.go
