package commands

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	rewireApply bool
	rewireCWD   bool
)

var rewireCmd = &cobra.Command{
	Use:   "rewire [session-id-or-path]",
	Short: "Inject synthetic tool_results for orphaned tool_use blocks",
	Long: `Detect and fix orphaned tool_use blocks that have no matching tool_result.

This is the main cause of unrecoverable "API Error: 400 due to tool use
concurrency issues" in Claude Code sessions. When a tool_result is dropped,
the API rejects every subsequent request because the conversation history
is structurally invalid.

Rewire injects synthetic tool_result entries to make the session resumable.

Without --apply, shows what would be fixed (dry run).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRewire,
}

func init() {
	rewireCmd.Flags().BoolVar(&rewireApply, "apply", false, "Apply fixes (inject synthetic tool_results)")
	rewireCmd.Flags().BoolVar(&rewireCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(rewireCmd)
}

func runRewire(_ *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, rewireCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	orphans := editor.FindOrphanedToolUses(entries)
	if len(orphans) == 0 {
		fmt.Println("No orphaned tool_use blocks found — session is clean.")
		return nil
	}

	fmt.Printf("Found %d orphaned tool_use block(s):\n\n", len(orphans))
	for _, o := range orphans {
		fmt.Printf("  entry %d: %s (tool: %s, id: %s)\n", o.EntryIndex, o.ToolUseID, o.ToolName, o.ToolUseID)
	}

	if !rewireApply {
		fmt.Println("\nDry run — use --apply to inject synthetic tool_results.")
		return nil
	}

	result, err := editor.Rewire(path)
	if err != nil {
		return fmt.Errorf("rewire: %w", err)
	}

	fmt.Printf("\nRewired: %d synthetic tool_result(s) injected.\n", result.Injected)
	if result.BytesBefore > 0 {
		delta := result.BytesAfter - result.BytesBefore
		fmt.Printf("Size: %s → %s (+%s)\n",
			humanBytes(result.BytesBefore), humanBytes(result.BytesAfter), humanBytes(delta))
	}

	return nil
}
