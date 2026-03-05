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
	Use:   "continuity [session-id-or-path]",
	Short: "Measure re-explanation tax across sessions for a project",
	Args:  cobra.MaximumNArgs(1),
	Long: `Scan all sessions for a project and find repeated file reads and
user text blocks across sessions. Quantifies the cost of rebuilding
context that existed in prior sessions.

Examples:
  contextspectre continuity 5d624f4a
  contextspectre continuity --cwd
  contextspectre continuity --project myapp
  contextspectre continuity --cwd --format json`,
	RunE: runContinuity,
}

func runContinuity(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}

	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	projectFilter := continuityProject
	useCWD := continuityCWD
	if len(args) > 0 {
		targetPath := resolveSessionPath(args[0])
		for _, si := range sessions {
			if si.FullPath == targetPath || si.SessionID == args[0] || si.ShortID() == args[0] {
				projectFilter = si.ProjectName
				break
			}
		}
	}
	if projectFilter == "" && !useCWD {
		return fmt.Errorf("--project, --cwd, or session-id is required")
	}

	filtered := filterDistillSessions(sessions, projectFilter, useCWD)
	if len(filtered) == 0 {
		if isJSON() {
			return printJSON(ContinuityOutputJSON{
				RepeatedFiles: []RepeatedFileJSON{},
				RepeatedTexts: []RepeatedTextJSON{},
				RepeatTopics:  []RepeatTopicJSON{},
				Suggestions:   []ContinuitySuggestJSON{},
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
				RepeatTopics:    []RepeatTopicJSON{},
				Suggestions:     []ContinuitySuggestJSON{},
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
	fmt.Printf("Continuity index: %.1f/100\n", r.ContinuityIndex)
	if r.TotalFileReads > 0 || r.TotalTextBlocks > 0 {
		fmt.Printf("  Files: %d unique / %d reads\n", r.UniqueFileReads, r.TotalFileReads)
		fmt.Printf("  Text blocks: %d unique / %d blocks\n", r.UniqueTextBlocks, r.TotalTextBlocks)
	}
	fmt.Println()

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
			fmt.Printf("  %-50s %d/%d sessions  %s  (%s)\n",
				truncatePath(rf.Path, 50),
				rf.SessionCount, r.SessionsScanned,
				formatTokens(rf.EstimatedTokens), analyzer.FormatCost(rf.EstimatedCost))
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
			fmt.Printf("  \"%s\" — %d sessions (%d chars, %s, %s)\n",
				rt.Text, rt.SessionCount, rt.CharCount,
				formatTokens(rt.EstimatedTokens), analyzer.FormatCost(rt.EstimatedCost))
		}
		fmt.Println()
	}

	if len(r.RepeatTopics) > 0 {
		fmt.Printf("Repeat topics: %d file clusters\n", len(r.RepeatTopics))
		limit := 5
		if len(r.RepeatTopics) < limit {
			limit = len(r.RepeatTopics)
		}
		for _, tp := range r.RepeatTopics[:limit] {
			fmt.Printf("  [%s] + [%s] — %d sessions (%s)\n",
				truncatePath(tp.Files[0], 30),
				truncatePath(tp.Files[1], 30),
				tp.SessionCount,
				analyzer.FormatCost(tp.EstimatedCost))
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

	if len(r.Suggestions) > 0 {
		fmt.Println()
		fmt.Println("CLAUDE.md candidates:")
		limit := 5
		if len(r.Suggestions) < limit {
			limit = len(r.Suggestions)
		}
		for _, s := range r.Suggestions[:limit] {
			fmt.Printf("  %s  (%d sessions, %s)\n",
				truncatePath(s.Path, 60), s.SessionCount, analyzer.FormatCost(s.EstimatedCost))
		}
	}

	if r.TotalTaxTokens > 1000 {
		fmt.Println()
		fmt.Println("Recommendation: export shared context to reduce re-explanation")
		fmt.Println("  Run: contextspectre distill --cwd --auto")
	}
}

func buildContinuityOutputJSON(r *analyzer.ContinuityReport) *ContinuityOutputJSON {
	out := &ContinuityOutputJSON{
		ProjectName:      r.ProjectName,
		SessionsScanned:  r.SessionsScanned,
		TotalFileReads:   r.TotalFileReads,
		UniqueFileReads:  r.UniqueFileReads,
		TotalTextBlocks:  r.TotalTextBlocks,
		UniqueTextBlocks: r.UniqueTextBlocks,
		ContinuityIndex:  r.ContinuityIndex,
		TotalFileTokens:  r.TotalFileTokens,
		TotalTextTokens:  r.TotalTextTokens,
		TotalTaxTokens:   r.TotalTaxTokens,
		TotalFileCost:    r.TotalFileCost,
		TotalTextCost:    r.TotalTextCost,
		TotalTaxCost:     r.TotalTaxCost,
	}

	for _, rf := range r.RepeatedFiles {
		out.RepeatedFiles = append(out.RepeatedFiles, RepeatedFileJSON{
			Path:            rf.Path,
			SessionCount:    rf.SessionCount,
			ReadCount:       rf.ReadCount,
			RedundantReads:  rf.RedundantReads,
			Sessions:        rf.Sessions,
			EstimatedTokens: rf.EstimatedTokens,
			EstimatedCost:   rf.EstimatedCost,
		})
	}
	for _, rt := range r.RepeatedTexts {
		out.RepeatedTexts = append(out.RepeatedTexts, RepeatedTextJSON{
			Text:            rt.Text,
			CharCount:       rt.CharCount,
			SessionCount:    rt.SessionCount,
			ReadCount:       rt.ReadCount,
			RedundantReads:  rt.RedundantReads,
			Sessions:        rt.Sessions,
			EstimatedTokens: rt.EstimatedTokens,
			EstimatedCost:   rt.EstimatedCost,
		})
	}
	for _, tp := range r.RepeatTopics {
		out.RepeatTopics = append(out.RepeatTopics, RepeatTopicJSON{
			Files:           tp.Files,
			SessionCount:    tp.SessionCount,
			EstimatedTokens: tp.EstimatedTokens,
			EstimatedCost:   tp.EstimatedCost,
		})
	}
	for _, s := range r.Suggestions {
		out.Suggestions = append(out.Suggestions, ContinuitySuggestJSON{
			Path:            s.Path,
			SessionCount:    s.SessionCount,
			EstimatedTokens: s.EstimatedTokens,
			EstimatedCost:   s.EstimatedCost,
			Reason:          s.Reason,
		})
	}

	if out.RepeatedFiles == nil {
		out.RepeatedFiles = []RepeatedFileJSON{}
	}
	if out.RepeatedTexts == nil {
		out.RepeatedTexts = []RepeatedTextJSON{}
	}
	if out.RepeatTopics == nil {
		out.RepeatTopics = []RepeatTopicJSON{}
	}
	if out.Suggestions == nil {
		out.Suggestions = []ContinuitySuggestJSON{}
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
