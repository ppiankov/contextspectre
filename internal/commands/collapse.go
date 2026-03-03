package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/spf13/cobra"
)

var (
	collapseCommitPoint string
	collapseDryRun      bool
)

var collapseCmd = &cobra.Command{
	Use:   "collapse <session-id-or-path>",
	Short: "Collapse entries above a commit point",
	Long: `Collapse a session at a commit point boundary, removing all entries
marked as CANDIDATE above the commit point UUID. Always creates a backup.

Set commit points in the TUI (p key) or via the mark command.
Use 'mark <session> --list' to see commit points.`,
	Args: cobra.ExactArgs(1),
	RunE: runCollapse,
}

func runCollapse(cmd *cobra.Command, args []string) error {
	if collapseCommitPoint == "" {
		return fmt.Errorf("--commit-point is required")
	}

	path := resolveSessionPath(args[0])
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", path)
	}

	if collapseDryRun {
		count, err := editor.CollapsePreview(path, collapseCommitPoint)
		if err != nil {
			return fmt.Errorf("preview: %w", err)
		}
		if isJSON() {
			return printJSON(CollapseOutput{
				SessionID:       filepath.Base(path),
				CommitPointUUID: collapseCommitPoint,
				EntriesRemoved:  count,
				DryRun:          true,
			})
		}
		fmt.Printf("Dry run: would remove %d CANDIDATE entries above commit point %s\n",
			count, collapseCommitPoint)
		return nil
	}

	result, err := editor.Collapse(path, collapseCommitPoint)
	if err != nil {
		return fmt.Errorf("collapse: %w", err)
	}

	if isJSON() {
		return printJSON(CollapseOutput{
			SessionID:       filepath.Base(path),
			CommitPointUUID: collapseCommitPoint,
			EntriesRemoved:  result.EntriesRemoved,
			ChainRepairs:    result.ChainRepairs,
			BytesSaved:      result.BytesBefore - result.BytesAfter,
		})
	}

	if result.EntriesRemoved == 0 {
		fmt.Println("No CANDIDATE entries to collapse above commit point.")
		return nil
	}

	fmt.Printf("Collapsed: %d entries removed, %d chain repairs, %s saved\n",
		result.EntriesRemoved, result.ChainRepairs,
		formatBytes(result.BytesBefore-result.BytesAfter))
	return nil
}

func init() {
	collapseCmd.Flags().StringVar(&collapseCommitPoint, "commit-point", "", "UUID of the commit point to collapse at")
	collapseCmd.Flags().BoolVar(&collapseDryRun, "dry-run", false, "Show what would be removed without modifying")
	rootCmd.AddCommand(collapseCmd)
}
