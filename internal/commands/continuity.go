package commands

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	continuityProject string
	continuityCWD     bool
)

var continuityCmd = &cobra.Command{
	Use:   "continuity",
	Short: "Measure re-explanation tax across sessions for a project",
	Long: `Scan all sessions for a project and find repeated file reads and
user text blocks across sessions. Quantifies the cost of rebuilding
context that existed in prior sessions.

Examples:
  contextspectre continuity --cwd
  contextspectre continuity --project myapp
  contextspectre continuity --cwd --format json`,
	RunE: runContinuity,
}

func runContinuity(cmd *cobra.Command, args []string) error {
	if continuityProject == "" && !continuityCWD {
		return fmt.Errorf("--project or --cwd is required")
	}

	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}

	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	filtered := filterDistillSessions(sessions, continuityProject, continuityCWD)
	if len(filtered) == 0 {
		if isJSON() {
			return printJSON(ContinuityOutputJSON{
				RepeatedFiles: []RepeatedFileJSON{},
				RepeatedTexts: []RepeatedTextJSON{},
			})
		}
		fmt.Println("No matching sessions found.")
		return nil
	}

	projectName := filtered[0].ProjectName
	if projectName == "" {
		projectName = "unknown"
	}

	var inputs []analyzer.ContinuitySessionInput
	for _, si := range filtered {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			continue
		}

		model := ""
		if si.ContextStats != nil {
			model = si.ContextStats.Model
		}

		inputs = append(inputs, analyzer.ContinuitySessionInput{
			SessionID:   si.SessionID,
			SessionSlug: si.Slug,
			Entries:     entries,
			Model:       model,
		})
	}

	if len(inputs) < 2 {
		if isJSON() {
			return printJSON(ContinuityOutputJSON{
				ProjectName:     projectName,
				SessionsScanned: len(inputs),
				RepeatedFiles:   []RepeatedFileJSON{},
				RepeatedTexts:   []RepeatedTextJSON{},
			})
		}
		fmt.Println("Need at least 2 sessions for continuity analysis.")
		return nil
	}

	report := analyzer.AnalyzeContinuity(inputs)
	report.ProjectName = projectName

	if isJSON() {
		return printJSON(buildContinuityOutputJSON(report))
	}

	printContinuityReport(report)
	return nil
}

func printContinuityReport(r *analyzer.ContinuityReport) {
	fmt.Printf("Cross-session continuity — %s (%d sessions)\n\n",
		r.ProjectName, r.SessionsScanned)

	if len(r.RepeatedFiles) > 0 {
		sessionSet := make(map[string]bool)
		for _, rf := range r.RepeatedFiles {
			for _, s := range rf.Sessions {
				sessionSet[s] = true
			}
		}
		fmt.Printf("Repeated file reads: %d files across %d sessions\n",
			len(r.RepeatedFiles), len(sessionSet))
		limit := 10
		if len(r.RepeatedFiles) < limit {
			limit = len(r.RepeatedFiles)
		}
		for _, rf := range r.RepeatedFiles[:limit] {
			fmt.Printf("  %-50s %d/%d sessions\n",
				truncatePath(rf.Path, 50),
				rf.SessionCount, r.SessionsScanned)
		}
		if len(r.RepeatedFiles) > 10 {
			fmt.Printf("  ... and %d more\n", len(r.RepeatedFiles)-10)
		}
		fmt.Println()
	} else {
		fmt.Println("No repeated file reads detected.")
		fmt.Println()
	}

	if len(r.RepeatedTexts) > 0 {
		fmt.Printf("Repeated explanations: %d text blocks\n", len(r.RepeatedTexts))
		limit := 5
		if len(r.RepeatedTexts) < limit {
			limit = len(r.RepeatedTexts)
		}
		for _, rt := range r.RepeatedTexts[:limit] {
			fmt.Printf("  \"%s\" — %d sessions (%d chars)\n",
				rt.Text, rt.SessionCount, rt.CharCount)
		}
		fmt.Println()
	}

	fmt.Printf("Re-explanation tax: ~%s tokens (%s)\n",
		formatTokens(r.TotalTaxTokens),
		analyzer.FormatCost(r.TotalTaxCost))
	fmt.Printf("  File re-reads: ~%s tokens (%s)\n",
		formatTokens(r.TotalFileTokens),
		analyzer.FormatCost(r.TotalFileCost))
	fmt.Printf("  Text repeats:  ~%s tokens (%s)\n",
		formatTokens(r.TotalTextTokens),
		analyzer.FormatCost(r.TotalTextCost))

	if r.TotalTaxTokens > 1000 {
		fmt.Println()
		fmt.Println("Recommendation: export shared context to reduce re-explanation")
		fmt.Println("  Run: contextspectre distill --cwd --auto")
	}
}

func buildContinuityOutputJSON(r *analyzer.ContinuityReport) *ContinuityOutputJSON {
	out := &ContinuityOutputJSON{
		ProjectName:     r.ProjectName,
		SessionsScanned: r.SessionsScanned,
		TotalFileTokens: r.TotalFileTokens,
		TotalTextTokens: r.TotalTextTokens,
		TotalTaxTokens:  r.TotalTaxTokens,
		TotalTaxCost:    r.TotalTaxCost,
	}

	for _, rf := range r.RepeatedFiles {
		out.RepeatedFiles = append(out.RepeatedFiles, RepeatedFileJSON{
			Path:            rf.Path,
			SessionCount:    rf.SessionCount,
			Sessions:        rf.Sessions,
			EstimatedTokens: rf.EstimatedTokens,
		})
	}
	for _, rt := range r.RepeatedTexts {
		out.RepeatedTexts = append(out.RepeatedTexts, RepeatedTextJSON{
			Text:            rt.Text,
			CharCount:       rt.CharCount,
			SessionCount:    rt.SessionCount,
			Sessions:        rt.Sessions,
			EstimatedTokens: rt.EstimatedTokens,
		})
	}

	if out.RepeatedFiles == nil {
		out.RepeatedFiles = []RepeatedFileJSON{}
	}
	if out.RepeatedTexts == nil {
		out.RepeatedTexts = []RepeatedTextJSON{}
	}
	return out
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

func init() {
	continuityCmd.Flags().StringVar(&continuityProject, "project", "",
		"Filter sessions by project name (substring match)")
	continuityCmd.Flags().BoolVar(&continuityCWD, "cwd", false,
		"Use sessions for the current working directory")
	rootCmd.AddCommand(continuityCmd)
}
