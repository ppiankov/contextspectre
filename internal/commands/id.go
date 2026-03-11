package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var idCmd = &cobra.Command{
	Use:   "id <short-id>",
	Short: "Resolve a short session ID to its full UUID and client type",
	Long:  "Takes a short (prefix) session ID and prints the full UUID, client type (cli/desktop), and project.",
	Args:  cobra.ExactArgs(1),
	RunE:  runID,
}

func init() {
	rootCmd.AddCommand(idCmd)
}

// IDOutput is the JSON output for the id command.
type IDOutput struct {
	ShortID    string `json:"short_id"`
	FullID     string `json:"full_id"`
	ClientType string `json:"client_type"`
	Project    string `json:"project"`
	Path       string `json:"path"`
}

func runID(_ *cobra.Command, args []string) error {
	path := resolveSessionPath(args[0])

	// Check the file exists
	if strings.HasSuffix(path, args[0]+".jsonl") && !strings.Contains(path, string(filepath.Separator)+"projects"+string(filepath.Separator)) {
		return fmt.Errorf("session not found: %s", args[0])
	}

	// Extract full session ID from filename
	base := filepath.Base(path)
	fullID := strings.TrimSuffix(base, ".jsonl")

	// Determine client type via ScanLight
	stats, err := jsonl.ScanLight(path)
	if err != nil {
		return fmt.Errorf("scan session: %w", err)
	}

	clientType := "unknown"
	snapshotCount := stats.TypeCounts[jsonl.TypeFileHistorySnapshot]
	if snapshotCount > 0 {
		clientType = "cli"
	} else if stats.StartsWithQueueOp {
		clientType = "desktop"
	} else if stats.LineCount > 100 {
		clientType = "cli"
	}

	// Extract project name from path
	project := extractProjectFromPath(path)

	if isJSON() {
		return printJSON(IDOutput{
			ShortID:    args[0],
			FullID:     fullID,
			ClientType: clientType,
			Project:    project,
			Path:       path,
		})
	}

	fmt.Printf("%-12s %s\n", "Full ID:", fullID)
	fmt.Printf("%-12s %s\n", "Client:", clientType)
	fmt.Printf("%-12s %s\n", "Project:", project)
	fmt.Printf("%-12s %s\n", "Path:", path)
	return nil
}
