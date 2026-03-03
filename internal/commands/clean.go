package commands

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	cleanImages        bool
	cleanProgress      bool
	cleanSeparators    bool
	cleanSnapshots     bool
	cleanDedupReads    bool
	cleanTruncate      bool
	cleanOutThreshold  int
	cleanOutKeepLines  int
	cleanFailedRetries bool
	cleanSidechains    bool
	cleanTangents      bool
	cleanAll           bool
	cleanLive          bool
	cleanAggressive    bool
	cleanAuto          bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean [session-id-or-path]",
	Short: "Clean a session (replace images, remove progress)",
	Long: `Clean a conversation session by replacing base64 images with tiny
placeholders or removing progress messages. Always creates a backup first.

Use --auto to automatically find and clean the most recent session:
  contextspectre clean --auto`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClean,
}

func runClean(cmd *cobra.Command, args []string) error {
	if !cleanImages && !cleanProgress && !cleanSeparators && !cleanSnapshots && !cleanDedupReads && !cleanTruncate && !cleanFailedRetries && !cleanSidechains && !cleanTangents && !cleanAll && !cleanLive && !cleanAuto {
		return fmt.Errorf("specify at least one clean operation flag")
	}

	if cleanAggressive && !cleanLive {
		return fmt.Errorf("--aggressive can only be used with --live")
	}

	if cleanAuto && len(args) > 0 {
		return fmt.Errorf("--auto does not accept a session argument (it finds the most recent session)")
	}
	if !cleanAuto && len(args) == 0 {
		return fmt.Errorf("session argument required (or use --auto to find the most recent session)")
	}

	// --auto: find the most recent session and run --all
	if cleanAuto {
		return runCleanAuto()
	}

	path := resolveSessionPath(args[0])
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", path)
	}

	if cleanLive {
		if cleanAll || cleanImages || cleanProgress || cleanSeparators || cleanSnapshots ||
			cleanDedupReads || cleanTruncate || cleanFailedRetries || cleanSidechains || cleanTangents {
			return fmt.Errorf("--live cannot be combined with --all or individual operation flags")
		}
		if !isJSON() {
			printSessionIdentity(path)
		}
		return runCleanLive(path)
	}

	if !isJSON() {
		printSessionIdentity(path)
	}

	if cleanAll {
		result, err := editor.CleanAll(path)
		if err != nil {
			return fmt.Errorf("clean all: %w", err)
		}
		if isJSON() {
			return printJSON(cleanAllToJSON(path, result))
		}
		fmt.Printf("Cleaned: %d prog, %d snap, %d chain, %d tangent, %d retry, %d stale, %d img, %d sep, %d trunc\n",
			result.ProgressRemoved, result.SnapshotsRemoved, result.SidechainsRemoved,
			result.TangentsRemoved, result.FailedRetries, result.StaleReadsRemoved,
			result.ImagesReplaced, result.SeparatorsStripped, result.OutputsTruncated)
		fmt.Printf("Total saved: ~%d tokens, %s\n",
			result.TotalTokensSaved, formatBytes(result.BytesBefore-result.BytesAfter))
		slog.Info("Clean all complete", "tokens", result.TotalTokensSaved)
		return nil
	}

	if cleanImages {
		result, err := editor.ReplaceImages(path)
		if err != nil {
			return fmt.Errorf("replace images: %w", err)
		}
		if result.ImagesReplaced > 0 {
			fmt.Printf("Replaced %d images, saved %s\n",
				result.ImagesReplaced,
				formatBytes(result.BytesSaved))
			slog.Info("Images replaced", "count", result.ImagesReplaced, "saved", result.BytesSaved)
		} else {
			fmt.Println("No images to replace.")
		}
	}

	if cleanProgress {
		result, err := editor.RemoveProgress(path)
		if err != nil {
			return fmt.Errorf("remove progress: %w", err)
		}
		if result.EntriesRemoved > 0 {
			fmt.Printf("Removed %d progress messages\n", result.EntriesRemoved)
			slog.Info("Progress removed", "count", result.EntriesRemoved)
		} else {
			fmt.Println("No progress messages to remove.")
		}
	}

	if cleanSeparators {
		result, err := editor.StripSeparators(path)
		if err != nil {
			return fmt.Errorf("strip separators: %w", err)
		}
		if result.LinesStripped > 0 {
			fmt.Printf("Stripped %d separator lines from %d messages, saved ~%d tokens\n",
				result.LinesStripped, result.MessagesModified, result.CharsSaved/4)
			slog.Info("Separators stripped", "lines", result.LinesStripped, "messages", result.MessagesModified)
		} else {
			fmt.Println("No decorative separators found.")
		}
	}

	if cleanSnapshots {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for snapshots: %w", err)
		}
		toDelete := make(map[int]bool)
		for i, e := range entries {
			if e.Type == jsonl.TypeFileHistorySnapshot {
				toDelete[i] = true
			}
		}
		if len(toDelete) == 0 {
			fmt.Println("No file-history-snapshot entries found.")
		} else {
			result, err := editor.Delete(path, toDelete)
			if err != nil {
				return fmt.Errorf("remove snapshots: %w", err)
			}
			fmt.Printf("Removed %d snapshot entries, saved %s\n",
				result.EntriesRemoved,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Snapshots removed", "count", result.EntriesRemoved)
		}
	}

	if cleanDedupReads {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for dedup: %w", err)
		}
		dupResult := analyzer.FindDuplicateReads(entries)
		if len(dupResult.Groups) == 0 {
			fmt.Println("No duplicate file reads found.")
		} else {
			result, err := editor.DeduplicateReads(path, dupResult)
			if err != nil {
				return fmt.Errorf("dedup reads: %w", err)
			}
			fmt.Printf("Removed %d stale file reads across %d files, saved %s\n",
				result.StaleReadsRemoved, dupResult.UniqueFiles,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Dedup reads", "stale", result.StaleReadsRemoved, "files", dupResult.UniqueFiles)
		}
	}

	if cleanTruncate {
		result, err := editor.TruncateOutputs(path, cleanOutThreshold, cleanOutKeepLines)
		if err != nil {
			return fmt.Errorf("truncate outputs: %w", err)
		}
		if result.OutputsTruncated > 0 {
			fmt.Printf("Truncated %d outputs, saved ~%d tokens (kept first/last %d lines)\n",
				result.OutputsTruncated, result.TokensSaved, cleanOutKeepLines)
			slog.Info("Outputs truncated", "count", result.OutputsTruncated, "tokens", result.TokensSaved)
		} else {
			fmt.Println("No large outputs to truncate.")
		}
	}

	if cleanFailedRetries {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for retries: %w", err)
		}
		retryResult := analyzer.FindFailedRetries(entries)
		if len(retryResult.Sequences) == 0 {
			fmt.Println("No failed retries found.")
		} else {
			result, err := editor.RemoveFailedRetries(path, retryResult)
			if err != nil {
				return fmt.Errorf("remove retries: %w", err)
			}
			fmt.Printf("Removed %d failed attempts, saved %s\n",
				result.FailedRemoved,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Failed retries removed", "count", result.FailedRemoved)
		}
	}

	if cleanSidechains {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for sidechains: %w", err)
		}
		toDelete := make(map[int]bool)
		for i, e := range entries {
			if e.IsSidechain {
				toDelete[i] = true
			}
		}
		if len(toDelete) == 0 {
			fmt.Println("No sidechain entries found.")
		} else {
			result, err := editor.Delete(path, toDelete)
			if err != nil {
				return fmt.Errorf("remove sidechains: %w", err)
			}
			fmt.Printf("Removed %d sidechain entries, saved %s\n",
				result.EntriesRemoved,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Sidechains removed", "count", result.EntriesRemoved)
		}
	}

	if cleanTangents {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for tangents: %w", err)
		}
		tangentResult := analyzer.FindTangents(entries)
		if len(tangentResult.Groups) == 0 {
			fmt.Println("No cross-repo tangents found.")
		} else {
			toDelete := tangentResult.AllTangentIndices()
			result, err := editor.Delete(path, toDelete)
			if err != nil {
				return fmt.Errorf("remove tangents: %w", err)
			}
			fmt.Printf("Removed %d tangent entries across %d groups referencing %d external repos, saved %s\n",
				result.EntriesRemoved, len(tangentResult.Groups), tangentResult.ExternalDirs,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Tangents removed", "entries", result.EntriesRemoved, "groups", len(tangentResult.Groups))
		}
	}

	return nil
}

func runCleanAuto() error {
	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("discover sessions: %w", err)
	}
	if len(sessions) == 0 {
		if isJSON() {
			return printJSON(map[string]string{"status": "no_sessions"})
		}
		fmt.Println("No sessions found.")
		return nil
	}

	// Most recent session (already sorted by mtime desc)
	target := sessions[0]
	path := target.FullPath
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session file not found: %s", path)
	}

	if !isJSON() {
		printSessionIdentity(path)
	}

	result, err := editor.CleanAll(path)
	if err != nil {
		return fmt.Errorf("clean auto: %w", err)
	}

	if isJSON() {
		out := cleanAllToJSON(path, result)
		out.Mode = "auto"
		return printJSON(out)
	}

	totalOps := result.ProgressRemoved + result.SnapshotsRemoved + result.SidechainsRemoved +
		result.TangentsRemoved + result.FailedRetries + result.StaleReadsRemoved +
		result.ImagesReplaced + result.SeparatorsStripped + result.OutputsTruncated
	if totalOps == 0 {
		fmt.Printf("Session %s (%s): nothing to clean\n", target.SessionID, target.ProjectName)
		return nil
	}

	fmt.Printf("Auto-cleaned session %s (%s): %d entries removed, ~%d tokens saved, %s\n",
		target.SessionID, target.ProjectName,
		result.ProgressRemoved+result.SnapshotsRemoved+result.SidechainsRemoved+
			result.TangentsRemoved+result.FailedRetries+result.StaleReadsRemoved,
		result.TotalTokensSaved,
		formatBytes(result.BytesBefore-result.BytesAfter))
	slog.Info("Clean auto complete", "session", target.SessionID, "project", target.ProjectName, "tokens", result.TotalTokensSaved)
	return nil
}

func runCleanLive(path string) error {
	opts := editor.CleanLiveOpts{
		Aggressive: cleanAggressive,
	}
	result, err := editor.CleanLive(path, opts)
	if err != nil {
		if errors.Is(err, editor.ErrRaceDetected) {
			return fmt.Errorf("aborted: Claude Code wrote to session during cleanup (file restored from backup)")
		}
		if errors.Is(err, editor.ErrSessionNotIdle) {
			return fmt.Errorf("session is actively being written to — wait a few seconds and retry")
		}
		return fmt.Errorf("clean live: %w", err)
	}

	if isJSON() {
		return printJSON(cleanLiveToJSON(path, result))
	}

	fmt.Printf("Live cleaned: %d prog, %d snap",
		result.ProgressRemoved, result.SnapshotsRemoved)
	if cleanAggressive {
		fmt.Printf(", %d img, %d sep, %d trunc",
			result.ImagesReplaced, result.SeparatorsStripped, result.OutputsTruncated)
	}
	fmt.Println()
	fmt.Printf("Total saved: ~%d tokens, %s\n",
		result.TotalTokensSaved, formatBytes(result.BytesBefore-result.BytesAfter))
	slog.Info("Clean live complete", "tokens", result.TotalTokensSaved, "aggressive", opts.Aggressive)
	return nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/1024/1024)
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// printSessionIdentity prints a one-line identity summary before destructive operations.
func printSessionIdentity(path string) {
	fi, err := os.Stat(path)
	if err != nil {
		return
	}

	slug := "—"
	msgs := 0
	if stats, err := jsonl.ScanLight(path); err == nil {
		if stats.Slug != "" {
			slug = stats.Slug
		}
		msgs = stats.LineCount
	}

	base := filepath.Base(path)
	sessionID := strings.TrimSuffix(base, ".jsonl")
	shortID := sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	// Derive project name from parent directory
	project := session.ProjectNameFromDir(filepath.Dir(path))

	size := float64(fi.Size()) / 1024 / 1024
	mod := timeAgo(fi.ModTime())

	fmt.Printf("Cleaning: %s (%s) | %s | %d msgs | %.1f MB | modified %s\n",
		slug, shortID, project, msgs, size, mod)
}

func init() {
	cleanCmd.Flags().BoolVar(&cleanImages, "images", false, "Replace base64 images with placeholders")
	cleanCmd.Flags().BoolVar(&cleanProgress, "progress", false, "Remove all progress messages")
	cleanCmd.Flags().BoolVar(&cleanSeparators, "separators", false, "Strip decorative separator lines")
	cleanCmd.Flags().BoolVar(&cleanSnapshots, "snapshots", false, "Remove all file-history-snapshot entries")
	cleanCmd.Flags().BoolVar(&cleanDedupReads, "dedup-reads", false, "Remove stale duplicate file reads")
	cleanCmd.Flags().BoolVar(&cleanTruncate, "truncate-output", false, "Truncate large Bash outputs")
	cleanCmd.Flags().IntVar(&cleanOutThreshold, "output-threshold", 4096, "Byte threshold for output truncation")
	cleanCmd.Flags().IntVar(&cleanOutKeepLines, "keep-lines", 10, "Lines to keep at start and end")
	cleanCmd.Flags().BoolVar(&cleanFailedRetries, "failed-retries", false, "Remove failed tool attempts that were retried")
	cleanCmd.Flags().BoolVar(&cleanSidechains, "sidechains", false, "Remove all sidechain entries")
	cleanCmd.Flags().BoolVar(&cleanTangents, "tangents", false, "Remove cross-repo tangent sequences")
	cleanCmd.Flags().BoolVar(&cleanAll, "all", false, "Run all cleanup operations")
	cleanCmd.Flags().BoolVar(&cleanLive, "live", false, "Safe cleanup for active sessions (Tier 1-3)")
	cleanCmd.Flags().BoolVar(&cleanAggressive, "aggressive", false, "Include Tier 4-5 operations (use with --live)")
	cleanCmd.Flags().BoolVar(&cleanAuto, "auto", false, "Find and clean the most recent session (no session arg needed)")
	rootCmd.AddCommand(cleanCmd)
}
