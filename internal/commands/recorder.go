package commands

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var recorderCWD bool

var recorderCmd = &cobra.Command{
	Use:   "flight-recorder [session-id-or-path]",
	Short: "Show structured reasoning event timeline for a session",
	Long: `Extract a structured timeline of reasoning events from a session.
Events include compaction points, context spikes, file bursts, and decisions.

  contextspectre flight-recorder --cwd
  contextspectre flight-recorder <session-id>`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRecorder,
}

func runRecorder(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, recorderCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	stats := analyzer.Analyze(entries)
	fr := analyzer.ComputeFlightRecord(entries, stats)

	if isJSON() {
		return printJSON(fr)
	}

	if fr.TotalTurns < 2 {
		fmt.Println("Session too short for flight recording.")
		return nil
	}

	fmt.Printf("Reasoning Flight Recorder\n\n")
	fmt.Printf("  Turns:        %d\n", fr.TotalTurns)
	fmt.Printf("  Tokens:       %s\n", formatTokenCount(fr.TotalTokens))
	fmt.Printf("  Half-life:    %d turns\n", fr.HalfLife)
	fmt.Printf("  Signal grade: %s\n", fr.SignalGrade)

	if len(fr.Epochs) > 0 {
		fmt.Printf("\nEpochs\n")
		for _, ep := range fr.Epochs {
			fmt.Printf("  %2d  %-16s  turns %3d-%3d  %8s tokens  %3d files\n",
				ep.Index, ep.Label, ep.Start, ep.End,
				formatTokenCount(ep.Tokens), ep.Files)
		}
	}

	if len(fr.Events) > 0 {
		fmt.Printf("\nEvents\n")
		for _, ev := range fr.Events {
			fmt.Printf("  t=%-4d  %-14s  %s\n", ev.Turn, ev.Type, ev.Detail)
		}
	}

	return nil
}

func init() {
	recorderCmd.Flags().BoolVar(&recorderCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(recorderCmd)
}
