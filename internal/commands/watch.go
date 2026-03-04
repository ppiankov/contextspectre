package commands

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	watchInterval int
	watchCWD      bool
	watchAlert    float64
)

var watchCmd = &cobra.Command{
	Use:   "watch [session-id-or-path]",
	Short: "Real-time context stats tail",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runWatch,
}

func runWatch(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, watchCWD)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(time.Duration(watchInterval) * time.Second)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	var prevTokens int
	alerted := false

	// Print header
	fmt.Printf("Watching: %s  (interval: %ds, Ctrl+C to quit)\n", path, watchInterval)

	// Initial display
	prevTokens = displayWatchLine(path, prevTokens, &alerted)

	for {
		select {
		case <-ticker.C:
			prevTokens = displayWatchLine(path, prevTokens, &alerted)
			tryExpertClean(path)
		case <-sigCh:
			fmt.Println()
			return nil
		}
	}
}

func displayWatchLine(path string, prevTokens int, alerted *bool) int {
	stats, err := jsonl.ScanLight(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\r  Error: %v", err)
		return prevTokens
	}

	tokens := 0
	pct := 0.0
	if stats.LastUsage != nil {
		tokens = stats.LastUsage.TotalContextTokens()
		pct = float64(tokens) / float64(analyzer.ContextWindowSize) * 100
	}

	turnsLeft := 0
	if tokens > 0 && stats.LineCount > 0 {
		// Rough estimate: (remaining capacity) / (avg tokens per message)
		remaining := analyzer.ContextWindowSize - tokens
		avgPerMsg := tokens / stats.LineCount
		if avgPerMsg > 0 {
			turnsLeft = remaining / avgPerMsg
		}
	}

	cost := analyzer.QuickCost(
		stats.TotalInputTokens, stats.TotalOutputTokens,
		stats.TotalCacheWriteTokens, stats.TotalCacheReadTokens,
		stats.Model,
	)

	grade := analyzer.GradeFromSignalPercent(stats.SignalPercent)

	// Color based on context percentage
	color := colorForPct(pct)

	// Context bar (20 chars)
	barWidth := 20
	filled := int(pct / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Compaction detection
	compactionNote := ""
	if prevTokens > 0 && tokens < prevTokens-10000 {
		compactionNote = fmt.Sprintf("  ⚡ COMPACTED %dK→%dK", prevTokens/1000, tokens/1000)
	}

	// Build line
	line := fmt.Sprintf("\r  %sctx:%4.1f%% [%s]%s  ~%d turns  sig:%s  $%s  %dx compact  msgs:%d%s",
		color, pct, bar, colorReset(),
		turnsLeft, grade,
		analyzer.FormatCost(cost),
		stats.CompactionCount,
		stats.LineCount,
		compactionNote,
	)

	// Pad to overwrite previous line
	fmt.Fprintf(os.Stdout, "%-120s", line)

	// Alert
	if watchAlert > 0 && pct >= watchAlert && !*alerted {
		fmt.Print("\a") // terminal bell
		*alerted = true
	}
	if pct < watchAlert {
		*alerted = false
	}

	return tokens
}

func colorForPct(pct float64) string {
	switch {
	case pct >= 75:
		return "\033[31m" // red
	case pct >= 50:
		return "\033[33m" // yellow
	default:
		return "\033[32m" // green
	}
}

func colorReset() string {
	return "\033[0m"
}

func init() {
	watchCmd.Flags().IntVar(&watchInterval, "interval", 5, "Refresh interval in seconds")
	watchCmd.Flags().BoolVar(&watchCWD, "cwd", false, "Auto-detect session from current working directory")
	watchCmd.Flags().Float64Var(&watchAlert, "alert", 0, "Emit terminal bell when context exceeds this percentage")
	rootCmd.AddCommand(watchCmd)
}
