package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	launchCWD       bool
	launchDryRun    bool
	launchCleanOnly bool
	launchPrint     bool
	launchWait      bool
	launchForce     bool
)

var launchCmd = &cobra.Command{
	Use:   "launch [session-id-or-path]",
	Short: "Fix, clean, checkpoint, then resume a session in Claude",
	Long: `Pre-flight cleanup for session resume. Runs fix, clean --all, and checkpoint
before exec-ing into claude --resume. The session must be idle (not actively
used by Claude) for fix to converge.

Pipeline: checkpoint → fix --apply → clean --all → claude --resume`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLaunch,
}

func init() {
	launchCmd.Flags().BoolVar(&launchCWD, "cwd", false, "Use most recent session for current directory")
	launchCmd.Flags().BoolVar(&launchDryRun, "dry-run", false, "Show what would be done without modifying or launching")
	launchCmd.Flags().BoolVar(&launchCleanOnly, "clean-only", false, "Skip fix, only clean")
	launchCmd.Flags().BoolVar(&launchPrint, "print", false, "Launch with claude -p --resume (headless mode)")
	launchCmd.Flags().BoolVar(&launchWait, "wait", false, "Wait for session to idle before launching (polls every 5s, 10m timeout)")
	launchCmd.Flags().BoolVar(&launchForce, "force", false, "Skip the active-session check")
	rootCmd.AddCommand(launchCmd)
}

func runLaunch(_ *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, launchCWD)
	if err != nil {
		return err
	}

	// Session identity (needed for process check).
	base := filepath.Base(path)
	fullID := strings.TrimSuffix(base, ".jsonl")
	shortID := fullID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	// Refuse if session is active (unless --force).
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if !launchForce && time.Since(fi.ModTime()) < 60*time.Second {
		if launchWait {
			if err := waitForIdle(path); err != nil {
				return err
			}
			// Re-stat after waiting.
			fi, err = os.Stat(path)
			if err != nil {
				return fmt.Errorf("stat after wait: %w", err)
			}
		} else {
			return fmt.Errorf("session is active (modified %s ago) — wait for it to idle before launching.\n"+
				"Use --wait to poll until idle, or --force to skip this check",
				time.Since(fi.ModTime()).Truncate(time.Second))
		}
	}

	// Check if another claude process already has this session open.
	if !launchForce {
		if pid, err := findClaudeProcess(fullID, ""); err == nil && pid > 0 {
			return fmt.Errorf("session already open by claude (PID %d) — close it first or use --force", pid)
		}
	}

	stats, err := jsonl.ScanLight(path)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	clientType := "unknown"
	if stats.TypeCounts[jsonl.TypeFileHistorySnapshot] > 0 {
		clientType = "cli"
	} else if stats.StartsWithQueueOp {
		clientType = "desktop"
	} else if stats.LineCount > 100 {
		clientType = "cli"
	}

	displayName := stats.CustomTitle
	if displayName == "" {
		displayName = stats.Slug
	}

	// Summary line.
	nameStr := shortID
	if displayName != "" {
		nameStr = fmt.Sprintf("%s (%s)", shortID, displayName)
	}
	clientLabel := strings.ToUpper(clientType[:1]) + clientType[1:]
	fmt.Printf("Session:  %s [%s]\n", nameStr, clientLabel)
	fmt.Printf("Entries:  %d | Size: %s\n", stats.LineCount, humanBytes(fi.Size()))

	if launchDryRun {
		fmt.Println("\nDry run — would run: checkpoint → fix → clean → claude --resume")
		return nil
	}

	// Step 1: Checkpoint.
	fmt.Print("\nCheckpoint... ")
	cpPath, err := runLaunchCheckpoint(path)
	if err != nil {
		fmt.Printf("skipped (%v)\n", err)
	} else if cpPath != "" {
		fmt.Printf("saved → %s\n", cpPath)
	} else {
		fmt.Println("done (stdout)")
	}

	// Step 2: Fix.
	if !launchCleanOnly {
		fmt.Print("Fix...        ")
		fixResult, err := runLaunchFix(path)
		if err != nil {
			fmt.Printf("error: %v\n", err)
		} else if fixResult == "" {
			fmt.Println("clean")
		} else {
			fmt.Println(fixResult)
		}
	}

	// Step 2b: Rewire — inject synthetic tool_results for orphaned tool_use blocks.
	if !launchCleanOnly {
		fmt.Print("Rewire...     ")
		rewireResult, err := editor.Rewire(path)
		if err != nil {
			fmt.Printf("error: %v\n", err)
		} else if rewireResult.Injected == 0 {
			fmt.Println("clean")
		} else {
			fmt.Printf("%d tool_result(s) injected\n", rewireResult.Injected)
		}
	}

	// Step 3: Clean.
	fmt.Print("Clean...      ")
	cleanResult, err := runLaunchClean(path, clientType == "desktop")
	if err != nil {
		fmt.Printf("error: %v\n", err)
	} else {
		fmt.Println(cleanResult)
	}

	// Step 4: Launch.
	// Use -r (short flag) because --resume is broken in Claude CLI v2.1.x.
	// Interactive: use slug/custom-title as the resume argument.
	// Print mode: use full UUID (headless, no picker).
	resumeArg := fullID
	if displayName != "" {
		resumeArg = displayName
	}
	if launchPrint {
		fmt.Printf("\nLaunching: claude -p -r %s\n", fullID)
		return execClaude([]string{"claude", "-p", "-r", fullID})
	}
	fmt.Printf("\nLaunching: claude -r %s\n", resumeArg)
	return execClaude([]string{"claude", "-r", resumeArg})
}

// runLaunchCheckpoint saves a checkpoint to docs/context.txt if the docs/ dir exists.
func runLaunchCheckpoint(path string) (string, error) {
	entries, err := jsonl.Parse(path)
	if err != nil {
		return "", err
	}

	stats := analyzer.Analyze(entries)

	// Find active epoch entries.
	activeStart := 0
	if stats.LastCompactionLine > 0 {
		activeStart = stats.LastCompactionLine
	}
	activeEntries := entries[activeStart:]
	epochSummary := extractCheckpointData(activeEntries)

	// Only write if there's something worth saving.
	if len(epochSummary.decisions) == 0 && len(epochSummary.findings) == 0 && len(epochSummary.files) == 0 {
		return "", nil
	}

	// Determine output path.
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	docsDir := filepath.Join(cwd, "docs")
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		// No docs/ dir — skip file write.
		return "", nil
	}
	outPath := filepath.Join(docsDir, "context.txt")

	// Build session identity for the brief.
	base := filepath.Base(path)
	sessionID := strings.TrimSuffix(base, ".jsonl")
	project := extractProjectFromPath(path)

	var slug string
	for _, e := range entries {
		if e.CustomTitle != "" {
			slug = e.CustomTitle
		} else if slug == "" && e.Slug != "" {
			slug = e.Slug
		}
	}

	activeHint := ""
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Type == jsonl.TypeUser && entries[i].Message != nil {
			activeHint = entries[i].ContentPreview(60)
			break
		}
	}
	epochs := analyzer.BuildEpochs(stats.EpochCosts, stats.Archaeology, activeHint)

	var activeEpoch CheckpointEpoch
	if len(epochs) > 0 {
		last := epochs[len(epochs)-1]
		activeEpoch = CheckpointEpoch{
			Index:      last.Index,
			TurnCount:  last.TurnCount,
			PeakTokens: last.PeakTokens,
			Cost:       last.Cost,
			Topic:      last.Topic,
		}
	}

	markers, _ := editor.LoadMarkers(path)
	var commitPoints []CheckpointCP
	for _, cp := range markers.CommitPoints {
		commitPoints = append(commitPoints, CheckpointCP{
			Goal:      cp.Goal,
			Decisions: cp.Decisions,
			Files:     cp.Files,
		})
	}

	output := CheckpointOutput{
		SessionID:      sessionID,
		Slug:           slug,
		Project:        project,
		ClientType:     stats.ClientType,
		Timestamp:      time.Now().Format(time.RFC3339),
		ContextPercent: stats.UsagePercent,
		TurnsRemaining: stats.EstimatedTurnsLeft,
		Epoch:          activeEpoch,
		Decisions:      epochSummary.decisions,
		Findings:       epochSummary.findings,
		Questions:      epochSummary.questions,
		Files:          epochSummary.files,
		CommitPoints:   commitPoints,
	}

	brief := renderCheckpointBrief(output)
	if err := os.WriteFile(outPath, []byte(brief), 0o644); err != nil {
		return "", err
	}
	return outPath, nil
}

// runLaunchFix runs fix --apply and returns a summary string.
func runLaunchFix(path string) (string, error) {
	entries, err := jsonl.Parse(path)
	if err != nil {
		return "", err
	}

	diagnosis := analyzer.Diagnose(entries)
	var fixable []analyzer.Issue
	for _, issue := range diagnosis.Issues {
		if isFixable(issue.Kind) {
			fixable = append(fixable, issue)
		}
	}
	if len(fixable) == 0 {
		return "", nil
	}

	tombstone := autoTombstone(path)
	result, err := editor.Repair(path, fixable, tombstone)
	if err != nil {
		return "", err
	}

	totalRemoved := result.EntriesRemoved
	totalChains := result.ChainRepairs
	totalPatches := result.ParentPatches

	// Cascade passes.
	for pass := 0; pass < 10; pass++ {
		entries, err = jsonl.Parse(path)
		if err != nil {
			return "", err
		}
		diagnosis = analyzer.Diagnose(entries)
		fixableCount := 0
		for _, issue := range diagnosis.Issues {
			if isFixable(issue.Kind) {
				fixableCount++
			}
		}
		if fixableCount == 0 {
			cr, _ := editor.Coalesce(path)
			if cr == nil || (cr.EntriesRemoved == 0 && cr.OrphansStripped == 0) {
				break
			}
			continue
		}

		toPatchParent := make(map[int]bool)
		toDeleteChain := make(map[int]bool)
		toDeleteOther := make(map[int]bool)
		for _, issue := range diagnosis.Issues {
			switch issue.Kind {
			case analyzer.IssueChainMissingParent:
				toPatchParent[issue.EntryIndex] = true
			case analyzer.IssueChainBadStart, analyzer.IssueChainBroken:
				toDeleteChain[issue.EntryIndex] = true
			case analyzer.IssueOrphanedResult:
				if tombstone {
					// skip tombstone tracking for launch simplicity
				} else {
					toDeleteOther[issue.EntryIndex] = true
				}
			}
		}
		if len(toPatchParent) > 0 {
			patched, err := editor.PatchParentUUID(path, toPatchParent)
			if err != nil {
				return "", err
			}
			totalPatches += patched
		}
		if len(toDeleteOther) > 0 {
			toDeleteOther = analyzer.CascadeDeleteSet(entries, toDeleteOther, func(string) bool { return false })
		}
		for idx := range toDeleteChain {
			toDeleteOther[idx] = true
		}
		if len(toDeleteOther) > 0 {
			dr, err := editor.Delete(path, toDeleteOther)
			if err != nil {
				return "", err
			}
			totalRemoved += dr.EntriesRemoved
			totalChains += dr.ChainRepairs
		}
	}

	parts := []string{}
	if totalRemoved > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", totalRemoved))
	}
	if totalChains > 0 {
		parts = append(parts, fmt.Sprintf("%d chains repaired", totalChains))
	}
	if totalPatches > 0 {
		parts = append(parts, fmt.Sprintf("%d parents reconnected", totalPatches))
	}
	if len(parts) == 0 {
		return "done", nil
	}
	return strings.Join(parts, ", "), nil
}

// runLaunchClean runs clean --all and returns a summary string.
func runLaunchClean(path string, isDesktop bool) (string, error) {
	opts := editor.CleanAllOpts{Tombstone: isDesktop}
	result, err := editor.CleanAll(path, opts)
	if err != nil {
		return "", err
	}

	totalRemoved := result.ProgressRemoved + result.SnapshotsRemoved +
		result.SidechainsRemoved + result.TangentsRemoved +
		result.FailedRetries + result.StaleReadsRemoved +
		result.OrphansRemoved
	saved := result.BytesBefore - result.BytesAfter

	totalSurgery := result.ThinkingRemoved + result.RemindersDeduped + result.MegaBlocksTruncated

	if totalRemoved == 0 && result.ImagesReplaced == 0 && result.CoalesceMerged == 0 && totalSurgery == 0 {
		return "already clean", nil
	}

	parts := []string{}
	if totalRemoved > 0 {
		parts = append(parts, fmt.Sprintf("%d entries removed", totalRemoved))
	}
	if result.ImagesReplaced > 0 {
		parts = append(parts, fmt.Sprintf("%d images replaced", result.ImagesReplaced))
	}
	if result.CoalesceMerged > 0 {
		parts = append(parts, fmt.Sprintf("%d coalesced", result.CoalesceMerged))
	}
	if result.ThinkingRemoved > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking blocks", result.ThinkingRemoved))
	}
	if result.RemindersDeduped > 0 {
		parts = append(parts, fmt.Sprintf("%d reminders deduped", result.RemindersDeduped))
	}
	if result.MegaBlocksTruncated > 0 {
		parts = append(parts, fmt.Sprintf("%d mega blocks truncated", result.MegaBlocksTruncated))
	}
	if saved > 0 {
		parts = append(parts, fmt.Sprintf("%s freed", humanBytes(saved)))
	}
	return strings.Join(parts, ", "), nil
}

// humanBytes formats bytes as a human-readable string.
func humanBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
