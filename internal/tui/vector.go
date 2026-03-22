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
	"github.com/ppiankov/contextspectre/internal/savings"
	"github.com/ppiankov/contextspectre/internal/session"
)

// vectorModel is the TUI model for the Vector Control panel.
type vectorModel struct {
	session session.Info
	entries []jsonl.Entry
	stats   *analyzer.ContextStats
	rec     *analyzer.CleanupRecommendation
	health  *analyzer.HealthScore
	decEcon *analyzer.DecisionEconomics
	gauge   *analyzer.VectorGauge
	cadence *analyzer.CadenceAssessment
	drift   *analyzer.ScopeDrift
	savings *savings.Summary

	counterfactualTokens int
	counterfactualPct    float64
	pastCleanups         int

	cleanResult string
	cleanErr    error
	statusMsg   string

	scroll     int
	totalLines int
	nav        navState
	help       helpModel
	claudeDir  string
	width      int
	height     int
}

type openVectorMsg struct {
	info session.Info
}

type backFromVectorMsg struct{}

type vectorCleanDoneMsg struct {
	result string
	err    error
}

func newVectorModel(info session.Info, claudeDir string) vectorModel {
	entries, err := jsonl.Parse(info.FullPath)
	if err != nil {
		return vectorModel{
			session:   info,
			claudeDir: claudeDir,
			statusMsg: fmt.Sprintf("Error: %v", err),
		}
	}

	stats := analyzer.Analyze(entries)
	dupResult := analyzer.FindDuplicateReads(entries)
	retryResult := analyzer.FindFailedRetries(entries)
	tangentResult := analyzer.FindTangents(entries)
	driftResult := analyzer.AnalyzeScopeDrift(entries, stats.Compactions, "")

	rec := analyzer.Recommend(entries, stats, dupResult, retryResult, tangentResult, nil)
	health := analyzer.ComputeHealth(stats, rec)
	decEcon := analyzer.ComputeDecisionEconomics(stats, driftResult)
	gauge := analyzer.ComputeGauge(stats, decEcon, analyzer.DefaultGaugeThresholds)
	cadence := analyzer.AssessCleanupCadence(stats, rec)

	allEvents, _ := savings.Load(claudeDir)
	sessionSavings := savings.ForSession(allEvents, info.SessionID)

	cfTokens := stats.CurrentContextTokens
	if sessionSavings != nil {
		cfTokens += sessionSavings.TotalRemoved
	}
	cfPct := float64(cfTokens) / float64(analyzer.ContextWindowSize) * 100

	pastCleanups := 0
	if sessionSavings != nil {
		pastCleanups = sessionSavings.TotalCleanups
	}

	m := vectorModel{
		session:              info,
		entries:              entries,
		stats:                stats,
		rec:                  rec,
		health:               health,
		decEcon:              decEcon,
		gauge:                gauge,
		cadence:              cadence,
		drift:                driftResult,
		savings:              sessionSavings,
		counterfactualTokens: cfTokens,
		counterfactualPct:    cfPct,
		pastCleanups:         pastCleanups,
		claudeDir:            claudeDir,
	}
	return m
}

func (m vectorModel) Init() tea.Cmd {
	return nil
}

func (m vectorModel) Update(msg tea.Msg) (vectorModel, tea.Cmd) {
	switch msg := msg.(type) {
	case vectorCleanDoneMsg:
		if msg.err != nil {
			m.cleanErr = msg.err
			m.cleanResult = ""
		} else {
			m.cleanResult = msg.result
			m.cleanErr = nil
			// Reload after clean
			reloaded := newVectorModel(m.session, m.claudeDir)
			reloaded.width = m.width
			reloaded.height = m.height
			reloaded.cleanResult = m.cleanResult
			return reloaded, nil
		}
		return m, nil

	case tea.KeyMsg:
		// Help overlay
		if m.help.visible {
			if key.Matches(msg, keys.Help) || key.Matches(msg, keys.Escape) {
				m.help.dismiss()
			}
			return m, nil
		}

		panelHeight := m.panelHeight()

		// Vim navigation (gg enabled)
		if action := m.nav.handleVimNav(msg, true); action != navNone {
			total := m.totalLines
			if total < 1 {
				total = 1
			}
			m.scroll, _ = applyNavAction(action, m.scroll, 0, total, panelHeight)
			if m.scroll < 0 {
				m.scroll = 0
			}
			return m, nil
		}

		// Space = page down
		if key.Matches(msg, keys.Space) {
			total := m.totalLines
			if total < 1 {
				total = 1
			}
			m.scroll, _ = applyNavAction(navPageDown, m.scroll, 0, total, panelHeight)
			return m, nil
		}

		switch {
		case key.Matches(msg, keys.Down):
			m.scroll++
		case key.Matches(msg, keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
		case key.Matches(msg, keys.Escape):
			return m, func() tea.Msg { return backFromVectorMsg{} }
		case msg.String() == "q":
			return m, func() tea.Msg { return backFromVectorMsg{} }

		// C = Clean
		case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'C':
			return m, m.runClean()

		// S = Split tangent
		case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'S':
			return m, m.runSplit()

		// E = Export (placeholder)
		case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'E':
			m.cleanResult = "Export: coming in a future release"
			return m, nil

		case key.Matches(msg, keys.Help):
			m.help.width = m.width
			m.help.height = m.height
			m.help.toggle("Vector Control", vectorHelp())
			return m, nil
		}
	}
	return m, nil
}

func (m vectorModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Title bar
	title := fmt.Sprintf(" Vector Control: %s (%s)",
		m.session.DisplayName(), m.session.ShortID())
	if m.session.IsActive() {
		title += " " + lipgloss.NewStyle().Foreground(colorYellow).Render("[ACTIVE]")
	}
	b.WriteString(styleTitle.Render(title))
	b.WriteString("\n")
	b.WriteString(styleMuted.Render(strings.Repeat("\u2500", m.width)))
	b.WriteString("\n")

	if m.statusMsg != "" {
		b.WriteString(" " + m.statusMsg + "\n")
		return b.String()
	}

	// Assemble all lines
	var lines []string
	lines = append(lines, m.renderNow()...)
	lines = append(lines, m.renderWhatIf()...)
	lines = append(lines, m.renderIfClean()...)
	lines = append(lines, m.renderSuggestion()...)

	// Footer
	lines = append(lines, "")
	footerParts := []string{"C clean", "Esc back", "? help"}
	if m.drift != nil && len(m.drift.TangentSeqs) > 0 {
		footerParts = append([]string{"C clean", "S split"}, footerParts[1:]...)
	}
	lines = append(lines, styleMuted.Render(" "+strings.Join(footerParts, "  ")))

	m.totalLines = len(lines)

	panelHeight := m.panelHeight()
	b.WriteString(m.scrolledView(lines, panelHeight))

	if m.help.visible {
		return m.help.View()
	}
	return b.String()
}

func (m vectorModel) panelHeight() int {
	h := m.height - 3 // title + separator + margin
	if h < 3 {
		h = 3
	}
	return h
}

func (m vectorModel) scrolledView(lines []string, height int) string {
	start := m.scroll
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

// --- Section Renderers ---

func (m vectorModel) renderNow() []string {
	var lines []string
	lines = append(lines, styleHeader.Render(" NOW"))
	lines = append(lines, "")

	if m.stats == nil {
		lines = append(lines, "   No analysis data available.")
		lines = append(lines, "")
		return lines
	}

	// Dual context bars
	pct := m.stats.UsagePercent
	barW := 20
	if m.stats.CompactionCount > 0 {
		bar := contextBarStrCompacted(pct, barW)
		lines = append(lines, fmt.Sprintf("   Actual:          %s %5.1f%%  (%d compactions)",
			bar, pct, m.stats.CompactionCount))
	} else {
		bar := contextBarStr(pct, barW)
		lines = append(lines, fmt.Sprintf("   Actual:          %s %5.1f%%",
			bar, pct))
	}
	if m.pastCleanups > 0 {
		cfBar := counterfactualBarStr(m.counterfactualPct, barW)
		lines = append(lines, fmt.Sprintf("   Without cleanup: %s %5.1f%%",
			cfBar, m.counterfactualPct))
	}
	lines = append(lines, "")

	// Compaction ETA
	if m.stats.EstimatedTurnsLeft >= 0 {
		lines = append(lines, fmt.Sprintf("   Compaction ETA:    %d turns", m.stats.EstimatedTurnsLeft))
	} else {
		lines = append(lines, "   Compaction ETA:    \u2014")
	}

	// Signal grade
	if m.health != nil {
		grade := gradeStyle(m.health.Grade).Render(m.health.Grade)
		lines = append(lines, fmt.Sprintf("   Signal:            %s (%.0f%% signal, %.0f%% noise)",
			grade, m.health.SignalPercent, m.health.NoisePercent))
		if m.health.BiggestOffender != "" {
			lines = append(lines, fmt.Sprintf("   Biggest offender:  %s (%s tokens)",
				m.health.BiggestOffender,
				formatTokensShort(m.health.OffenderTokens)))
		}
	}

	// Cost
	if m.stats.Cost != nil {
		costLine := fmt.Sprintf("   Cost:              %s", analyzer.FormatCost(m.stats.Cost.TotalCost))
		if m.stats.Cost.TurnCount > 0 {
			costLine += fmt.Sprintf("  (%s/turn)", analyzer.FormatCost(m.stats.Cost.CostPerTurn))
		}
		lines = append(lines, costLine)
	}

	// Scope drift
	if m.drift != nil && m.drift.OverallDrift > 0 {
		lines = append(lines, fmt.Sprintf("   Scope drift:       %.0f%%", m.drift.OverallDrift*100))
	}

	// Decision economics (compact single line)
	if m.decEcon != nil && m.decEcon.HasDecisions {
		lines = append(lines, fmt.Sprintf("   Decisions:         CPD %s  TTC %d  CDR %.0f%%",
			analyzer.FormatCost(m.decEcon.CPD), m.decEcon.TTC, m.decEcon.CDR*100))
	}

	// Vector gauge
	if m.gauge != nil && m.gauge.State != analyzer.VectorHealthy {
		stateColor := gaugeStateColor(m.gauge.State)
		stateStyle := lipgloss.NewStyle().Foreground(stateColor).Bold(true)
		lines = append(lines, fmt.Sprintf("   Vector:            %s \u2014 %s",
			stateStyle.Render(string(m.gauge.State)),
			styleMuted.Render(string(m.gauge.Action))))
	}

	lines = append(lines, "")
	return lines
}

func (m vectorModel) renderWhatIf() []string {
	var lines []string
	lines = append(lines, styleHeader.Render(" WHAT-IF (no cleaning)"))
	lines = append(lines, "")

	if m.stats == nil {
		lines = append(lines, "   No analysis data.")
		lines = append(lines, "")
		return lines
	}

	// Compaction projection
	if m.stats.EstimatedTurnsLeft >= 0 {
		lines = append(lines, fmt.Sprintf("   You'll compact in ~%d turns", m.stats.EstimatedTurnsLeft))
	} else {
		lines = append(lines, "   No compaction pressure detected")
	}

	// Wasted cache-read until compaction
	if m.cadence != nil && m.cadence.ProjectedSaveTokens > 0 {
		lines = append(lines, fmt.Sprintf("   Wasted cache-read until compaction: %s tokens (%s)",
			formatTokensShort(m.cadence.ProjectedSaveTokens),
			analyzer.FormatCost(m.cadence.ProjectedSaveCost)))
		lines = append(lines, fmt.Sprintf("   Per-turn noise waste: %s tokens (%s)",
			formatTokensShort(m.cadence.NoiseTokens),
			analyzer.FormatCost(m.cadence.PerTurnSaveCost)))
	}

	// Top offenders
	if m.rec != nil && len(m.rec.Items) > 0 {
		lines = append(lines, "")
		lines = append(lines, "   Top offenders:")
		limit := 3
		if len(m.rec.Items) < limit {
			limit = len(m.rec.Items)
		}
		for _, item := range m.rec.Items[:limit] {
			lines = append(lines, fmt.Sprintf("     %s: %d items, %s tokens",
				item.Label, item.Count, formatTokensShort(item.TokensSaved)))
		}
	}

	// Counterfactual
	if m.pastCleanups > 0 && m.savings != nil && m.savings.TotalRemoved > 0 {
		lines = append(lines, "")
		if m.counterfactualPct > 82.5 {
			lines = append(lines, fmt.Sprintf("   Without your %d cleanups, you'd be at %.0f%%",
				m.pastCleanups, m.counterfactualPct))
			if m.counterfactualPct > 100 {
				lines = append(lines, styleWarning.Render(
					"   Compaction would have triggered already!"))
			}
		} else {
			lines = append(lines, fmt.Sprintf("   Your %d cleanups removed %s tokens (%s saved)",
				m.pastCleanups,
				formatTokensShort(m.savings.TotalRemoved),
				analyzer.FormatCost(m.savings.TotalSavedCost)))
		}
	}

	lines = append(lines, "")
	return lines
}

func (m vectorModel) renderIfClean() []string {
	var lines []string
	lines = append(lines, styleHeader.Render(" IF CLEAN NOW"))
	lines = append(lines, "")

	if m.rec == nil || m.rec.TotalTokens == 0 {
		lines = append(lines, "   Nothing to clean. Context is signal-pure.")
		lines = append(lines, "")
		return lines
	}

	// Projected context after cleanup
	projBar := contextBarStr(m.rec.ProjectedPercent, 20)
	lines = append(lines, fmt.Sprintf("   Projected:       %s %5.1f%%  (from %.1f%%)",
		projBar, m.rec.ProjectedPercent, m.rec.CurrentPercent))

	// Turns gained
	if m.rec.TotalTurnsGained > 0 {
		lines = append(lines, fmt.Sprintf("   Turns gained:    +%d", m.rec.TotalTurnsGained))
	}

	// Tokens and dollar savings
	lines = append(lines, fmt.Sprintf("   Tokens recovered: %s", formatTokensShort(m.rec.TotalTokens)))

	turnsLeft := m.stats.EstimatedTurnsLeft
	if turnsLeft < 0 {
		turnsLeft = 0
	}
	if turnsLeft > 0 {
		pricing := analyzer.PricingForModel(m.stats.Model)
		projectedSaveTokens := m.rec.TotalTokens * turnsLeft
		projectedSaveCost := float64(projectedSaveTokens) / 1_000_000 * pricing.CacheReadPerMillion
		lines = append(lines, fmt.Sprintf("   Projected savings: %s cache-read tokens (%s)",
			formatTokensShort(projectedSaveTokens),
			analyzer.FormatCost(projectedSaveCost)))
	}

	// Breakdown
	lines = append(lines, "")
	for _, item := range m.rec.Items {
		lines = append(lines, fmt.Sprintf("     %-18s %s tokens (%d items)",
			item.Label, formatTokensShort(item.TokensSaved), item.Count))
	}
	lines = append(lines, "")

	// Action prompt
	actionStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	if m.session.IsActive() {
		lines = append(lines, actionStyle.Render("   Press C to clean (live mode, tier 1-3)"))
	} else {
		lines = append(lines, actionStyle.Render("   Press C to clean (full, all tiers)"))
	}
	lines = append(lines, "")

	return lines
}

func (m vectorModel) renderSuggestion() []string {
	var lines []string

	// Cadence suggestion
	if m.cadence != nil && m.cadence.Status != analyzer.CadenceClean {
		var style lipgloss.Style
		switch m.cadence.Status {
		case analyzer.CadenceOverdue:
			style = styleWarning
		default:
			style = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
		}
		lines = append(lines, style.Render(fmt.Sprintf(" %s: %s",
			cadenceStatusLabel(m.cadence.Status), m.cadence.Reason)))
	}

	// Clean result (inline after action)
	if m.cleanResult != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(colorGreen).Render(
			" "+m.cleanResult))
	}
	if m.cleanErr != nil {
		lines = append(lines, "")
		lines = append(lines, styleWarning.Render(
			fmt.Sprintf(" Clean error: %v", m.cleanErr)))
	}

	return lines
}

// --- Action Handlers ---

func (m vectorModel) runClean() tea.Cmd {
	path := m.session.FullPath
	isActive := m.session.IsActive()

	return func() tea.Msg {
		if isActive {
			result, err := editor.CleanLive(path, editor.CleanLiveOpts{Tier3: true})
			if err != nil {
				return vectorCleanDoneMsg{err: err}
			}
			tokensSaved := result.TotalTokensSaved
			if tokensSaved < 0 {
				tokensSaved = 0
			}
			msg := fmt.Sprintf("Cleaned (live): %d prog, %d snap, %d stale, %d retry \u2014 %s tokens saved",
				result.ProgressRemoved, result.SnapshotsRemoved,
				result.StaleReadsRemoved, result.FailedRetries,
				formatTokensShort(tokensSaved))
			return vectorCleanDoneMsg{result: msg}
		}

		result, err := editor.CleanAll(path, editor.CleanAllOpts{})
		if err != nil {
			return vectorCleanDoneMsg{err: err}
		}
		tokensSaved := result.TotalTokensSaved
		if tokensSaved < 0 {
			tokensSaved = 0
		}
		msg := fmt.Sprintf("Cleaned (full): %d prog, %d snap, %d stale, %d tangent, %d orphan \u2014 %s tokens saved",
			result.ProgressRemoved, result.SnapshotsRemoved,
			result.StaleReadsRemoved, result.TangentsRemoved, result.OrphansRemoved,
			formatTokensShort(tokensSaved))
		return vectorCleanDoneMsg{result: msg}
	}
}

func (m vectorModel) runSplit() tea.Cmd {
	if m.drift == nil || len(m.drift.TangentSeqs) == 0 {
		return func() tea.Msg {
			return vectorCleanDoneMsg{result: "No tangent sequences detected"}
		}
	}

	ts := m.drift.TangentSeqs[0]
	entries := m.entries
	sessionID := m.session.SessionID
	cwd := m.drift.SessionProject

	return func() tea.Msg {
		meta := analyzer.ComputeRangeMetadata(entries, ts.StartIdx, ts.EndIdx, cwd)
		outputPath := fmt.Sprintf("tangent-%s-%d-%d.md",
			sessionID[:8], ts.StartIdx, ts.EndIdx)

		_, err := editor.SplitToMarkdown(entries, ts.StartIdx, ts.EndIdx,
			meta, sessionID, outputPath)
		if err != nil {
			return vectorCleanDoneMsg{err: err}
		}
		msg := fmt.Sprintf("Split tangent to %s: entries %d-%d (%s tokens)",
			outputPath, ts.StartIdx, ts.EndIdx,
			formatTokensShort(meta.TokenCost))
		return vectorCleanDoneMsg{result: msg}
	}
}

// --- Helpers ---

func cadenceStatusLabel(status analyzer.CadenceStatus) string {
	switch status {
	case analyzer.CadenceOverdue:
		return "!!"
	case analyzer.CadenceDue:
		return "!"
	default:
		return "OK"
	}
}

func counterfactualBarStr(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	filledStr := lipgloss.NewStyle().Foreground(colorRed).Render(strings.Repeat("\u2588", filled))
	emptyStr := styleMuted.Render(strings.Repeat("\u2591", width-filled))
	return filledStr + emptyStr
}
