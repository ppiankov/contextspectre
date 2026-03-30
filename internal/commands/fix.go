package commands

import (
	"fmt"
	"log/slog"
	"os"
	"time"

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
	fixForce     bool
	fixWait      bool
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
		case analyzer.IssueOrphanedToolUse:
			prefix = "  [tool_use]"
		}
		fmt.Printf("%sline %d: %s\n", prefix, entries[issue.EntryIndex].LineNumber, issue.Description)
	}

	if !fixApply {
		fmt.Println("\nDry run — no changes made. Use --apply to fix.")
		return nil
	}

	// Guard: refuse to repair live sessions unless --force or --wait is set.
	// Active writers race with fix, preventing convergence.
	if !fixForce {
		if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) < 60*time.Second {
			if fixWait {
				if err := waitForIdle(path); err != nil {
					return err
				}
				// Re-parse after waiting — file contents may have changed.
				entries, err = jsonl.Parse(path)
				if err != nil {
					return fmt.Errorf("parse after wait: %w", err)
				}
				diagnosis = analyzer.Diagnose(entries)
				if len(diagnosis.Issues) == 0 {
					fmt.Println("No issues found after waiting.")
					return nil
				}
				fmt.Printf("Found %d issue(s) after waiting:\n\n", len(diagnosis.Issues))
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
					case analyzer.IssueOrphanedToolUse:
						prefix = "  [tool_use]"
					}
					fmt.Printf("%sline %d: %s\n", prefix, entries[issue.EntryIndex].LineNumber, issue.Description)
				}
			} else {
				return fmt.Errorf("session is active (modified %s ago) — fix cannot converge while Claude is writing.\n"+
					"Use --wait to poll until idle, or --force to attempt anyway",
					time.Since(fi.ModTime()).Truncate(time.Second))
			}
		}
	}

	// Split fixable vs unfixable (orphaned tool_use requires rewire, not fix).
	var fixable []analyzer.Issue
	for _, issue := range diagnosis.Issues {
		if isFixable(issue.Kind) {
			fixable = append(fixable, issue)
		}
	}

	// Preserve decisions/findings from entries about to be deleted
	if fixPreserve {
		toDelete := make(map[int]bool)
		for _, issue := range fixable {
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

	result, err := editor.Repair(path, fixable, tombstone)
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
	// Each pass may reveal new chain breaks after patching missing parents
	// or coalescing adjacent entries.
	coalesced := 0
	for cascadePass := 0; cascadePass < 10; cascadePass++ {
		entries, err = jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("reparse: %w", err)
		}
		diagnosis = analyzer.Diagnose(entries)
		fixableCount := 0
		for _, issue := range diagnosis.Issues {
			if isFixable(issue.Kind) {
				fixableCount++
			}
		}
		if fixableCount == 0 {
			// No fixable issues — coalesce and check if that introduced new ones.
			cr, err := editor.Coalesce(path)
			if err != nil {
				slog.Warn("coalesce failed", "err", err)
				break
			}
			if cr != nil {
				coalesced += cr.EntriesRemoved
			}
			if cr == nil || (cr.EntriesRemoved == 0 && cr.OrphansStripped == 0) {
				break // coalesce was a no-op, truly converged
			}
			continue // coalesce changed the file, re-diagnose
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

	// Check for remaining orphaned tool_use blocks that require rewire.
	entries, _ = jsonl.Parse(path)
	if entries != nil {
		diagnosis = analyzer.Diagnose(entries)
		var remaining []analyzer.Issue
		for _, issue := range diagnosis.Issues {
			if issue.Kind == analyzer.IssueOrphanedToolUse {
				remaining = append(remaining, issue)
			}
		}
		if len(remaining) > 0 {
			fmt.Printf("\n%d orphaned tool_use block(s) require rewire:\n", len(remaining))
			for _, issue := range remaining {
				fmt.Printf("  [tool_use] line %d: %s\n",
					entries[issue.EntryIndex].LineNumber, issue.Description)
			}
			fmt.Println("\nRun: contextspectre rewire --apply")
		}
	}

	return nil
}

// waitForIdle polls the session file until it hasn't been modified for 60 seconds.
// Times out after 10 minutes.
func waitForIdle(path string) error {
	const (
		idleThreshold = 60 * time.Second
		pollInterval  = 5 * time.Second
		timeout       = 10 * time.Minute
	)
	deadline := time.Now().Add(timeout)
	fmt.Printf("Waiting for session to idle (no writes for %s)...\n", idleThreshold)

	for time.Now().Before(deadline) {
		fi, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("stat: %w", err)
		}
		idle := time.Since(fi.ModTime())
		if idle >= idleThreshold {
			fmt.Printf("Session idle for %s — proceeding with repair.\n", idle.Truncate(time.Second))
			return nil
		}
		remaining := idleThreshold - idle
		fmt.Printf("\r  last write %s ago, need %s more quiet...  ",
			idle.Truncate(time.Second), remaining.Truncate(time.Second))
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("timed out after %s waiting for session to idle", timeout)
}

// isFixable returns true for issue kinds that Repair() can handle.
// IssueOrphanedToolUse requires rewire (synthetic injection), not fix (deletion/patching).
func isFixable(k analyzer.IssueKind) bool {
	switch k {
	case analyzer.IssueFilterBlock, analyzer.IssueOrphanedResult,
		analyzer.IssueMalformed, analyzer.IssueChainMissingParent,
		analyzer.IssueChainBadStart, analyzer.IssueChainBroken,
		analyzer.IssueOversizedImage, analyzer.IssueMediaTypeMismatch:
		return true
	}
	return false
}

func init() {
	fixCmd.Flags().BoolVar(&fixApply, "apply", false, "Apply repairs (default: dry-run)")
	fixCmd.Flags().BoolVar(&fixCWD, "cwd", false, "Use most recent session for current directory")
	fixCmd.Flags().BoolVar(&fixTombstone, "tombstone", false, "Replace orphaned entries with placeholders instead of deleting (preserves Mac scroll-back)")
	fixCmd.Flags().BoolVar(&fixPreserve, "preserve", false, "Extract decisions and findings before repair (writes .preserved.md sidecar)")
	fixCmd.Flags().BoolVar(&fixForce, "force", false, "Attempt repair even on active sessions (may not converge)")
	fixCmd.Flags().BoolVar(&fixWait, "wait", false, "Wait for session to idle before repairing (polls every 5s, 10m timeout)")
	rootCmd.AddCommand(fixCmd)
}
