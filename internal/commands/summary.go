package commands

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	summaryCWD   bool
	summaryQuiet bool
)

var summaryCmd = &cobra.Command{
	Use:   "summary [session-id-or-path]",
	Short: "Print a one-screen session summary",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSummary,
}

func runSummary(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, summaryCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	stats := analyzer.Analyze(entries)
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	// Compute health
	dupResult := analyzer.FindDuplicateReads(entries)
	retryResult := analyzer.FindFailedRetries(entries)
	tangentResult := analyzer.FindTangents(entries)
	rec := analyzer.Recommend(stats, dupResult, retryResult, tangentResult)
	health := analyzer.ComputeHealth(stats, rec)

	// Extract top files
	topFiles := extractTopFiles(entries, 5)

	// Session duration
	duration := sessionDuration(entries)

	if isJSON() {
		return printJSON(buildSummaryJSON(sessionID, stats, health, rec, topFiles, duration))
	}

	if summaryQuiet {
		return printQuietSummary(stats, health, topFiles)
	}

	return printFullSummary(sessionID, stats, health, rec, topFiles, duration)
}

func printQuietSummary(stats *analyzer.ContextStats, health *analyzer.HealthScore, topFiles []fileCount) error {
	cost := "$0.00"
	if stats.Cost != nil {
		cost = analyzer.FormatCost(stats.Cost.TotalCost)
	}

	grade := "—"
	if health != nil && health.TotalTokens > 0 {
		grade = health.Grade
	}

	files := ""
	if len(topFiles) > 0 {
		names := make([]string, 0, 3)
		for i, f := range topFiles {
			if i >= 3 {
				break
			}
			names = append(names, filepath.Base(f.path))
		}
		files = " | Top: " + strings.Join(names, ", ")
	}

	fmt.Printf("Session: %s | %d turns | Signal %s | %dx compact%s\n",
		cost, stats.ConversationalTurns, grade, stats.CompactionCount, files)
	return nil
}

func printFullSummary(sessionID string, stats *analyzer.ContextStats, health *analyzer.HealthScore, rec *analyzer.CleanupRecommendation, topFiles []fileCount, duration time.Duration) error {
	fmt.Printf("Session summary: %s\n", sessionID[:min(8, len(sessionID))])
	fmt.Println(strings.Repeat("─", 50))

	// Cost and model
	if stats.Cost != nil {
		fmt.Printf("Cost:        %s (%s)\n", analyzer.FormatCost(stats.Cost.TotalCost), stats.Model)
	} else {
		fmt.Printf("Model:       %s\n", stats.Model)
	}

	// Turns and duration
	fmt.Printf("Turns:       %d conversational", stats.ConversationalTurns)
	if duration > 0 {
		fmt.Printf(" over %s", formatDuration(duration))
	}
	fmt.Println()

	// Context
	fmt.Printf("Context:     %.1f%% (%d/%d tokens)\n",
		stats.UsagePercent, stats.CurrentContextTokens, analyzer.ContextWindowSize)

	// Signal grade
	if health != nil && health.TotalTokens > 0 {
		fmt.Printf("Signal:      %s (%.0f%% signal, %.0f%% noise)\n",
			health.Grade, health.SignalPercent, health.NoisePercent)
	}

	// Compactions
	if stats.CompactionCount > 0 {
		fmt.Printf("Compactions: %d", stats.CompactionCount)
		if len(stats.Compactions) > 0 {
			last := stats.Compactions[len(stats.Compactions)-1]
			fmt.Printf(" (last: %dK → %dK)", last.BeforeTokens/1000, last.AfterTokens/1000)
		}
		fmt.Println()
	}

	// Top files
	if len(topFiles) > 0 {
		fmt.Println("\nTop files modified:")
		for i, f := range topFiles {
			if i >= 5 {
				break
			}
			fmt.Printf("  %s (%dx)\n", f.path, f.count)
		}
	}

	// Cleanup recommendation
	if rec != nil && rec.TotalTokens > 0 {
		fmt.Printf("\nCleanup opportunity: %s tokens recoverable (+%d turns)\n",
			formatTokens(rec.TotalTokens), rec.TotalTurnsGained)
	}

	// Cost alert
	if stats.Cost != nil && stats.Cost.TotalCost > 0 {
		threshold := loadCostAlertThreshold()
		printCostAlert(stats.Cost.TotalCost, threshold)
	}

	return nil
}

type fileCount struct {
	path  string
	count int
}

// extractTopFiles finds the most frequently written/edited files in the session.
func extractTopFiles(entries []jsonl.Entry, limit int) []fileCount {
	counts := make(map[string]int)

	for _, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			if !isWriteToolForSummary(b.Name) {
				continue
			}
			path := analyzer.ExtractToolInputPath(b.Input)
			if path != "" {
				counts[path]++
			}
		}
	}

	result := make([]fileCount, 0, len(counts))
	for p, c := range counts {
		result = append(result, fileCount{path: p, count: c})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].count > result[j].count
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func isWriteToolForSummary(name string) bool {
	switch name {
	case "Write", "Edit", "write_file", "edit_file", "WriteFile", "NotebookEdit":
		return true
	}
	return false
}

func sessionDuration(entries []jsonl.Entry) time.Duration {
	if len(entries) < 2 {
		return 0
	}
	first := entries[0].Timestamp
	last := entries[len(entries)-1].Timestamp
	if first.IsZero() || last.IsZero() {
		return 0
	}
	return last.Sub(first)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// SummaryJSON is the JSON output for the summary command.
type SummaryJSON struct {
	SessionID   string   `json:"session_id"`
	Model       string   `json:"model,omitempty"`
	TotalCost   float64  `json:"total_cost"`
	Turns       int      `json:"turns"`
	DurationSec int      `json:"duration_seconds"`
	ContextPct  float64  `json:"context_percent"`
	Compactions int      `json:"compactions"`
	Grade       string   `json:"signal_grade,omitempty"`
	NoisePct    float64  `json:"noise_percent,omitempty"`
	TopFiles    []string `json:"top_files,omitempty"`
	Cleanable   int      `json:"cleanable_tokens,omitempty"`
}

func buildSummaryJSON(sessionID string, stats *analyzer.ContextStats, health *analyzer.HealthScore, rec *analyzer.CleanupRecommendation, topFiles []fileCount, duration time.Duration) *SummaryJSON {
	out := &SummaryJSON{
		SessionID:   sessionID,
		Model:       stats.Model,
		Turns:       stats.ConversationalTurns,
		DurationSec: int(duration.Seconds()),
		ContextPct:  stats.UsagePercent,
		Compactions: stats.CompactionCount,
	}
	if stats.Cost != nil {
		out.TotalCost = stats.Cost.TotalCost
	}
	if health != nil && health.TotalTokens > 0 {
		out.Grade = health.Grade
		out.NoisePct = health.NoisePercent
	}
	for _, f := range topFiles {
		out.TopFiles = append(out.TopFiles, f.path)
	}
	if rec != nil {
		out.Cleanable = rec.TotalTokens
	}
	return out
}

func init() {
	summaryCmd.Flags().BoolVar(&summaryCWD, "cwd", false, "Auto-detect session from current working directory")
	summaryCmd.Flags().BoolVar(&summaryQuiet, "quiet", false, "Single-line output for hooks")
	rootCmd.AddCommand(summaryCmd)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
