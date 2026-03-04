package commands

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/savings"
	"github.com/spf13/cobra"
)

var savingsCmd = &cobra.Command{
	Use:   "savings",
	Short: "Show lifetime cleanup savings summary",
	RunE:  runSavings,
}

func runSavings(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	events, err := savings.Load(dir)
	if err != nil {
		return err
	}

	summary := savings.Aggregate(events, 5)

	if isJSON() {
		return printJSON(summary)
	}

	if summary.TotalCleanups == 0 {
		fmt.Println("No cleanup savings recorded yet.")
		fmt.Println("Run `contextspectre clean <session> --all` to start tracking savings.")
		return nil
	}

	fmt.Println("Lifetime cleanup savings (conservative estimate)")
	fmt.Printf("  Total cleanups:     %d\n", summary.TotalCleanups)
	fmt.Printf("  Tokens removed:     %s\n", formatTokens(summary.TotalRemoved))
	fmt.Printf("  Projected savings:  %s cache-read tokens\n", formatTokens(summary.TotalAvoided))
	fmt.Printf("  Estimated value:    %s\n", analyzer.FormatCost(summary.TotalSavedCost))
	fmt.Printf("  Avg per cleanup:    %s\n", analyzer.FormatCost(summary.AvgPerCleanup))

	if len(summary.RecentEvents) > 0 {
		fmt.Println()
		fmt.Printf("Last %d cleanups:\n", len(summary.RecentEvents))
		for _, e := range summary.RecentEvents {
			date := e.Timestamp.Format("2006-01-02")
			sid := e.SessionID
			if len(sid) > 8 {
				sid = sid[:8]
			}
			if e.Slug != "" {
				sid = e.Slug
			}
			fmt.Printf("  %s  %-20s  %s tokens removed  ~%s saved\n",
				date, sid,
				formatTokens(e.TokensRemoved),
				analyzer.FormatCost(e.AvoidedCost))
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(savingsCmd)
}
