package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats <session-id-or-path>",
	Short: "Show context statistics for a session",
	Args:  cobra.ExactArgs(1),
	RunE:  runStats,
}

func runStats(cmd *cobra.Command, args []string) error {
	path := resolveSessionPath(args[0])
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", path)
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	stats := analyzer.Analyze(entries)
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	if isJSON() {
		out := buildStatsOutput(sessionID, stats)
		return printJSON(out)
	}

	fi, _ := os.Stat(path)

	fmt.Printf("Session: %s\n", filepath.Base(path))
	if fi != nil {
		fmt.Printf("File size: %.1f MB\n", float64(fi.Size())/1024/1024)
	}
	fmt.Printf("Total lines: %d\n", stats.TotalLines)
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

	// Growth and distance
	fmt.Printf("Token growth rate: ~%.0f tokens/turn\n", stats.TokenGrowthRate)
	if stats.EstimatedTurnsLeft >= 0 {
		fmt.Printf("Estimated turns until compaction: ~%d\n", stats.EstimatedTurnsLeft)
	} else {
		fmt.Println("Estimated turns until compaction: unknown")
	}
	fmt.Println()

	// Images
	fmt.Printf("Images: %d", stats.ImageCount)
	if stats.ImageBytesTotal > 0 {
		fmt.Printf(" (%.1f MB)", float64(stats.ImageBytesTotal)/1024/1024)
	}
	fmt.Println()

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
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(projectsDir, e.Name(), arg+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return arg + ".jsonl"
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

func init() {
	rootCmd.AddCommand(statsCmd)
}
