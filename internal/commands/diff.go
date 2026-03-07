package commands

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	diffCWD  bool
	diffFrom int
	diffTo   int
)

var reasoningDiffCmd = &cobra.Command{
	Use:   "reasoning-diff [session-id-or-path]",
	Short: "Compare reasoning states between epochs",
	Long: `Show what changed structurally between two reasoning epochs:
files added/dropped, tool usage shifts, scope changes.

  contextspectre reasoning-diff --cwd --from 0 --to 3
  contextspectre reasoning-diff <session-id> --from 1 --to 5
  contextspectre reasoning-diff <sessionA> <sessionB>`,
	Args: cobra.MaximumNArgs(2),
	RunE: runReasoningDiff,
}

func runReasoningDiff(cmd *cobra.Command, args []string) error {
	// Cross-session mode: two positional args.
	if len(args) == 2 {
		return runCrossSessionDiff(args[0], args[1])
	}

	// Within-session mode.
	path, err := resolveSessionArg(args, diffCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	stats := analyzer.Analyze(entries)
	numEpochs := len(stats.Compactions) + 1

	// Default --to to last epoch.
	toEpoch := diffTo
	if toEpoch < 0 {
		toEpoch = numEpochs - 1
	}

	if diffFrom < 0 || diffFrom >= numEpochs {
		return fmt.Errorf("--from epoch %d out of range (0-%d)", diffFrom, numEpochs-1)
	}
	if toEpoch < 0 || toEpoch >= numEpochs {
		return fmt.Errorf("--to epoch %d out of range (0-%d)", toEpoch, numEpochs-1)
	}
	if diffFrom == toEpoch {
		return fmt.Errorf("--from and --to must be different epochs")
	}

	fromState := analyzer.ExtractEpochState(entries, stats.Compactions, diffFrom)
	toState := analyzer.ExtractEpochState(entries, stats.Compactions, toEpoch)
	if fromState == nil || toState == nil {
		return fmt.Errorf("failed to extract epoch states")
	}

	diff := analyzer.ComputeReasoningDiff(fromState, toState)
	if isJSON() {
		return printJSON(diff)
	}

	renderReasoningDiff(diff)
	return nil
}

func runCrossSessionDiff(argA, argB string) error {
	pathA := resolveSessionPath(argA)
	pathB := resolveSessionPath(argB)

	entriesA, err := jsonl.Parse(pathA)
	if err != nil {
		return fmt.Errorf("parse A: %w", err)
	}
	entriesB, err := jsonl.Parse(pathB)
	if err != nil {
		return fmt.Errorf("parse B: %w", err)
	}

	statsA := analyzer.Analyze(entriesA)
	statsB := analyzer.Analyze(entriesB)

	// Use last epoch of each session for comparison.
	lastA := len(statsA.Compactions)
	lastB := len(statsB.Compactions)

	stateA := analyzer.ExtractEpochState(entriesA, statsA.Compactions, lastA)
	stateB := analyzer.ExtractEpochState(entriesB, statsB.Compactions, lastB)
	if stateA == nil || stateB == nil {
		return fmt.Errorf("failed to extract session states")
	}

	diff := analyzer.ComputeReasoningDiff(stateA, stateB)
	if isJSON() {
		return printJSON(diff)
	}

	renderReasoningDiff(diff)
	return nil
}

func renderReasoningDiff(diff *analyzer.ReasoningDiff) {
	fmt.Printf("Reasoning Diff\n\n")
	fmt.Printf("  From:  epoch %d (turns %d-%d, %s tokens)\n",
		diff.From.Epoch, diff.From.TurnRange[0], diff.From.TurnRange[1],
		formatTokenCount(diff.From.Tokens))
	fmt.Printf("  To:    epoch %d (turns %d-%d, %s tokens)\n",
		diff.To.Epoch, diff.To.TurnRange[0], diff.To.TurnRange[1],
		formatTokenCount(diff.To.Tokens))
	fmt.Printf("  Scope: %s\n", diff.ScopeChange)

	if diff.TokenDelta != 0 {
		sign := "+"
		if diff.TokenDelta < 0 {
			sign = ""
		}
		fmt.Printf("  Token delta: %s%s\n", sign, formatTokenCount(diff.TokenDelta))
	}

	if len(diff.FilesAdded) > 0 {
		fmt.Printf("\nFiles added (%d)\n", len(diff.FilesAdded))
		for _, f := range diff.FilesAdded {
			fmt.Printf("  + %s\n", shortenPath(f))
		}
	}

	if len(diff.FilesDropped) > 0 {
		fmt.Printf("\nFiles dropped (%d)\n", len(diff.FilesDropped))
		for _, f := range diff.FilesDropped {
			fmt.Printf("  - %s\n", shortenPath(f))
		}
	}

	if len(diff.FilesKept) > 0 {
		fmt.Printf("\nFiles kept (%d)\n", len(diff.FilesKept))
		limit := 10
		if len(diff.FilesKept) < limit {
			limit = len(diff.FilesKept)
		}
		for i := 0; i < limit; i++ {
			fmt.Printf("    %s\n", shortenPath(diff.FilesKept[i]))
		}
		if len(diff.FilesKept) > limit {
			fmt.Printf("    ... and %d more\n", len(diff.FilesKept)-limit)
		}
	}

	if len(diff.ToolShifts) > 0 {
		fmt.Printf("\nTool shifts\n")
		limit := 8
		if len(diff.ToolShifts) < limit {
			limit = len(diff.ToolShifts)
		}
		for i := 0; i < limit; i++ {
			ts := diff.ToolShifts[i]
			fmt.Printf("  %-14s %3d → %3d  (%s)\n",
				ts.Tool, ts.FromCount, ts.ToCount, ts.Direction)
		}
	}
}

// shortenPath strips common home directory prefixes for display.
func shortenPath(path string) string {
	// Strip absolute paths to show relative-like form.
	if len(path) > 60 {
		// Keep last 60 chars.
		return "..." + path[len(path)-57:]
	}
	return path
}

func init() {
	reasoningDiffCmd.Flags().BoolVar(&diffCWD, "cwd", false, "Use most recent session for current directory")
	reasoningDiffCmd.Flags().IntVar(&diffFrom, "from", 0, "Source epoch index")
	reasoningDiffCmd.Flags().IntVar(&diffTo, "to", -1, "Target epoch index (-1 = last)")
	rootCmd.AddCommand(reasoningDiffCmd)
}
