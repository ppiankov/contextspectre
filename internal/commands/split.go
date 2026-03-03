package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	splitFrom   int
	splitTo     int
	splitOutput string
	splitClean  bool
)

var splitCWD bool

var splitCmd = &cobra.Command{
	Use:   "split [session-id-or-path]",
	Short: "Extract a tangent sequence to markdown",
	Long: `Extract entries from a session into a standalone markdown file.

Use 'stats --scope' to identify tangent sequences, then split them out.

Examples:
  contextspectre split <id> --from 45 --to 82 --output tangent.md
  contextspectre split --cwd --from 45 --to 82 --output tangent.md --clean`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSplit,
}

func runSplit(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, splitCWD)
	if err != nil {
		return err
	}

	if splitFrom < 0 || splitTo < 0 {
		return fmt.Errorf("--from and --to must be non-negative")
	}
	if splitFrom > splitTo {
		return fmt.Errorf("--from (%d) must be <= --to (%d)", splitFrom, splitTo)
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if splitTo >= len(entries) {
		return fmt.Errorf("--to (%d) out of range (session has %d entries, indices 0-%d)",
			splitTo, len(entries), len(entries)-1)
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	// Compute metadata for the range
	cwd := analyzer.DetectSessionCWD(entries)
	meta := analyzer.ComputeRangeMetadata(entries, splitFrom, splitTo, cwd)

	// Print cost summary
	if !isJSON() {
		if meta.TargetRepo != "" {
			fmt.Printf("Split: entries %d-%d → %s\n", splitFrom, splitTo, meta.TargetRepo)
		} else {
			fmt.Printf("Split: entries %d-%d\n", splitFrom, splitTo)
		}
		fmt.Printf("  Entries:  %d\n", splitTo-splitFrom+1)
		fmt.Printf("  Tokens:   ~%s\n", formatTokens(meta.TokenCost))
		fmt.Printf("  Cost:     %s\n", analyzer.FormatCost(meta.DollarCost))
		if len(meta.ReExplFiles) > 0 {
			fmt.Printf("  Re-explanation files: %d\n", len(meta.ReExplFiles))
		}
		fmt.Printf("  Output:   %s\n", splitOutput)
		fmt.Println()
	}

	// Write markdown
	result, err := editor.SplitToMarkdown(entries, splitFrom, splitTo, meta, sessionID, splitOutput)
	if err != nil {
		return fmt.Errorf("split: %w", err)
	}

	// Optional: clean extracted entries from session
	var cleanResult *editor.DeleteResult
	if splitClean {
		toDelete := make(map[int]bool, splitTo-splitFrom+1)
		for i := splitFrom; i <= splitTo; i++ {
			toDelete[i] = true
		}
		cleanResult, err = editor.Delete(path, toDelete)
		if err != nil {
			return fmt.Errorf("clean after split: %w", err)
		}
	}

	if isJSON() {
		out := SplitOutput{
			SessionID:        sessionID,
			From:             splitFrom,
			To:               splitTo,
			EntriesExtracted: result.EntriesExtracted,
			TargetRepo:       result.TargetRepo,
			TokenCost:        result.TokenCost,
			DollarCost:       result.DollarCost,
			ReExplFiles:      result.ReExplFiles,
			OutputPath:       result.OutputPath,
			Cleaned:          splitClean,
		}
		if cleanResult != nil {
			out.CleanResult = &SplitCleanJSON{
				EntriesRemoved: cleanResult.EntriesRemoved,
				ChainRepairs:   cleanResult.ChainRepairs,
			}
		}
		return printJSON(out)
	}

	fmt.Printf("Extracted %d entries to %s\n", result.EntriesExtracted, result.OutputPath)
	if cleanResult != nil {
		fmt.Printf("Cleaned: %d entries removed, %d chain repairs\n",
			cleanResult.EntriesRemoved, cleanResult.ChainRepairs)
	}

	return nil
}

func init() {
	splitCmd.Flags().IntVar(&splitFrom, "from", -1, "Start entry index (inclusive)")
	splitCmd.Flags().IntVar(&splitTo, "to", -1, "End entry index (inclusive)")
	splitCmd.Flags().StringVar(&splitOutput, "output", "", "Output markdown file path")
	splitCmd.Flags().BoolVar(&splitClean, "clean", false, "Remove extracted entries from session after export")
	_ = splitCmd.MarkFlagRequired("from")
	_ = splitCmd.MarkFlagRequired("to")
	_ = splitCmd.MarkFlagRequired("output")
	splitCmd.Flags().BoolVar(&splitCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(splitCmd)
}
