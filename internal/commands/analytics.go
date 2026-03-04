package commands

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/ppiankov/contextspectre/internal/analytics"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/spf13/cobra"
)

var (
	analyticsSince   string
	analyticsProject string
	analyticsCSV     bool
)

var analyticsCmd = &cobra.Command{
	Use:   "analytics",
	Short: "Show aggregated session analytics",
	Long: `Display aggregated session analytics from recorded snapshots.
Snapshots are automatically recorded when using summary, clean --active --all,
and stats --record commands.`,
	RunE: runAnalytics,
}

func runAnalytics(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	snapshots, err := analytics.Load(dir)
	if err != nil {
		return err
	}

	if len(snapshots) == 0 {
		fmt.Println("No analytics data yet. Run 'summary', 'clean --active --all', or 'stats --record' to start recording.")
		return nil
	}

	// Parse --since duration.
	sinceDuration, err := parseSinceDuration(analyticsSince)
	if err != nil {
		return fmt.Errorf("invalid --since value %q: %w", analyticsSince, err)
	}

	filtered := analytics.Filter(snapshots, analytics.FilterOpts{
		Since:   sinceDuration,
		Project: analyticsProject,
	})

	if len(filtered) == 0 {
		fmt.Println("No analytics data matching filters.")
		return nil
	}

	if analyticsCSV {
		return printAnalyticsCSV(filtered)
	}

	if isJSON() {
		summary := analytics.Aggregate(filtered)
		return printJSON(summary)
	}

	return printAnalyticsHuman(filtered, sinceDuration)
}

func printAnalyticsHuman(snapshots []analytics.Snapshot, since time.Duration) error {
	summary := analytics.Aggregate(snapshots)

	label := "all time"
	if since > 0 {
		label = fmt.Sprintf("last %s", formatSinceDuration(since))
	}

	fmt.Printf("Session analytics (%s):\n", label)
	fmt.Printf("  Sessions:        %d\n", summary.Sessions)
	fmt.Printf("  Total cost:      %s\n", analyzer.FormatCost(summary.TotalCost))
	fmt.Printf("  Avg cost:        %s/session\n", analyzer.FormatCost(summary.AvgCost))
	fmt.Printf("  Total turns:     %s\n", formatLargeNumber(summary.TotalTurns))
	fmt.Printf("  Avg signal:      %s (%.1f%%)\n", summary.AvgGrade, summary.AvgSignalPct)

	if summary.TokensSaved > 0 {
		fmt.Printf("  Tokens saved:    %s (from %d cleanups)\n",
			formatTokens(summary.TokensSaved), summary.CleanupCount)
	}

	if len(summary.ModelMix) > 0 {
		fmt.Printf("  Model mix:       %s\n", formatModelMix(summary.ModelMix))
	}

	fmt.Printf("  Avg compactions: %.1f/session\n", summary.AvgCompactions)

	return nil
}

func printAnalyticsCSV(snapshots []analytics.Snapshot) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	header := []string{
		"timestamp", "session_id", "project", "slug", "client",
		"duration_hours", "turns", "compactions", "context_pct",
		"signal_grade", "signal_pct", "noise_tokens",
		"cost_total", "cost_per_turn", "cost_velocity_hr",
		"model_primary", "cleanup_tokens_saved", "cleanup_count", "top_noise",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, s := range snapshots {
		row := []string{
			s.Timestamp.Format(time.RFC3339),
			s.SessionID,
			s.Project,
			s.Slug,
			s.Client,
			strconv.FormatFloat(s.DurationHours, 'f', 2, 64),
			strconv.Itoa(s.Turns),
			strconv.Itoa(s.Compactions),
			strconv.FormatFloat(s.ContextPct, 'f', 1, 64),
			s.SignalGrade,
			strconv.FormatFloat(s.SignalPct, 'f', 1, 64),
			strconv.Itoa(s.NoiseTokens),
			strconv.FormatFloat(s.CostTotal, 'f', 2, 64),
			strconv.FormatFloat(s.CostPerTurn, 'f', 4, 64),
			strconv.FormatFloat(s.CostVelocityHr, 'f', 2, 64),
			s.ModelPrimary,
			strconv.Itoa(s.CleanupTokensSaved),
			strconv.Itoa(s.CleanupCount),
			s.TopNoise,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// parseSinceDuration parses duration strings like "7d", "30d", "1h", "10m".
func parseSinceDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	// Handle day suffix (not supported by time.ParseDuration).
	if len(s) > 1 && s[len(s)-1] == 'd' {
		days, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, fmt.Errorf("invalid day value: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	return time.ParseDuration(s)
}

func formatSinceDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%d days", days)
	}
	return d.String()
}

func formatLargeNumber(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return strconv.Itoa(n)
}

func formatModelMix(mix map[string]int) string {
	type modelEntry struct {
		name  string
		count int
	}

	total := 0
	var entries []modelEntry
	for name, count := range mix {
		entries = append(entries, modelEntry{name, count})
		total += count
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})

	if total == 0 {
		return "—"
	}

	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		pct := float64(e.count) / float64(total) * 100
		short := analyzer.PricingForModel(e.name).Name
		if short == "" {
			short = e.name
		}
		parts = append(parts, fmt.Sprintf("%s %.0f%%", short, pct))
	}

	return joinComma(parts)
}

func joinComma(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}

func init() {
	analyticsCmd.Flags().StringVar(&analyticsSince, "since", "30d", "Time filter (e.g. 7d, 30d, 1h)")
	analyticsCmd.Flags().StringVar(&analyticsProject, "project", "", "Filter by project name")
	analyticsCmd.Flags().BoolVar(&analyticsCSV, "csv", false, "Export as CSV")
	rootCmd.AddCommand(analyticsCmd)
}
