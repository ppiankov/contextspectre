package commands

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	findMove string
)

var findCmd = &cobra.Command{
	Use:   "find <session-id>",
	Short: "Find a session by ID across all projects",
	Long: `Search all Claude Code project directories for a session by full UUID or prefix.

Find where a session lives:
  contextspectre find 88789f29

Move it to the correct project:
  contextspectre find 88789f29 --move /path/to/project

Useful when "claude --resume <id>" fails because the session was created
from a parent directory or a different project path.`,
	Args: cobra.ExactArgs(1),
	RunE: runFind,
}

// FindJSON is the JSON output for the find command.
type FindJSON struct {
	SessionID   string    `json:"session_id"`
	ProjectDir  string    `json:"project_dir"`
	ProjectPath string    `json:"project_path"`
	FullPath    string    `json:"full_path"`
	Moved       *MoveJSON `json:"moved,omitempty"`
}

// MoveJSON is the move result within find output.
type MoveJSON struct {
	FromProject string `json:"from_project"`
	ToProject   string `json:"to_project"`
	NewPath     string `json:"new_path"`
}

func runFind(_ *cobra.Command, args []string) error {
	dir := resolveClaudeDir()

	found, err := session.FindByID(dir, args[0])
	if err != nil {
		return err
	}

	if findMove == "" {
		if isJSON() {
			return printJSON(FindJSON{
				SessionID:   found.SessionID,
				ProjectDir:  found.ProjectDir,
				ProjectPath: found.ProjectPath,
				FullPath:    found.FullPath,
			})
		}

		fmt.Printf("%-12s %s\n", "Session:", found.SessionID)
		fmt.Printf("%-12s %s\n", "Project:", found.ProjectPath)
		fmt.Printf("%-12s %s\n", "Dir:", found.ProjectDir)
		fmt.Printf("%-12s %s\n", "Path:", found.FullPath)
		fmt.Println()
		fmt.Println("To move this session to the correct project:")
		fmt.Printf("  contextspectre find %s --move /path/to/project\n", args[0])
		return nil
	}

	// Move the session
	result, err := session.MoveSession(dir, found, findMove)
	if err != nil {
		return fmt.Errorf("move: %w", err)
	}

	if isJSON() {
		return printJSON(FindJSON{
			SessionID:   found.SessionID,
			ProjectDir:  found.ProjectDir,
			ProjectPath: found.ProjectPath,
			FullPath:    found.FullPath,
			Moved: &MoveJSON{
				FromProject: result.FromProject,
				ToProject:   result.ToProject,
				NewPath:     result.NewPath,
			},
		})
	}

	fmt.Printf("Moved session %s\n", found.SessionID)
	fmt.Printf("  From: %s\n", result.FromProject)
	fmt.Printf("  To:   %s\n", result.ToProject)
	if result.IndexUpdated {
		fmt.Println("  Updated sessions-index.json")
	}
	fmt.Println()
	fmt.Println("You can now resume with:")
	fmt.Printf("  claude --resume %s\n", found.SessionID)
	return nil
}

func init() {
	findCmd.Flags().StringVar(&findMove, "move", "", "Move the session to this project directory path")
	rootCmd.AddCommand(findCmd)
}
