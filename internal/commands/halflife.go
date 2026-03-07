package commands

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var halflifeCWD bool

var halflifeCmd = &cobra.Command{
	Use:   "half-life [session-id-or-path]",
	Short: "Measure reasoning half-life of a session",
	Long: `Reasoning Half-Life (RHL) measures how quickly reasoning tokens become
irrelevant. Short RHL = volatile exploration. Long RHL = stable, coherent reasoning.

  contextspectre half-life --cwd
  contextspectre half-life <session-id>`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHalfLife,
}

func runHalfLife(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, halflifeCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	stats := analyzer.Analyze(entries)
	hl := analyzer.ComputeHalfLife(entries, stats.Compactions)

	if isJSON() {
		return printJSON(hl)
	}

	if hl.TotalTurns < 2 {
		fmt.Println("Session too short for half-life analysis.")
		return nil
	}

	fmt.Printf("Reasoning Half-Life\n\n")
	fmt.Printf("  Total turns:      %d\n", hl.TotalTurns)
	fmt.Printf("  Total tokens:     %s\n", formatTokenCount(hl.TotalTokens))
	fmt.Printf("  Half-life:        %d turns\n", hl.HalfLife)
	fmt.Printf("  Dead context:     %.1f%%\n", hl.DeadContextPct)

	var label string
	switch {
	case hl.HalfLife < 20:
		label = "volatile — compact aggressively"
	case hl.HalfLife < 50:
		label = "normal"
	case hl.HalfLife < 100:
		label = "stable"
	default:
		label = "very stable"
	}
	fmt.Printf("  Assessment:       %s\n", label)

	if len(hl.Epochs) > 1 {
		fmt.Printf("\nEpochs:\n")
		for _, ep := range hl.Epochs {
			fmt.Printf("  %2d  turns %3d-%3d  %8s tokens  %3d files  decay:%.0f%%  dead:%.0f%%\n",
				ep.Index, ep.StartTurn, ep.EndTurn,
				formatTokenCount(ep.Tokens), ep.FileTouches,
				ep.FileDecay*100, ep.DeadPct)
		}
	}

	return nil
}

func init() {
	halflifeCmd.Flags().BoolVar(&halflifeCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(halflifeCmd)
}
