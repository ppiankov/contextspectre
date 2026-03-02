package commands

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var fixApply bool

var fixCmd = &cobra.Command{
	Use:   "fix <session-id-or-path>",
	Short: "Diagnose and repair session problems",
	Long: `Scan a session for common problems (content filter blocks, oversized images,
orphaned tool results) and optionally repair them.

By default runs in dry-run mode (report only). Use --apply to fix detected issues.
Always creates a backup before any modification.`,
	Args: cobra.ExactArgs(1),
	RunE: runFix,
}

func runFix(cmd *cobra.Command, args []string) error {
	path := resolveSessionPath(args[0])
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", path)
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
		}
		fmt.Printf("%sline %d: %s\n", prefix, entries[issue.EntryIndex].LineNumber, issue.Description)
	}

	if !fixApply {
		fmt.Println("\nDry run — no changes made. Use --apply to fix.")
		return nil
	}

	// Apply repairs
	fmt.Println()
	result, err := editor.Repair(path, diagnosis.Issues)
	if err != nil {
		return fmt.Errorf("repair: %w", err)
	}

	fmt.Printf("Repaired: %d entries removed, %d images replaced, %d chains repaired\n",
		result.EntriesRemoved, result.ImagesReplaced, result.ChainRepairs)
	slog.Info("Session repaired",
		"path", path,
		"issues", len(diagnosis.Issues),
		"removed", result.EntriesRemoved,
		"images", result.ImagesReplaced,
		"chains", result.ChainRepairs)

	return nil
}

func init() {
	fixCmd.Flags().BoolVar(&fixApply, "apply", false, "Apply repairs (default: dry-run)")
	rootCmd.AddCommand(fixCmd)
}
