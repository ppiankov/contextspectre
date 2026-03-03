package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	exportBranches string
	exportOutput   string
	exportWipe     bool
)

var exportCmd = &cobra.Command{
	Use:   "export <session-id-or-path>",
	Short: "Export branches to markdown",
	Long: `Export selected conversation branches to a standalone markdown file.

Use 'contextspectre sessions' to find session IDs. Open in TUI to see branches.

Examples:
  contextspectre export <id> --branches all
  contextspectre export <id> --branches 1,3,5 --output context.md
  contextspectre export <id> --branches 2,4 --wipe`,
	Args: cobra.ExactArgs(1),
	RunE: runExport,
}

func runExport(cmd *cobra.Command, args []string) error {
	path := resolveSessionPath(args[0])
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", path)
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	stats := analyzer.Analyze(entries)
	branches := analyzer.FindBranches(entries, stats.Compactions)
	if len(branches) == 0 {
		return fmt.Errorf("no branches found in session")
	}

	// Parse branch indices
	var indices []int
	if exportBranches != "all" {
		for _, s := range strings.Split(exportBranches, ",") {
			s = strings.TrimSpace(s)
			idx, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Errorf("invalid branch index %q: %w", s, err)
			}
			if idx < 0 || idx >= len(branches) {
				return fmt.Errorf("branch index %d out of range (0-%d)", idx, len(branches)-1)
			}
			indices = append(indices, idx)
		}
	}

	// Default output path
	if exportOutput == "" {
		exportOutput = fmt.Sprintf("branch-export-%s.md", time.Now().Format("2006-01-02-150405"))
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	result, err := editor.ExportBranches(entries, branches, indices, sessionID, exportOutput)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	// Optional wipe
	var wipeResult *editor.DeleteResult
	if exportWipe {
		toDelete := make(map[int]bool)
		selected := indices
		if len(selected) == 0 {
			for i := range branches {
				selected = append(selected, i)
			}
		}
		for _, idx := range selected {
			br := branches[idx]
			for i := br.StartIdx; i <= br.EndIdx; i++ {
				toDelete[i] = true
			}
		}
		wipeResult, err = editor.Delete(path, toDelete)
		if err != nil {
			return fmt.Errorf("wipe: %w", err)
		}
	}

	if isJSON() {
		out := ExportOutput{
			SessionID:        sessionID,
			BranchesExported: result.BranchesExported,
			EntriesExtracted: result.EntriesExtracted,
			TokenCost:        result.TokenCost,
			DollarCost:       result.DollarCost,
			OutputPath:       result.OutputPath,
			Wiped:            exportWipe,
		}
		if wipeResult != nil {
			out.WipeResult = &SplitCleanJSON{
				EntriesRemoved: wipeResult.EntriesRemoved,
				ChainRepairs:   wipeResult.ChainRepairs,
			}
		}
		return printJSON(out)
	}

	fmt.Printf("Exported %d branches (%d entries, ~%s tokens) to %s\n",
		result.BranchesExported, result.EntriesExtracted,
		formatTokens(result.TokenCost), result.OutputPath)
	if wipeResult != nil {
		fmt.Printf("Wiped: %d entries removed, %d chain repairs\n",
			wipeResult.EntriesRemoved, wipeResult.ChainRepairs)
	}

	return nil
}

func init() {
	exportCmd.Flags().StringVar(&exportBranches, "branches", "", "Branch indices (comma-separated) or 'all'")
	exportCmd.Flags().StringVar(&exportOutput, "output", "", "Output markdown file path (default: branch-export-<date>.md)")
	exportCmd.Flags().BoolVar(&exportWipe, "wipe", false, "Delete exported entries from session after export")
	_ = exportCmd.MarkFlagRequired("branches")
	rootCmd.AddCommand(exportCmd)
}
