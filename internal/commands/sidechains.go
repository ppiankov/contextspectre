package commands

import (
	"fmt"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var sidechainsCWD bool

var sidechainsCmd = &cobra.Command{
	Use:   "sidechains [session-id-or-path]",
	Short: "Report structural sidechains in a session",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSidechains,
}

func runSidechains(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, sidechainsCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	report := analyzer.DetectSidechains(entries)

	if isJSON() {
		return printJSON(buildSidechainOutput(report))
	}

	if report.TotalEntries == 0 {
		fmt.Println("No sidechains detected.")
		return nil
	}

	fmt.Printf("Sidechains: %d entries (~%s tokens) across %d groups\n",
		report.TotalEntries, formatTokens(report.TotalTokens), report.GroupCount)
	fmt.Printf("  Repairable: %d\n", report.RepairableCount)
	fmt.Printf("  Prune-only: %d\n", report.PruneOnlyCount)
	fmt.Println()

	for _, sc := range report.Entries {
		reasons := make([]string, 0, len(sc.Reasons))
		for _, r := range sc.Reasons {
			reasons = append(reasons, string(r))
		}
		fmt.Printf("[%d] line %d  %-10s ~%s  %s\n",
			sc.EntryIndex, sc.LineNumber, sc.Classification, formatTokens(sc.TokenCost),
			strings.Join(reasons, ","))
		if sc.ToolUseID != "" {
			fmt.Printf("      tool_use_id: %s\n", sc.ToolUseID)
		}
		if sc.ReconnectParent != "" {
			fmt.Printf("      reconnect:   parentUuid -> %s\n", sc.ReconnectParent)
		}
		if sc.Preview != "" {
			fmt.Printf("      preview:     %s\n", sc.Preview)
		}
	}

	return nil
}

func buildSidechainOutput(report *analyzer.SidechainReport) SidechainOutputJSON {
	out := SidechainOutputJSON{
		Entries: []SidechainEntryJSON{},
	}
	if report == nil {
		return out
	}

	out.TotalEntries = report.TotalEntries
	out.TotalTokens = report.TotalTokens
	out.GroupCount = report.GroupCount
	out.RepairableCount = report.RepairableCount
	out.PruneOnlyCount = report.PruneOnlyCount

	for _, e := range report.Entries {
		reasons := make([]string, 0, len(e.Reasons))
		for _, r := range e.Reasons {
			reasons = append(reasons, string(r))
		}
		out.Entries = append(out.Entries, SidechainEntryJSON{
			EntryIndex:      e.EntryIndex,
			LineNumber:      e.LineNumber,
			UUID:            e.UUID,
			ParentUUID:      e.ParentUUID,
			ToolUseID:       e.ToolUseID,
			TokenCost:       e.TokenCost,
			Preview:         e.Preview,
			Reasons:         reasons,
			Classification:  e.Classification,
			ReconnectParent: e.ReconnectParent,
		})
	}

	return out
}

func init() {
	sidechainsCmd.Flags().BoolVar(&sidechainsCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(sidechainsCmd)
}
