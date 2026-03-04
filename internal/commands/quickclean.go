package commands

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	quickCleanProject    string
	quickCleanLive       bool
	quickCleanAggressive bool
	quickCleanCWD        bool
)

var quickCleanCmd = &cobra.Command{
	Use:   "quick-clean",
	Short: "Find and clean the most recent session",
	Long: `Discovers the most recently modified session and runs cleanup.

Examples:
  contextspectre quick-clean                    # clean --all on most recent session
  contextspectre quick-clean --project myproj   # scoped to a specific project
  contextspectre quick-clean --live             # live cleanup (Tier 1-3) on active session
  contextspectre quick-clean --live --cwd       # live cleanup scoped to current directory's project
  contextspectre quick-clean --live --aggressive # live cleanup (Tier 1-5)`,
	Args: cobra.NoArgs,
	RunE: runQuickClean,
}

func runQuickClean(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("discover sessions: %w", err)
	}

	if quickCleanAggressive && !quickCleanLive {
		return fmt.Errorf("--aggressive can only be used with --live")
	}

	// CWD-based project scoping: --cwd flag or auto-detect inside Claude Code
	if quickCleanCWD || os.Getenv("CLAUDECODE") == "1" {
		cwd, err := os.Getwd()
		if err == nil && cwd != "" {
			encodedDir := session.EncodePath(cwd)
			projectDir := filepath.Join(dir, "projects", encodedDir)
			var filtered []session.Info
			for _, s := range sessions {
				if strings.HasPrefix(s.FullPath, projectDir+"/") {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) > 0 {
				sessions = filtered
				slog.Debug("CWD filter applied", "cwd", cwd, "matched", len(filtered))
			} else {
				fmt.Fprintf(os.Stderr, "Warning: no sessions found for CWD %s, falling back to most recent\n", cwd)
			}
		}
	}

	// Filter by project if specified (alias-aware)
	if quickCleanProject != "" {
		sessions = resolveProjectSessions(sessions, quickCleanProject, resolveClaudeDir())
	}

	if len(sessions) == 0 {
		if isJSON() {
			return printJSON(map[string]string{"status": "no_sessions"})
		}
		fmt.Println("No sessions found.")
		return nil
	}

	target := sessions[0]
	path := target.FullPath
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session file not found: %s", path)
	}

	if !isJSON() {
		printSessionIdentity(path)
	}

	if quickCleanLive {
		return runQuickCleanLive(path, target)
	}

	return runQuickCleanAll(path, target)
}

func runQuickCleanAll(path string, target session.Info) error {
	result, err := editor.CleanAll(path)
	if err != nil {
		return fmt.Errorf("quick-clean: %w", err)
	}

	if isJSON() {
		out := cleanAllToJSON(path, result)
		out.Mode = "quick-clean"
		return printJSON(out)
	}

	totalOps := result.ProgressRemoved + result.SnapshotsRemoved + result.SidechainsRemoved +
		result.TangentsRemoved + result.FailedRetries + result.StaleReadsRemoved +
		result.ImagesReplaced + result.SeparatorsStripped + result.OutputsTruncated
	if totalOps == 0 {
		fmt.Printf("Session %s (%s): nothing to clean\n", target.SessionID, target.ProjectName)
		return nil
	}

	fmt.Printf("Cleaned session %s (%s): %d entries removed, ~%d tokens saved, %s\n",
		target.SessionID, target.ProjectName,
		result.ProgressRemoved+result.SnapshotsRemoved+result.SidechainsRemoved+
			result.TangentsRemoved+result.FailedRetries+result.StaleReadsRemoved,
		result.TotalTokensSaved,
		formatBytes(result.BytesBefore-result.BytesAfter))
	printSavingsLine(recordCleanupSavings(path, result.TotalTokensSaved))
	slog.Info("Quick-clean complete", "session", target.SessionID, "project", target.ProjectName, "tokens", result.TotalTokensSaved)
	return nil
}

func runQuickCleanLive(path string, target session.Info) error {
	opts := editor.CleanLiveOpts{
		Aggressive: quickCleanAggressive,
	}
	result, err := editor.CleanLive(path, opts)
	if err != nil {
		if errors.Is(err, editor.ErrRaceDetected) {
			return fmt.Errorf("aborted: Claude Code wrote to session during cleanup (file restored from backup)")
		}
		if errors.Is(err, editor.ErrSessionNotIdle) {
			return fmt.Errorf("session %s is actively being written to — wait a few seconds and retry", target.SessionID)
		}
		return fmt.Errorf("quick-clean live: %w", err)
	}

	if isJSON() {
		out := cleanLiveToJSON(path, result)
		out.Mode = "quick-clean-live"
		return printJSON(out)
	}

	fmt.Printf("Live cleaned session %s (%s): %d prog, %d snap",
		target.SessionID, target.ProjectName,
		result.ProgressRemoved, result.SnapshotsRemoved)
	if quickCleanAggressive {
		fmt.Printf(", %d img, %d sep, %d trunc",
			result.ImagesReplaced, result.SeparatorsStripped, result.OutputsTruncated)
	}
	fmt.Println()
	fmt.Printf("Total saved: ~%d tokens, %s\n",
		result.TotalTokensSaved, formatBytes(result.BytesBefore-result.BytesAfter))
	printSavingsLine(recordCleanupSavings(path, result.TotalTokensSaved))
	slog.Info("Quick-clean live complete", "session", target.SessionID, "project", target.ProjectName, "tokens", result.TotalTokensSaved)
	return nil
}

func init() {
	quickCleanCmd.Flags().StringVar(&quickCleanProject, "project", "", "Scope to a specific project name")
	quickCleanCmd.Flags().BoolVar(&quickCleanLive, "live", false, "Live cleanup for active sessions (Tier 1-3)")
	quickCleanCmd.Flags().BoolVar(&quickCleanAggressive, "aggressive", false, "Include Tier 4-5 operations (use with --live)")
	quickCleanCmd.Flags().BoolVar(&quickCleanCWD, "cwd", false, "Scope to current directory's project (auto-detected inside Claude Code)")
	rootCmd.AddCommand(quickCleanCmd)
}
