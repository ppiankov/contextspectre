package commands

import (
	"fmt"
	"log/slog"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	fixApply     bool
	fixCWD       bool
	fixTombstone bool
)

var fixCmd = &cobra.Command{
	Use:   "fix [session-id-or-path]",
	Short: "Diagnose and repair session problems",
	Long: `Scan a session for common problems (content filter blocks, oversized images,
orphaned tool results) and optionally repair them.

By default runs in dry-run mode (report only). Use --apply to fix detected issues.
Always creates a backup before any modification.

Use --tombstone to replace orphaned entries with placeholders instead of deleting
them. This preserves conversation continuity in Claude for Mac's scroll-back.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFix,
}

func runFix(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, fixCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	diagnosis := analyzer.Diagnose(entries)

	if len(diagnosis.Issues) == 0 {
		fmt.Println("No issues found.")
		return nil
	}

	// Print report
	fmt.Printf("Found %d issue(s):\n\n", len(diagnosis.Issues))
	for _, issue := range diagnosis.Issues {
		prefix := "  "
		switch issue.Kind {
		case analyzer.IssueFilterBlock:
			prefix = "  [filter]  "
		case analyzer.IssueOversizedImage:
			prefix = "  [image]   "
		case analyzer.IssueOrphanedResult:
			prefix = "  [orphan]  "
		case analyzer.IssueMalformed:
			prefix = "  [broken]  "
		case analyzer.IssueChainBroken:
			prefix = "  [chain]   "
		}
		fmt.Printf("%sline %d: %s\n", prefix, entries[issue.EntryIndex].LineNumber, issue.Description)
	}

	if !fixApply {
		fmt.Println("\nDry run — no changes made. Use --apply to fix.")
		return nil
	}

	// Apply repairs — loop until convergence because each fix can cascade
	// (e.g., removing an orphan exposes an assistant chain start, removing
	// that exposes another orphan).
	fmt.Println()
	const maxPasses = 50
	totalRemoved := 0
	totalTombstoned := 0
	totalImages := 0
	totalChains := 0
	totalIssues := len(diagnosis.Issues)

	result, err := editor.Repair(path, diagnosis.Issues, fixTombstone)
	if err != nil {
		return fmt.Errorf("repair: %w", err)
	}
	totalRemoved += result.EntriesRemoved
	totalTombstoned += result.EntriesTombstoned
	totalImages += result.ImagesReplaced
	totalChains += result.ChainRepairs

	for pass := 1; pass < maxPasses; pass++ {
		entries, err = jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("reparse: %w", err)
		}
		diagnosis = analyzer.Diagnose(entries)
		if len(diagnosis.Issues) == 0 {
			break
		}
		totalIssues += len(diagnosis.Issues)
		cascadeResult, err := editor.Repair(path, diagnosis.Issues, fixTombstone)
		if err != nil {
			return fmt.Errorf("cascade repair: %w", err)
		}
		totalRemoved += cascadeResult.EntriesRemoved
		totalTombstoned += cascadeResult.EntriesTombstoned
		totalImages += cascadeResult.ImagesReplaced
		totalChains += cascadeResult.ChainRepairs
	}

	if totalTombstoned > 0 {
		fmt.Printf("Repaired: %d entries removed, %d tombstoned, %d images replaced, %d chains repaired\n",
			totalRemoved, totalTombstoned, totalImages, totalChains)
	} else {
		fmt.Printf("Repaired: %d entries removed, %d images replaced, %d chains repaired\n",
			totalRemoved, totalImages, totalChains)
	}
	slog.Info("Session repaired",
		"path", path,
		"issues", totalIssues,
		"removed", totalRemoved,
		"tombstoned", totalTombstoned,
		"images", totalImages,
		"chains", totalChains)

	return nil
}

func init() {
	fixCmd.Flags().BoolVar(&fixApply, "apply", false, "Apply repairs (default: dry-run)")
	fixCmd.Flags().BoolVar(&fixCWD, "cwd", false, "Use most recent session for current directory")
	fixCmd.Flags().BoolVar(&fixTombstone, "tombstone", false, "Replace orphaned entries with placeholders instead of deleting (preserves Mac scroll-back)")
	rootCmd.AddCommand(fixCmd)
}
