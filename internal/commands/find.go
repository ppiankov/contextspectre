package commands

import (
	"fmt"
	"path/filepath"

	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	findMove string
	findCopy string
)

var findCmd = &cobra.Command{
	Use:   "find <session-id>",
	Short: "Find a session by ID across all projects",
	Long: `Search all Claude Code project directories for a session by full UUID or prefix.

Find where a session lives:
  contextspectre find 88789f29

Move it to the correct project:
  contextspectre find 88789f29 --move /path/to/project

Copy it to another project (original stays):
  contextspectre find 88789f29 --copy /path/to/project

Useful when "claude --resume <id>" fails because the session was created
from a parent directory or a different project path. Use --copy for
multi-repo sessions that belong to the parent but should be visible
from child project directories.`,
	Args: cobra.ExactArgs(1),
	RunE: runFind,
}

// FindJSON is the JSON output for the find command.
type FindJSON struct {
	SessionID   string        `json:"session_id"`
	ProjectDir  string        `json:"project_dir"`
	ProjectPath string        `json:"project_path"`
	FullPath    string        `json:"full_path"`
	Moved       *TransferJSON `json:"moved,omitempty"`
	Copied      *TransferJSON `json:"copied,omitempty"`
}

// TransferJSON is the move/copy result within find output.
type TransferJSON struct {
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

	if findMove != "" && findCopy != "" {
		return fmt.Errorf("use --move or --copy, not both")
	}

	if findMove == "" && findCopy == "" {
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
		fmt.Println("To copy it (original stays):")
		fmt.Printf("  contextspectre find %s --copy /path/to/project\n", args[0])
		return nil
	}

	targetFlag := findMove
	action := "move"
	if findCopy != "" {
		targetFlag = findCopy
		action = "copy"
	}

	targetPath, err := filepath.Abs(targetFlag)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	var result *session.MoveResult
	if action == "move" {
		result, err = session.MoveSession(dir, found, targetPath)
	} else {
		result, err = session.CopySession(dir, found, targetPath)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}

	if isJSON() {
		out := FindJSON{
			SessionID:   found.SessionID,
			ProjectDir:  found.ProjectDir,
			ProjectPath: found.ProjectPath,
			FullPath:    found.FullPath,
		}
		tj := &TransferJSON{
			FromProject: result.FromProject,
			ToProject:   result.ToProject,
			NewPath:     result.NewPath,
		}
		if action == "move" {
			out.Moved = tj
		} else {
			out.Copied = tj
		}
		return printJSON(out)
	}

	verb := "Moved"
	if action == "copy" {
		verb = "Copied"
	}
	fmt.Printf("%s session %s\n", verb, found.SessionID)
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
	findCmd.Flags().StringVar(&findCopy, "copy", "", "Copy the session to this project directory (original stays)")
	rootCmd.AddCommand(findCmd)
}
