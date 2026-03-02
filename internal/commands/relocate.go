package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	relocateFrom      string
	relocateTo        string
	relocateApply     bool
	relocateUpdateCWD bool
	relocateScan      bool
)

var relocateCmd = &cobra.Command{
	Use:   "relocate",
	Short: "Relocate sessions after a project directory move",
	Long: `Relocate sessions when a project directory has been moved or renamed.

Scan for orphaned sessions:
  contextspectre relocate --scan

Plan a relocation (dry-run by default):
  contextspectre relocate --from /old/path --to /new/path

Execute the relocation:
  contextspectre relocate --from /old/path --to /new/path --apply

Also update CWD fields in JSONL:
  contextspectre relocate --from /old/path --to /new/path --apply --update-cwd`,
	RunE: runRelocate,
}

// RelocateJSON is the JSON output for the relocate command.
type RelocateJSON struct {
	Plan    *RelocatePlanJSON   `json:"plan,omitempty"`
	Result  *RelocateResultJSON `json:"result,omitempty"`
	Orphans []OrphanJSON        `json:"orphans,omitempty"`
}

// RelocatePlanJSON is the dry-run plan.
type RelocatePlanJSON struct {
	OldPath      string `json:"old_path"`
	NewPath      string `json:"new_path"`
	OldDirName   string `json:"old_dir_name"`
	NewDirName   string `json:"new_dir_name"`
	SessionCount int    `json:"session_count"`
	IndexEntries int    `json:"index_entries"`
	OldDirExists bool   `json:"old_dir_exists"`
	NewDirExists bool   `json:"new_dir_exists"`
}

// RelocateResultJSON is the result of an applied relocation.
type RelocateResultJSON struct {
	OldDirName    string `json:"old_dir_name"`
	NewDirName    string `json:"new_dir_name"`
	SessionsFound int    `json:"sessions_found"`
	IndexUpdated  bool   `json:"index_updated"`
	CWDUpdated    int    `json:"cwd_updated"`
}

// OrphanJSON is a single orphaned project.
type OrphanJSON struct {
	DirName      string `json:"dir_name"`
	DecodedPath  string `json:"decoded_path"`
	SessionCount int    `json:"session_count"`
}

func runRelocate(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()

	if relocateScan {
		return runOrphanScan(dir)
	}

	if relocateFrom == "" || relocateTo == "" {
		return fmt.Errorf("both --from and --to are required (or use --scan to find orphans)")
	}

	if !relocateApply {
		return runRelocateDryRun(dir)
	}

	return runRelocateApply(dir)
}

func runOrphanScan(claudeDir string) error {
	orphans, err := session.FindOrphans(claudeDir)
	if err != nil {
		return fmt.Errorf("scan orphans: %w", err)
	}

	if isJSON() {
		out := RelocateJSON{}
		for _, o := range orphans {
			out.Orphans = append(out.Orphans, OrphanJSON{
				DirName:      o.DirName,
				DecodedPath:  o.DecodedPath,
				SessionCount: o.SessionCount,
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	if len(orphans) == 0 {
		fmt.Println("No orphaned sessions found.")
		return nil
	}

	fmt.Println("Orphaned sessions (project path no longer exists):")
	fmt.Println()
	for _, o := range orphans {
		fmt.Printf("  %s (%d sessions)\n", o.DecodedPath, o.SessionCount)
		fmt.Printf("    dir: %s\n", o.DirName)
	}
	fmt.Println()
	fmt.Println("Use --from and --to to relocate:")
	fmt.Println("  contextspectre relocate --from /old/path --to /new/path --apply")
	return nil
}

func runRelocateDryRun(claudeDir string) error {
	plan, err := session.PlanRelocate(claudeDir, relocateFrom, relocateTo)
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}

	if isJSON() {
		out := RelocateJSON{
			Plan: &RelocatePlanJSON{
				OldPath:      plan.OldPath,
				NewPath:      plan.NewPath,
				OldDirName:   plan.OldDirName,
				NewDirName:   plan.NewDirName,
				SessionCount: plan.SessionCount,
				IndexEntries: plan.IndexEntries,
				OldDirExists: plan.OldDirExists,
				NewDirExists: plan.NewDirExists,
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	if !plan.OldDirExists {
		return fmt.Errorf("source directory not found: %s", plan.OldDirName)
	}

	fmt.Println("Relocation plan:")
	fmt.Printf("  Directory: %s -> %s\n", plan.OldDirName, plan.NewDirName)
	fmt.Printf("  Sessions: %d\n", plan.SessionCount)
	fmt.Printf("  Index entries: %d\n", plan.IndexEntries)
	if plan.NewDirExists {
		fmt.Println("  WARNING: Target directory already exists!")
	}
	if relocateUpdateCWD {
		fmt.Println("  CWD update: will rewrite JSONL entries")
	} else {
		fmt.Println("  CWD update: skipped (use --update-cwd to rewrite)")
	}
	fmt.Println()
	fmt.Println("  Run with --apply to execute.")
	return nil
}

func runRelocateApply(claudeDir string) error {
	result, err := session.Relocate(claudeDir, relocateFrom, relocateTo, relocateUpdateCWD)
	if err != nil {
		return fmt.Errorf("relocate: %w", err)
	}

	if isJSON() {
		out := RelocateJSON{
			Result: &RelocateResultJSON{
				OldDirName:    result.OldDirName,
				NewDirName:    result.NewDirName,
				SessionsFound: result.SessionsFound,
				IndexUpdated:  result.IndexUpdated,
				CWDUpdated:    result.CWDUpdated,
			},
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("Renamed directory: %s -> %s\n", result.OldDirName, result.NewDirName)
	fmt.Printf("Sessions found: %d\n", result.SessionsFound)
	if result.IndexUpdated {
		fmt.Println("Updated sessions-index.json")
	}
	if result.CWDUpdated > 0 {
		fmt.Printf("Updated CWD in %d JSONL entries\n", result.CWDUpdated)
	}
	fmt.Println("Relocation complete.")
	return nil
}

func init() {
	relocateCmd.Flags().StringVar(&relocateFrom, "from", "", "Original project path")
	relocateCmd.Flags().StringVar(&relocateTo, "to", "", "New project path")
	relocateCmd.Flags().BoolVar(&relocateApply, "apply", false, "Execute the relocation (default: dry-run)")
	relocateCmd.Flags().BoolVar(&relocateUpdateCWD, "update-cwd", false, "Also rewrite CWD fields in JSONL entries")
	relocateCmd.Flags().BoolVar(&relocateScan, "scan", false, "Scan for orphaned sessions")
	rootCmd.AddCommand(relocateCmd)
}
