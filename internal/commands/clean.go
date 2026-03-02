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
)

var cleanCmd = &cobra.Command{
	Use:   "clean <session-id-or-path>",
	Short: "Clean a session (replace images, remove progress)",
	Long: `Clean a conversation session by replacing base64 images with tiny
placeholders or removing progress messages. Always creates a backup first.`,
	Args: cobra.ExactArgs(1),
	RunE: runClean,
}

func runClean(cmd *cobra.Command, args []string) error {
	if !cleanImages && !cleanProgress && !cleanSeparators && !cleanSnapshots && !cleanDedupReads && !cleanTruncate && !cleanFailedRetries && !cleanSidechains {
		return fmt.Errorf("specify at least one clean operation flag")
	}

	path := resolveSessionPath(args[0])
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", path)
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
	rootCmd.AddCommand(cleanCmd)
}
