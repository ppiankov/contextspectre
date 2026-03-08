package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	injectionCWD bool
	injectionCmd = &cobra.Command{
		Use:   "injection [session-id-or-path]",
		Short: "Detect vector injection patterns in session content",
		Long: `Scans tool results for structural injection patterns: embedded directives,
zero-width unicode, imperative language, system-like tags, and role confusion.
Detection only — no prevention or filtering (mirrors, not oracles).`,
		Args: cobra.MaximumNArgs(1),
		RunE: runInjection,
	}
)

func init() {
	injectionCmd.Flags().BoolVar(&injectionCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(injectionCmd)
}

func runInjection(_ *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, injectionCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	report := analyzer.DetectInjection(entries)

	if isJSON() {
		return printJSON(report)
	}

	fmt.Println("Injection Scan")
	fmt.Printf("  Session: %s\n", filepath.Base(path))
	fmt.Println()
	fmt.Printf("  Risk score:     %.0f/100\n", report.RiskScore)
	fmt.Printf("  Findings:       %d\n", len(report.Findings))
	fmt.Printf("  Highest:        %s\n", report.HighestSev)

	if len(report.Findings) == 0 {
		fmt.Println("\n  No injection patterns detected.")
		return nil
	}

	fmt.Println()

	// Group by severity for display
	sevOrder := []string{"critical", "high", "medium", "low"}
	for _, sev := range sevOrder {
		for _, f := range report.Findings {
			if f.Severity != sev {
				continue
			}
			label := strings.ToUpper(f.Severity[:3])
			if f.Severity == "critical" {
				label = "CRIT"
			}
			context := f.Context
			if len(context) > 60 {
				context = context[:57] + "..."
			}
			fmt.Printf("  %-4s  t=%-4d %-10s %s: %q\n",
				label, f.Turn, f.ToolName, f.Kind, context)
		}
	}

	return nil
}
