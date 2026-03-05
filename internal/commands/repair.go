package commands

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	repairApply     bool
	repairReconnect bool
	repairPrune     bool
	repairCWD       bool
)

var repairCmd = &cobra.Command{
	Use:   "repair [session-id-or-path]",
	Short: "Repair or prune structurally orphaned sidechains",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRepair,
}

func runRepair(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, repairCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	report := analyzer.DetectSidechains(entries)

	if report.TotalEntries == 0 {
		if isJSON() {
			return printJSON(buildSidechainOutput(report))
		}
		fmt.Println("No sidechains detected.")
		return nil
	}

	if !repairApply {
		if isJSON() {
			return printJSON(buildSidechainOutput(report))
		}
		fmt.Printf("Dry run: %d sidechains (~%s tokens)\n", report.TotalEntries, formatTokens(report.TotalTokens))
		fmt.Printf("  repairable: %d | prune-only: %d\n", report.RepairableCount, report.PruneOnlyCount)
		fmt.Println("Use --apply --reconnect to repair parent chains or --apply --prune to remove sidechains.")
		return nil
	}

	if repairReconnect == repairPrune {
		return fmt.Errorf("with --apply, choose exactly one mode: --reconnect or --prune")
	}

	if repairReconnect {
		res, err := editor.ReconnectSidechains(path, report)
		if err != nil {
			return fmt.Errorf("reconnect sidechains: %w", err)
		}
		if isJSON() {
			return printJSON(map[string]any{
				"mode":                 "reconnect",
				"entries_reconnected":  res.EntriesReconnected,
				"sidechains_remaining": report.TotalEntries,
			})
		}
		fmt.Printf("Reconnected %d sidechain entries.\n", res.EntriesReconnected)
		return nil
	}

	toDelete := analyzer.SidechainIndexSet(report)
	delResult, err := editor.Delete(path, toDelete)
	if err != nil {
		return fmt.Errorf("prune sidechains: %w", err)
	}
	if isJSON() {
		return printJSON(map[string]any{
			"mode":            "prune",
			"entries_removed": delResult.EntriesRemoved,
			"chain_repairs":   delResult.ChainRepairs,
			"tokens_saved":    (delResult.BytesBefore - delResult.BytesAfter) / 4,
		})
	}
	fmt.Printf("Pruned %d sidechain entries, repaired %d chains, saved ~%s tokens.\n",
		delResult.EntriesRemoved,
		delResult.ChainRepairs,
		formatTokens(int((delResult.BytesBefore-delResult.BytesAfter)/4)))
	return nil
}

func init() {
	repairCmd.Flags().BoolVar(&repairApply, "apply", false, "Apply the selected repair mode (default: dry-run)")
	repairCmd.Flags().BoolVar(&repairReconnect, "reconnect", false, "Reconnect orphaned parent chains where possible")
	repairCmd.Flags().BoolVar(&repairPrune, "prune", false, "Prune all detected sidechain entries")
	repairCmd.Flags().BoolVar(&repairCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(repairCmd)
}
