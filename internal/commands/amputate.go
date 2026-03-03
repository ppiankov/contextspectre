package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	amputateFrom  int
	amputateTo    int
	amputateLast  int
	amputateAfter int
	amputateApply bool
)

var (
	amputateCWD bool
)

var amputateCmd = &cobra.Command{
	Use:   "amputate [session-id-or-path]",
	Short: "Surgically remove entries to unblock stuck sessions",
	Long: `Remove a range of entries from a session to unblock content filter blocks
or other corruption. Always creates a backup before any modification.

By default runs in dry-run mode (preview only). Use --apply to execute.

Examples:
  contextspectre amputate <id> --from 45 --to 50           # preview removal
  contextspectre amputate <id> --from 45 --to 50 --apply   # execute
  contextspectre amputate <id> --last 3 --apply            # remove last 3 entries
  contextspectre amputate --cwd --last 3 --apply           # use current directory's session`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAmputate,
}

func runAmputate(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, amputateCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("session is empty")
	}

	// Resolve range from flags
	from, to, err := resolveAmputateRange(len(entries))
	if err != nil {
		return err
	}

	// Build deletion set
	toDelete := make(map[int]bool, to-from+1)
	for i := from; i <= to; i++ {
		toDelete[i] = true
	}

	// Compute impact
	stats := analyzer.Analyze(entries)
	impact := analyzer.PredictImpact(entries, toDelete, stats)

	// Count types in range
	typeCounts := make(map[jsonl.MessageType]int)
	for i := from; i <= to; i++ {
		typeCounts[entries[i].Type]++
	}

	if isJSON() {
		out := AmputateOutput{
			SessionID:      sessionIDFromPath(path),
			From:           from,
			To:             to,
			EntriesRemoved: to - from + 1,
			TokensSaved:    impact.EstimatedTokenSaved,
			ChainRepairs:   impact.ChainRepairs,
			DryRun:         !amputateApply,
		}
		if amputateApply {
			result, err := editor.Delete(path, toDelete)
			if err != nil {
				return fmt.Errorf("amputate: %w", err)
			}
			out.EntriesRemoved = result.EntriesRemoved
			out.ChainRepairs = result.ChainRepairs
			out.DryRun = false
		}
		return printJSON(out)
	}

	// Print identity and preview
	printSessionIdentity(path)
	fmt.Printf("Amputate: entries %d-%d (%d entries)\n", from, to, to-from+1)

	// Type breakdown
	if len(typeCounts) > 0 {
		fmt.Print("  Types:   ")
		first := true
		for typ, count := range typeCounts {
			if !first {
				fmt.Print(", ")
			}
			fmt.Printf("%d %s", count, typ)
			first = false
		}
		fmt.Println()
	}

	fmt.Printf("  Tokens:   ~%s\n", formatTokens(impact.EstimatedTokenSaved))
	if impact.ChainRepairs > 0 {
		fmt.Printf("  Chains:   %d parentUuid repairs needed\n", impact.ChainRepairs)
	}
	if len(impact.Warnings) > 0 {
		for _, w := range impact.Warnings {
			fmt.Printf("  Warning:  %s\n", w)
		}
	}

	if !amputateApply {
		fmt.Println("\nDry run — use --apply to execute.")
		return nil
	}

	// Execute
	result, err := editor.Delete(path, toDelete)
	if err != nil {
		return fmt.Errorf("amputate: %w", err)
	}
	fmt.Printf("\nAmputated %d entries, %d chain repairs, saved %s\n",
		result.EntriesRemoved, result.ChainRepairs,
		formatBytes(result.BytesBefore-result.BytesAfter))
	return nil
}

func resolveAmputateRange(total int) (int, int, error) {
	hasFromTo := amputateFrom >= 0 && amputateTo >= 0
	hasLast := amputateLast > 0
	hasAfter := amputateAfter >= 0

	// Exactly one mode
	modes := 0
	if hasFromTo {
		modes++
	}
	if hasLast {
		modes++
	}
	if hasAfter {
		modes++
	}
	if modes == 0 {
		return 0, 0, fmt.Errorf("specify --from/--to, --last N, or --after N")
	}
	if modes > 1 {
		return 0, 0, fmt.Errorf("use only one of --from/--to, --last, or --after")
	}

	var from, to int
	switch {
	case hasFromTo:
		from, to = amputateFrom, amputateTo
	case hasLast:
		from = total - amputateLast
		to = total - 1
		if from < 0 {
			fmt.Fprintf(os.Stderr, "Warning: --last %d exceeds total entries (%d), clamping to 0\n", amputateLast, total)
			from = 0
		}
	case hasAfter:
		from = amputateAfter
		to = total - 1
	}

	if from < 0 || to < 0 {
		return 0, 0, fmt.Errorf("indices must be non-negative")
	}
	if from > to {
		return 0, 0, fmt.Errorf("--from (%d) must be <= --to (%d)", from, to)
	}
	if to >= total {
		return 0, 0, fmt.Errorf("--to (%d) exceeds total entries (%d)", to, total)
	}
	if from == 0 && to == total-1 {
		return 0, 0, fmt.Errorf("cannot amputate entire session (%d entries)", total)
	}
	return from, to, nil
}

func sessionIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl")
}

func init() {
	amputateCmd.Flags().IntVar(&amputateFrom, "from", -1, "Start entry index (inclusive)")
	amputateCmd.Flags().IntVar(&amputateTo, "to", -1, "End entry index (inclusive)")
	amputateCmd.Flags().IntVar(&amputateLast, "last", 0, "Remove the last N entries")
	amputateCmd.Flags().IntVar(&amputateAfter, "after", -1, "Remove all entries from this index onward")
	amputateCmd.Flags().BoolVar(&amputateApply, "apply", false, "Execute the amputation (default: dry-run)")
	amputateCmd.Flags().BoolVar(&amputateCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(amputateCmd)
}
