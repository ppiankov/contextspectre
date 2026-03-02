package commands

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/spf13/cobra"
)

var (
	cleanImages   bool
	cleanProgress bool
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
	if !cleanImages && !cleanProgress {
		return fmt.Errorf("specify --images and/or --progress")
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
	rootCmd.AddCommand(cleanCmd)
}
