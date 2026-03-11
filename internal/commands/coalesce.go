package commands

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	coalesceApply bool
	coalesceCWD   bool
)

var coalesceCmd = &cobra.Command{
	Use:   "coalesce [session-id-or-path]",
	Short: "Merge adjacent same-role entries to fix tool_result adjacency errors",
	Long: `Merges consecutive same-role messages (user+user, assistant+assistant)
into single entries by combining their content blocks.

This fixes API errors where tool_result blocks don't match the immediately
preceding assistant's tool_use blocks — common in Claude for Mac sessions
that split multi-tool calls across separate JSONL entries.

Dry run by default. Use --apply to modify the session file.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCoalesce,
}

func init() {
	coalesceCmd.Flags().BoolVar(&coalesceApply, "apply", false, "Actually merge entries (default: dry run)")
	coalesceCmd.Flags().BoolVar(&coalesceCWD, "cwd", false, "Auto-detect session from current working directory")
	rootCmd.AddCommand(coalesceCmd)
}

// CoalesceOutputJSON is the JSON output for the coalesce command.
type CoalesceOutputJSON struct {
	SessionID       string `json:"session_id"`
	GroupsMerged    int    `json:"groups_merged"`
	EntriesRemoved  int    `json:"entries_removed"`
	OrphansStripped int    `json:"orphans_stripped"`
	BytesBefore     int64  `json:"bytes_before"`
	BytesAfter      int64  `json:"bytes_after"`
	Applied         bool   `json:"applied"`
}

func runCoalesce(_ *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, coalesceCWD)
	if err != nil {
		return err
	}

	// Dry-run: count adjacent same-role groups without modifying.
	if !coalesceApply {
		groups, entries, err := countCoalesceTargets(path)
		if err != nil {
			return err
		}

		if isJSON() {
			return printJSON(CoalesceOutputJSON{
				SessionID:      sessionIDFromPath(path),
				GroupsMerged:   groups,
				EntriesRemoved: entries,
				Applied:        false,
			})
		}

		if groups == 0 {
			fmt.Println("No adjacent same-role entries found — nothing to coalesce.")
			return nil
		}

		fmt.Printf("Found %d merge groups (%d entries would be absorbed).\n", groups, entries)
		fmt.Println("Dry run — use --apply to merge.")
		return nil
	}

	result, err := editor.Coalesce(path)
	if err != nil {
		return fmt.Errorf("coalesce: %w", err)
	}

	if isJSON() {
		return printJSON(CoalesceOutputJSON{
			SessionID:       sessionIDFromPath(path),
			GroupsMerged:    result.GroupsMerged,
			EntriesRemoved:  result.EntriesRemoved,
			OrphansStripped: result.OrphansStripped,
			BytesBefore:     result.BytesBefore,
			BytesAfter:      result.BytesAfter,
			Applied:         true,
		})
	}

	if result.GroupsMerged == 0 {
		fmt.Println("No adjacent same-role entries found — nothing to coalesce.")
		return nil
	}

	fmt.Printf("Coalesced %d groups (%d entries merged, %d orphaned tool_results stripped).\n",
		result.GroupsMerged, result.EntriesRemoved, result.OrphansStripped)
	saved := result.BytesBefore - result.BytesAfter
	if saved > 0 {
		fmt.Printf("Size: %.1f KB → %.1f KB (saved %.1f KB)\n",
			float64(result.BytesBefore)/1024, float64(result.BytesAfter)/1024, float64(saved)/1024)
	}
	return nil
}

// countCoalesceTargets counts merge groups without modifying the file.
func countCoalesceTargets(path string) (groups, absorbed int, err error) {
	entries, _, err := jsonl.ParseRaw(path)
	if err != nil {
		return 0, 0, fmt.Errorf("parse: %w", err)
	}

	i := 0
	for i < len(entries) {
		role := coalescableRole(entries[i])
		if role == "" {
			i++
			continue
		}

		mergeCount := 1
		j := i + 1
		for j < len(entries) {
			jr := coalescableRole(entries[j])
			if jr == role {
				mergeCount++
				j++
			} else if jr == "" && isTransparentEntry(entries[j]) {
				j++
			} else {
				break
			}
		}

		if mergeCount > 1 {
			groups++
			absorbed += mergeCount - 1
		}
		i = j
	}
	return groups, absorbed, nil
}

func coalescableRole(e jsonl.Entry) string {
	if e.Type != jsonl.TypeUser && e.Type != jsonl.TypeAssistant {
		return ""
	}
	if e.Message == nil {
		return ""
	}
	return e.Message.Role
}

func isTransparentEntry(e jsonl.Entry) bool {
	return e.Type == jsonl.TypeSystem || e.Type == jsonl.TypeQueueOperation
}
