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
		case analyzer.IssueChainBroken, analyzer.IssueChainMissingParent, analyzer.IssueChainBadStart:
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
	totalPatches := result.ParentPatches
	totalIssues := len(diagnosis.Issues)

	// Cascade: re-parse and repair until no more issues (max 10 passes).
	// Each pass may reveal new chain breaks after patching missing parents.
	for cascadePass := 0; cascadePass < 10; cascadePass++ {
		entries, err = jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("reparse: %w", err)
		}
		diagnosis = analyzer.Diagnose(entries)
		if len(diagnosis.Issues) == 0 {
			break
		}
		totalIssues += len(diagnosis.Issues)
		toDeleteChain := make(map[int]bool)
		toDeleteOther := make(map[int]bool)
		toTombstone := make(map[int]bool)
		toPatchParent := make(map[int]bool)
		for _, issue := range diagnosis.Issues {
			switch issue.Kind {
			case analyzer.IssueOrphanedResult:
				if tombstone {
					toTombstone[issue.EntryIndex] = true
				} else {
					toDeleteOther[issue.EntryIndex] = true
				}
			case analyzer.IssueChainMissingParent:
				toPatchParent[issue.EntryIndex] = true
			case analyzer.IssueChainBadStart, analyzer.IssueChainBroken:
				toDeleteChain[issue.EntryIndex] = true
			}
		}
		if len(toPatchParent) > 0 {
			patched, err := editor.PatchParentUUID(path, toPatchParent)
			if err != nil {
				return fmt.Errorf("cascade patch parent: %w", err)
			}
			totalPatches += patched
		}
		// Only cascade-expand non-chain deletes (orphans). Chain issues are
		// handled by Delete's built-in parent repair — cascading them destroys
		// sidechains and prevents convergence.
		if len(toDeleteOther) > 0 {
			toDeleteOther = analyzer.CascadeDeleteSet(entries, toDeleteOther, func(string) bool { return false })
		}
		for idx := range toDeleteChain {
			toDeleteOther[idx] = true
		}
		if len(toTombstone) > 0 {
			tsResult, err := editor.Tombstone(path, toTombstone)
			if err != nil {
				return fmt.Errorf("cascade tombstone: %w", err)
			}
			totalTombstoned += tsResult.EntriesTombstoned
		}
		if len(toDeleteOther) > 0 {
			dr, err := editor.Delete(path, toDeleteOther)
			if err != nil {
				return fmt.Errorf("cascade: %w", err)
			}
			totalRemoved += dr.EntriesRemoved
			totalChains += dr.ChainRepairs
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
	if totalPatches > 0 {
		fmt.Printf(", %d parents reconnected", totalPatches)
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
		"parents_patched", totalPatches,
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
