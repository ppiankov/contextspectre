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
	fixPreserve  bool
)

var fixCmd = &cobra.Command{
	Use:   "fix [session-id-or-path]",
	Short: "Diagnose and repair session problems",
	Long: `Scan a session for common problems (content filter blocks, oversized images,
orphaned tool results) and optionally repair them.

By default runs in dry-run mode (report only). Use --apply to fix detected issues.
Always creates a backup before any modification.

For Claude for Mac sessions, tombstone mode is enabled automatically (orphaned entries
are replaced with placeholders instead of deleted, preserving scroll-back). Use
--tombstone to force this on CLI sessions too.`,
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

	// Preserve decisions/findings from entries about to be deleted
	if fixPreserve {
		toDelete := make(map[int]bool)
		for _, issue := range diagnosis.Issues {
			toDelete[issue.EntryIndex] = true
		}
		expanded := analyzer.CascadeDeleteSet(entries, toDelete, func(string) bool { return false })
		result, err := editor.Preserve(path, entries, expanded)
		if err != nil {
			slog.Warn("Preserve failed", "err", err)
		} else if result.Decisions > 0 || result.Findings > 0 {
			fmt.Printf("Preserved: %d decisions, %d findings → %s\n",
				result.Decisions, result.Findings, result.OutputPath)
		}
	}

	// Apply repairs: first pass handles non-cascade issues (filter blocks,
	// images), then CascadeDeleteSet pre-computes all orphan/chain cascades
	// in memory for a single Delete() call.
	fmt.Println()
	tombstone := fixTombstone || autoTombstone(path)

	result, err := editor.Repair(path, diagnosis.Issues, tombstone)
	if err != nil {
		return fmt.Errorf("repair: %w", err)
	}
	totalRemoved := result.EntriesRemoved
	totalTombstoned := result.EntriesTombstoned
	totalImages := result.ImagesReplaced
	totalChains := result.ChainRepairs
	totalIssues := len(diagnosis.Issues)

	// Cascade: re-parse after initial repair, expand remaining orphans/chains.
	entries, err = jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("reparse: %w", err)
	}
	diagnosis = analyzer.Diagnose(entries)
	if len(diagnosis.Issues) > 0 {
		totalIssues += len(diagnosis.Issues)
		toDelete := make(map[int]bool)
		toTombstone := make(map[int]bool)
		for _, issue := range diagnosis.Issues {
			switch issue.Kind {
			case analyzer.IssueOrphanedResult:
				if tombstone {
					toTombstone[issue.EntryIndex] = true
				} else {
					toDelete[issue.EntryIndex] = true
				}
			case analyzer.IssueChainBroken:
				toDelete[issue.EntryIndex] = true
			}
		}
		toDelete = analyzer.CascadeDeleteSet(entries, toDelete, func(string) bool { return false })
		if len(toTombstone) > 0 {
			tsResult, err := editor.Tombstone(path, toTombstone)
			if err != nil {
				return fmt.Errorf("cascade tombstone: %w", err)
			}
			totalTombstoned += tsResult.EntriesTombstoned
		}
		if len(toDelete) > 0 {
			dr, err := editor.Delete(path, toDelete)
			if err != nil {
				return fmt.Errorf("cascade: %w", err)
			}
			totalRemoved += dr.EntriesRemoved
			totalChains += dr.EntriesRemoved // chain repairs from cascade
		}
	}

	// Post-repair: coalesce adjacent same-role entries.
	cr, err := editor.Coalesce(path)
	if err != nil {
		slog.Warn("coalesce failed", "err", err)
	}
	coalesced := 0
	if cr != nil {
		coalesced = cr.EntriesRemoved
	}

	if totalTombstoned > 0 {
		fmt.Printf("Repaired: %d entries removed, %d tombstoned, %d images replaced, %d chains repaired",
			totalRemoved, totalTombstoned, totalImages, totalChains)
	} else {
		fmt.Printf("Repaired: %d entries removed, %d images replaced, %d chains repaired",
			totalRemoved, totalImages, totalChains)
	}
	if coalesced > 0 {
		fmt.Printf(", %d coalesced", coalesced)
	}
	fmt.Println()
	slog.Info("Session repaired",
		"path", path,
		"issues", totalIssues,
		"removed", totalRemoved,
		"tombstoned", totalTombstoned,
		"images", totalImages,
		"chains", totalChains,
		"coalesced", coalesced)

	return nil
}

func init() {
	fixCmd.Flags().BoolVar(&fixApply, "apply", false, "Apply repairs (default: dry-run)")
	fixCmd.Flags().BoolVar(&fixCWD, "cwd", false, "Use most recent session for current directory")
	fixCmd.Flags().BoolVar(&fixTombstone, "tombstone", false, "Replace orphaned entries with placeholders instead of deleting (preserves Mac scroll-back)")
	fixCmd.Flags().BoolVar(&fixPreserve, "preserve", false, "Extract decisions and findings before repair (writes .preserved.md sidecar)")
	rootCmd.AddCommand(fixCmd)
}
