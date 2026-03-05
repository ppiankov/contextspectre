package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sort"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/savings"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	statsCmd = &cobra.Command{
		Use:   "stats [session-id-or-path]",
		Short: "Show context statistics for a session",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runStats,
	}
	showEpochs  bool
	showScope   bool
	statsCWD    bool
	statsRecord bool
	statsHealth bool
)

func runStats(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, statsCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	stats := analyzer.Analyze(entries)
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	// Structural sidechain detection (not just explicit isSidechain flags).
	sidechainReport := analyzer.DetectSidechains(entries)
	stats.SidechainCount = sidechainReport.TotalEntries
	stats.SidechainGroups = sidechainReport.GroupCount
	stats.SidechainTokens = sidechainReport.TotalTokens

	// Compute cleanup recommendations
	dupResult := analyzer.FindDuplicateReads(entries)
	retryResult := analyzer.FindFailedRetries(entries)
	tangentResult := analyzer.FindTangents(entries)
	rec := analyzer.Recommend(stats, dupResult, retryResult, tangentResult)

	// Scope drift analysis
	driftResult := analyzer.AnalyzeScopeDrift(entries, stats.Compactions, "")
	health := analyzer.ComputeHealth(stats, rec)
	signalRatio := analyzer.SignalRatioForGrade("A")
	if health != nil {
		signalRatio = analyzer.SignalRatioForGrade(health.Grade)
	}
	entropy := analyzer.CalculateEntropy(analyzer.EntropyInput{
		SignalRatio:     signalRatio,
		CurrentTokens:   stats.CurrentContextTokens,
		DriftRatio:      driftResult.OverallDrift,
		OrphanTokens:    stats.SidechainTokens,
		TotalTokens:     stats.CurrentContextTokens,
		CompactionCount: stats.CompactionCount,
	})

	duration := sessionDuration(entries)

	if isJSON() {
		threshold := loadCostAlertThreshold()
		decEconJSON := analyzer.ComputeDecisionEconomics(stats, driftResult)
		thresholds := loadGaugeThresholds()
		gauge := analyzer.ComputeGauge(stats, decEconJSON, thresholds)
		out := buildStatsOutput(sessionID, stats, rec, driftResult, statsOutputOpt{
			duration:           duration,
			costAlertThreshold: threshold,
			decisionEconomics:  decEconJSON,
			vectorGauge:        gauge,
			entropy:            &entropy,
		})
		return printJSON(out)
	}

	fi, _ := os.Stat(path)

	fmt.Printf("Session: %s\n", filepath.Base(path))
	if fi != nil {
		fmt.Printf("File size: %.1f MB\n", float64(fi.Size())/1024/1024)
	}
	fmt.Printf("Total lines: %d\n", stats.TotalLines)
	if stats.ClientType != "" && stats.ClientType != "unknown" {
		fmt.Printf("Client: %s\n", stats.ClientType)
	}
	fmt.Println()

	// Message counts
	fmt.Println("Message counts:")
	for typ, count := range stats.MessageCounts {
		fmt.Printf("  %-25s %d\n", typ, count)
	}
	fmt.Println()

	// Context usage
	fmt.Println("Context usage:")
	fmt.Printf("  Current tokens:  %s / %s (%.1f%%)\n",
		formatTokens(stats.CurrentContextTokens),
		formatTokens(analyzer.ContextWindowSize),
		stats.UsagePercent)
	fmt.Printf("  Max observed:    %s\n", formatTokens(stats.MaxContextTokens))
	fmt.Printf("  Context bar:     %s\n", contextBar(stats.UsagePercent, 30))
	fmt.Println()

	// Context health
	if health != nil && health.TotalTokens > 0 {
		fmt.Println("Context health:")
		fmt.Printf("  Signal:           %.1f%% (%s)\n", health.SignalPercent, health.Grade)
		fmt.Printf("  Signal tokens:    %s\n", formatTokens(health.SignalTokens))
		fmt.Printf("  Noise tokens:     %s\n", formatTokens(health.NoiseTokens))
		if health.BiggestOffender != "" {
			fmt.Printf("  Biggest offender: %s (%s tokens)\n", health.BiggestOffender, formatTokens(health.OffenderTokens))
		}
		fmt.Printf("  Session entropy:  %s (%.1f/100)\n", entropy.Level, entropy.Score)
		fmt.Printf("  Entropy axes:     noise %.1f | pressure %.1f | drift %.1f | orphans %.1f | compression %.1f\n",
			entropy.Breakdown.Noise,
			entropy.Breakdown.CompactionPressure,
			entropy.Breakdown.Drift,
			entropy.Breakdown.Orphans,
			entropy.Breakdown.CompressionLoss)
		fmt.Println()
	}

	// Compaction info
	fmt.Printf("Compactions: %d\n", stats.CompactionCount)
	if stats.CompactionCount > 0 {
		for i, c := range stats.Compactions {
			fmt.Printf("  #%d: %s → %s (-%s)\n",
				i+1,
				formatTokens(c.BeforeTokens),
				formatTokens(c.AfterTokens),
				formatTokens(c.TokensDrop))
		}
	}
	fmt.Println()

	// Compaction archaeology
	if stats.Archaeology != nil && len(stats.Archaeology.Events) > 0 {
		for _, arch := range stats.Archaeology.Events {
			fmt.Printf("Compaction #%d archaeology:\n", arch.CompactionIndex+1)
			fmt.Printf("  Before: %d turns, %s peak, %d files, %d tool calls\n",
				arch.Before.TurnCount,
				formatTokens(arch.Before.TokensPeak),
				len(arch.Before.FilesReferenced),
				arch.Before.TotalToolCalls())

			// Top tool calls
			if arch.Before.TotalToolCalls() > 0 {
				fmt.Printf("  Tools:")
				for name, count := range arch.Before.ToolCallCounts {
					fmt.Printf(" %s(%d)", name, count)
				}
				fmt.Println()
			}

			// User questions
			if len(arch.Before.UserQuestions) > 0 {
				fmt.Printf("  Questions (%d):\n", len(arch.Before.UserQuestions))
				for j, q := range arch.Before.UserQuestions {
					if j >= 3 {
						fmt.Printf("    ... and %d more\n", len(arch.Before.UserQuestions)-3)
						break
					}
					fmt.Printf("    - %s\n", q)
				}
			}

			// Decision hints
			if len(arch.Before.DecisionHints) > 0 {
				fmt.Printf("  Decisions (%d):\n", len(arch.Before.DecisionHints))
				for j, d := range arch.Before.DecisionHints {
					if j >= 3 {
						break
					}
					fmt.Printf("    - %s\n", d)
				}
			}

			// Summary
			fmt.Printf("  After: %d chars summary, %.1fx compression\n",
				arch.After.SummaryCharCount, arch.After.CompressionRatio)
			if arch.After.SummaryText != "" {
				preview := arch.After.SummaryText
				if len(preview) > 200 {
					preview = preview[:197] + "..."
				}
				fmt.Printf("  Summary: \"%s\"\n", preview)
			}
			fmt.Println()
		}
	}

	// Ghost context warnings
	if stats.GhostReport != nil && stats.GhostReport.TotalGhosts > 0 {
		fmt.Printf("Ghost context: %d files modified after compaction summary\n", stats.GhostReport.TotalGhosts)
		for _, g := range stats.GhostReport.Files {
			fmt.Printf("  #%d → %s (modified in epoch %d)\n",
				g.CompactionIndex+1, g.Path, g.EpochModified)
		}
		fmt.Println()
	}

	// Growth and distance
	fmt.Printf("Token growth rate: ~%.0f tokens/turn\n", stats.TokenGrowthRate)
	if stats.EstimatedTurnsLeft >= 0 {
		fmt.Printf("Estimated turns until compaction: ~%d\n", stats.EstimatedTurnsLeft)
	} else {
		fmt.Println("Estimated turns until compaction: unknown")
	}
	fmt.Println()

	// Session cost
	if stats.Cost != nil && stats.Cost.TurnCount > 0 {
		fmt.Println("Session cost:")
		modelName := stats.Cost.Model
		if p := analyzer.PricingForModel(modelName); p.Name != "" {
			modelName = p.Name
		}
		if modelName != "" {
			fmt.Printf("  Model:         %s\n", modelName)
		}
		fmt.Printf("  Total:         %s (%s/turn)\n",
			analyzer.FormatCost(stats.Cost.TotalCost),
			analyzer.FormatCost(stats.Cost.CostPerTurn))

		// Cost velocity ($/hour)
		duration := sessionDuration(entries)
		if duration > 0 && stats.Cost.TotalCost > 0 {
			velocity := stats.Cost.TotalCost / duration.Hours()
			fmt.Printf("  Velocity:      %s/hour\n", analyzer.FormatCost(velocity))
		}

		// Breakdown by component, sorted by magnitude
		type costLine struct {
			label string
			cost  float64
		}
		lines := []costLine{
			{"Cache read", stats.Cost.CacheReadCost},
			{"Cache write", stats.Cost.CacheWriteCost},
			{"Output", stats.Cost.OutputCost},
			{"Input", stats.Cost.InputCost},
		}
		for _, l := range lines {
			pct := analyzer.CostPercent(l.cost, stats.Cost.TotalCost)
			if pct < 1 {
				fmt.Printf("  %-14s %s (<1%%)\n", l.label+":", analyzer.FormatCost(l.cost))
			} else {
				fmt.Printf("  %-14s %s (%.0f%%)\n", l.label+":", analyzer.FormatCost(l.cost), pct)
			}
		}
		fmt.Println()

		// Per-model breakdown (only if multiple models used)
		if len(stats.Cost.PerModel) > 1 {
			fmt.Println("Model breakdown:")
			// Sort by cost descending
			type modelLine struct {
				model string
				name  string
				turns int
				cost  float64
			}
			var mlines []modelLine
			for model, pm := range stats.Cost.PerModel {
				name := analyzer.PricingForModel(model).Name
				if name == "" {
					name = model
				}
				mlines = append(mlines, modelLine{model, name, pm.TurnCount, pm.TotalCost})
			}
			sort.Slice(mlines, func(i, j int) bool {
				return mlines[i].cost > mlines[j].cost
			})
			for _, ml := range mlines {
				pct := analyzer.CostPercent(ml.cost, stats.Cost.TotalCost)
				fmt.Printf("  %-18s %3d turns   %s  (%.0f%%)\n",
					ml.name, ml.turns, analyzer.FormatCost(ml.cost), pct)
			}
			fmt.Println()
		}

		// Epoch costs summary
		if len(stats.EpochCosts) > 1 {
			var mostExpIdx, cheapestIdx int
			for i, ec := range stats.EpochCosts {
				if ec.Cost.TotalCost > stats.EpochCosts[mostExpIdx].Cost.TotalCost {
					mostExpIdx = i
				}
				if ec.Cost.TotalCost < stats.EpochCosts[cheapestIdx].Cost.TotalCost {
					cheapestIdx = i
				}
			}
			fmt.Println("Epoch costs:")
			fmt.Printf("  Most expensive: #%d (%s, %d turns)\n",
				mostExpIdx, analyzer.FormatCost(stats.EpochCosts[mostExpIdx].Cost.TotalCost),
				stats.EpochCosts[mostExpIdx].TurnCount)
			fmt.Printf("  Cheapest:       #%d (%s, %d turns)\n",
				cheapestIdx, analyzer.FormatCost(stats.EpochCosts[cheapestIdx].Cost.TotalCost),
				stats.EpochCosts[cheapestIdx].TurnCount)
			fmt.Println()
		}
	}

	// Decision economics
	decEcon := analyzer.ComputeDecisionEconomics(stats, driftResult)
	if decEcon.HasDecisions {
		fmt.Println("Decision economics:")
		fmt.Printf("  Cost per decision: %s (%d decisions)\n",
			analyzer.FormatCost(decEcon.CPD), decEcon.TotalDecisions)
		fmt.Printf("  Turns to convergence: %d turns/decision\n", decEcon.TTC)
		fmt.Printf("  Decision density: 1 per %d turns\n", invertDensity(decEcon.DecisionDensity))
		if decEcon.CDR > 0 {
			fmt.Printf("  Context drift rate: %.0f%%\n", decEcon.CDR*100)
		}
		if decEcon.CDR > 0.35 {
			fmt.Println("  Warning: CDR > 35% — consider splitting session")
		}
		fmt.Println()
	}

	// Session health vector
	if statsHealth {
		thresholds := loadGaugeThresholds()
		gauge := analyzer.ComputeGauge(stats, decEcon, thresholds)
		fmt.Println("Session health vector:")
		fmt.Printf("  State:  %s (score: %d)\n", gauge.State, gauge.Score)
		fmt.Printf("  Action: %s\n", gauge.Action)
		if gauge.PostCompaction {
			fmt.Println("  Note: post-compaction checkpoint pending")
		}
		fmt.Println()
	}

	// Images
	fmt.Printf("Images: %d", stats.ImageCount)
	if stats.ImageBytesTotal > 0 {
		fmt.Printf(" (%.1f MB)", float64(stats.ImageBytesTotal)/1024/1024)
	}
	fmt.Println()
	if sidechainReport.TotalEntries > 0 {
		fmt.Printf("Sidechains: %d entries (%d groups, ~%s tokens)\n",
			sidechainReport.TotalEntries,
			sidechainReport.GroupCount,
			formatTokens(sidechainReport.TotalTokens))
		fmt.Printf("  Repairable: %d | Prune-only: %d\n",
			sidechainReport.RepairableCount,
			sidechainReport.PruneOnlyCount)
	}

	// Cleanup recommendations
	if rec != nil && len(rec.Items) > 0 {
		fmt.Println()
		fmt.Println("Cleanup recommendations:")
		for _, item := range rec.Items {
			turnsStr := ""
			if item.TurnsGained > 0 {
				turnsStr = fmt.Sprintf(" (+~%d turns)", item.TurnsGained)
			}
			fmt.Printf("  %-20s %3d items,  %s tokens%s\n",
				item.Label+":", item.Count, formatTokens(item.TokensSaved), turnsStr)
		}
		turnsStr := ""
		if rec.TotalTurnsGained > 0 {
			turnsStr = fmt.Sprintf(" (~%d additional turns)", rec.TotalTurnsGained)
		}
		fmt.Printf("  Total recoverable: %s tokens%s\n",
			formatTokens(rec.TotalTokens), turnsStr)
		fmt.Printf("  Projected: %.1f%% → %.1f%%\n",
			rec.CurrentPercent, rec.ProjectedPercent)

		// Projected savings if cleaned now
		if rec.TotalTokens > 0 && stats.EstimatedTurnsLeft > 0 {
			pricing := analyzer.PricingForModel(stats.Model)
			avoidedTokens := rec.TotalTokens * stats.EstimatedTurnsLeft
			avoidedCost := float64(avoidedTokens) / 1_000_000 * pricing.CacheReadPerMillion
			if avoidedCost > 0.001 {
				fmt.Printf("  Projected savings if cleaned now: ~%s cache-read tokens (~%s)\n",
					formatTokens(avoidedTokens), analyzer.FormatCost(avoidedCost))
			}
		}
	}

	// Past savings for this session
	dir := resolveClaudeDir()
	if allEvents, err := savings.Load(dir); err == nil && len(allEvents) > 0 {
		sessionSavings := savings.ForSession(allEvents, sessionID)
		if sessionSavings.TotalCleanups > 0 {
			fmt.Printf("\nSaved so far: ~%s tokens (~%s) from %d cleanups\n",
				formatTokens(sessionSavings.TotalRemoved),
				analyzer.FormatCost(sessionSavings.TotalSavedCost),
				sessionSavings.TotalCleanups)
		}
	}

	// Epoch timeline
	if showEpochs && len(stats.EpochCosts) > 1 && stats.Archaeology != nil {
		activeHint := extractFirstUserText(entries, stats)
		epochs := analyzer.BuildEpochs(stats.EpochCosts, stats.Archaeology, activeHint)

		fmt.Println()
		fmt.Println("Epoch timeline:")
		fmt.Printf("  %-7s %6s %10s %9s  %-30s %10s\n",
			"Epoch", "Turns", "Peak", "Cost", "Topic", "Survived")
		fmt.Println("  " + strings.Repeat("─", 78))

		costliestIdx := 0
		for i, ep := range epochs {
			if ep.Cost > epochs[costliestIdx].Cost {
				costliestIdx = i
			}
		}

		for i, ep := range epochs {
			survived := fmt.Sprintf("%d chars", ep.SurvivedChars)
			if ep.IsActive {
				survived = "(active)"
			}

			marker := " "
			if i == costliestIdx {
				marker = "*"
			}

			topic := ep.Topic
			if len([]rune(topic)) > 30 {
				topic = string([]rune(topic)[:27]) + "..."
			}

			fmt.Printf(" %s#%-6d %6d %10s %9s  %-30s %10s\n",
				marker, ep.Index, ep.TurnCount,
				formatTokens(ep.PeakTokens),
				analyzer.FormatCost(ep.Cost),
				topic, survived)
		}
		fmt.Println()
	}

	// Scope drift analysis
	if showScope && driftResult != nil && driftResult.TotalOutScope > 0 {
		fmt.Println("Scope drift analysis:")
		fmt.Printf("  Project: %s\n", driftResult.SessionProject)
		fmt.Printf("  Overall drift: %.1f%% (%d/%d entries out of scope)\n",
			driftResult.OverallDrift*100, driftResult.TotalOutScope,
			driftResult.TotalInScope+driftResult.TotalOutScope)
		fmt.Println()

		// Per-epoch table
		fmt.Printf("  %-7s %6s %6s %8s %9s  %s\n",
			"Epoch", "In", "Out", "Drift", "Cost", "External repos")
		fmt.Println("  " + strings.Repeat("─", 70))
		for _, es := range driftResult.EpochScopes {
			if es.InScope+es.OutScope == 0 {
				continue
			}
			repos := ""
			for repo, count := range es.OutScopeByRepo {
				repos += fmt.Sprintf("%s(%d) ", filepath.Base(repo), count)
			}
			fmt.Printf("  #%-6d %6d %6d %7.1f%% %9s  %s\n",
				es.EpochIndex, es.InScope, es.OutScope,
				es.DriftRatio*100, analyzer.FormatCost(es.DriftCost),
				strings.TrimSpace(repos))
		}

		// Tangent sequences
		if len(driftResult.TangentSeqs) > 0 {
			fmt.Println()
			fmt.Printf("  Tangent sequences (%d):\n", len(driftResult.TangentSeqs))
			for _, ts := range driftResult.TangentSeqs {
				fmt.Printf("    [%d-%d] → %s  %s tokens  %s\n",
					ts.StartIdx, ts.EndIdx, ts.TargetRepo,
					formatTokens(ts.TokenCost), analyzer.FormatCost(ts.DollarCost))
			}
		}
		fmt.Println()
	}

	// Cost alert check (text output only — JSON handled above)
	if stats.Cost != nil && stats.Cost.TotalCost > 0 {
		threshold := loadCostAlertThreshold()
		printCostAlert(stats.Cost.TotalCost, threshold)
	}

	// Record analytics snapshot on --record.
	if statsRecord {
		recordAnalyticsSnapshot(path)
		fmt.Println("\nAnalytics snapshot recorded.")
	}

	return nil
}

func resolveSessionPath(arg string) string {
	// If it's already a path, use it
	if strings.HasSuffix(arg, ".jsonl") {
		if filepath.IsAbs(arg) {
			return arg
		}
		return arg
	}

	// Try to find it in the claude projects dir
	dir := resolveClaudeDir()
	projectsDir := filepath.Join(dir, "projects")
	// Search all project dirs for a matching session UUID
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return arg + ".jsonl"
	}

	// First try exact match
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(projectsDir, e.Name(), arg+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Then try prefix match (short ID like "5d624f4a")
	if !strings.Contains(arg, "-") || len(arg) < 36 {
		var match string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			files, err := os.ReadDir(filepath.Join(projectsDir, e.Name()))
			if err != nil {
				continue
			}
			for _, f := range files {
				name := f.Name()
				if strings.HasPrefix(name, arg) && strings.HasSuffix(name, ".jsonl") && !strings.Contains(name, ".bak") {
					if match != "" {
						// Ambiguous — multiple matches, fall through
						return arg + ".jsonl"
					}
					match = filepath.Join(projectsDir, e.Name(), name)
				}
			}
		}
		if match != "" {
			return match
		}
	}

	return arg + ".jsonl"
}

// resolveCWDSession finds the most recent session for the current working directory.
// Returns the full path to the session JSONL file.
func resolveCWDSession() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	dir := resolveClaudeDir()
	encodedDir := session.EncodePath(cwd)
	projectDir := filepath.Join(dir, "projects", encodedDir)

	files, err := os.ReadDir(projectDir)
	if err != nil {
		return "", fmt.Errorf("no sessions found for %s", cwd)
	}

	type candidate struct {
		path    string
		modTime int64
	}
	var candidates []candidate
	for _, f := range files {
		name := f.Name()
		if strings.HasSuffix(name, ".jsonl") && !strings.Contains(name, ".bak") {
			info, err := f.Info()
			if err != nil {
				continue
			}
			candidates = append(candidates, candidate{
				path:    filepath.Join(projectDir, name),
				modTime: info.ModTime().UnixNano(),
			})
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no sessions found for %s", cwd)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime > candidates[j].modTime
	})
	return candidates[0].path, nil
}

// resolveSessionArg resolves the session path from args or --cwd flag.
// If useCWD is true and no args provided, auto-discovers the most recent session for CWD.
func resolveSessionArg(args []string, useCWD bool) (string, error) {
	if len(args) > 0 {
		path := resolveSessionPath(args[0])
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return "", fmt.Errorf("session not found: %s", path)
		}
		return path, nil
	}
	if useCWD || os.Getenv("CLAUDECODE") == "1" {
		return resolveCWDSession()
	}
	return "", fmt.Errorf("provide a session ID or use --cwd")
}

func formatTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func contextBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func extractFirstUserText(entries []jsonl.Entry, stats *analyzer.ContextStats) string {
	if len(stats.Compactions) == 0 {
		return ""
	}
	lastBoundary := stats.Compactions[len(stats.Compactions)-1].LineIndex
	for i := lastBoundary; i < len(entries); i++ {
		if entries[i].Type != jsonl.TypeUser || entries[i].Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(entries[i].Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				return b.Text
			}
		}
	}
	return ""
}

// invertDensity converts a density (decisions/turn) to turns/decision.
func invertDensity(density float64) int {
	if density <= 0 {
		return 0
	}
	return int(1 / density)
}

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.Flags().BoolVar(&showEpochs, "epochs", false, "Show epoch timeline")
	statsCmd.Flags().BoolVar(&showScope, "scope", false, "Show scope drift analysis")
	statsCmd.Flags().BoolVar(&statsCWD, "cwd", false, "Use most recent session for current directory")
	statsCmd.Flags().BoolVar(&statsRecord, "record", false, "Record an analytics snapshot for this session")
	statsCmd.Flags().BoolVar(&statsHealth, "health", false, "Show session health vector gauge")
}
