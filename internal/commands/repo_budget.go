package commands

import (
	"fmt"
	"sort"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	repoBudgetProject string
	repoBudgetCWD     bool
	repoBudgetAll     bool
)

var repoBudgetCmd = &cobra.Command{
	Use:   "repo-budget",
	Short: "Show project-level reasoning token budget",
	Long: `Aggregate all sessions for a project and report total tokens,
cost, and per-session breakdown.

  contextspectre repo-budget --cwd
  contextspectre repo-budget --project myapp
  contextspectre repo-budget --all`,
	RunE: runRepoBudget,
}

// RepoBudgetOutput is the JSON output for the repo-budget command.
type RepoBudgetOutput struct {
	Project      string              `json:"project"`
	Sessions     int                 `json:"sessions"`
	InputTokens  int                 `json:"input_tokens"`
	OutputTokens int                 `json:"output_tokens"`
	TotalTokens  int                 `json:"total_tokens"`
	TotalCost    float64             `json:"total_cost"`
	TopSessions  []RepoBudgetSession `json:"top_sessions"`
}

// RepoBudgetSession is one session in the budget breakdown.
type RepoBudgetSession struct {
	SessionID    string  `json:"session_id"`
	Slug         string  `json:"slug,omitempty"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Cost         float64 `json:"cost"`
	Turns        int     `json:"turns"`
}

func runRepoBudget(cmd *cobra.Command, args []string) error {
	if !repoBudgetCWD && repoBudgetProject == "" && !repoBudgetAll {
		return fmt.Errorf("specify --cwd, --project <name>, or --all")
	}

	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if repoBudgetCWD {
		sessions = filterDistillSessions(sessions, "", true)
	} else if repoBudgetProject != "" {
		sessions = resolveProjectSessions(sessions, repoBudgetProject, dir)
	}

	if len(sessions) == 0 {
		if isJSON() {
			return printJSON(RepoBudgetOutput{TopSessions: []RepoBudgetSession{}})
		}
		fmt.Println("No sessions found.")
		return nil
	}

	projectName := sessions[0].ProjectName
	var budgetSessions []RepoBudgetSession
	var totalInput, totalOutput int
	var totalCost float64

	for _, si := range sessions {
		stats, err := jsonl.ScanLight(si.FullPath)
		if err != nil {
			continue
		}

		input := stats.TotalInputTokens + stats.TotalCacheWriteTokens + stats.TotalCacheReadTokens
		output := stats.TotalOutputTokens
		cost := analyzer.QuickCost(
			stats.TotalInputTokens, stats.TotalOutputTokens,
			stats.TotalCacheWriteTokens, stats.TotalCacheReadTokens,
			stats.Model,
		)

		totalInput += input
		totalOutput += output
		totalCost += cost

		budgetSessions = append(budgetSessions, RepoBudgetSession{
			SessionID:    si.SessionID,
			Slug:         si.DisplayName(),
			InputTokens:  input,
			OutputTokens: output,
			TotalTokens:  input + output,
			Cost:         cost,
			Turns:        stats.LineCount,
		})
	}

	// Sort by total tokens descending.
	sort.Slice(budgetSessions, func(i, j int) bool {
		return budgetSessions[i].TotalTokens > budgetSessions[j].TotalTokens
	})

	out := RepoBudgetOutput{
		Project:      projectName,
		Sessions:     len(budgetSessions),
		InputTokens:  totalInput,
		OutputTokens: totalOutput,
		TotalTokens:  totalInput + totalOutput,
		TotalCost:    totalCost,
		TopSessions:  budgetSessions,
	}

	if isJSON() {
		return printJSON(out)
	}

	// Text output.
	fmt.Printf("Project: %s\n", out.Project)
	fmt.Printf("Sessions:       %d\n", out.Sessions)
	fmt.Printf("Total tokens:   %s\n", formatTokenCount(out.TotalTokens))
	fmt.Printf("  Input:        %s\n", formatTokenCount(out.InputTokens))
	fmt.Printf("  Output:       %s\n", formatTokenCount(out.OutputTokens))
	fmt.Printf("Estimated cost: %s\n", analyzer.FormatCost(out.TotalCost))
	fmt.Println()

	limit := 10
	if len(out.TopSessions) < limit {
		limit = len(out.TopSessions)
	}
	fmt.Println("Top sessions by token usage:")
	for i := 0; i < limit; i++ {
		s := out.TopSessions[i]
		fmt.Printf("  %d. %-28s %10s  %s  %d turns\n",
			i+1, s.Slug, formatTokenCount(s.TotalTokens),
			analyzer.FormatCost(s.Cost), s.Turns)
	}

	return nil
}

func formatTokenCount(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fK", float64(tokens)/1_000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

func init() {
	repoBudgetCmd.Flags().StringVar(&repoBudgetProject, "project", "", "Filter by project name or alias")
	repoBudgetCmd.Flags().BoolVar(&repoBudgetCWD, "cwd", false, "Use sessions for current working directory")
	repoBudgetCmd.Flags().BoolVar(&repoBudgetAll, "all", false, "Aggregate all sessions globally")
	rootCmd.AddCommand(repoBudgetCmd)
}
