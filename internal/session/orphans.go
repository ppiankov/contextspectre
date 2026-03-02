package session

import (
	"os"
	"path/filepath"
)

// OrphanedProject represents a project directory whose decoded path no longer exists.
type OrphanedProject struct {
	DirName       string // directory name under projects/
	DecodedPath   string // decoded filesystem path
	FullDirPath   string // full path to the project directory
	SessionCount  int    // number of JSONL files
	TotalMessages int    // total messages across all sessions
}

// FindOrphans scans all project directories and returns those whose
// decoded filesystem path no longer exists on disk.
func FindOrphans(claudeDir string) ([]OrphanedProject, error) {
	projectsDir := filepath.Join(claudeDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, err
	}

	var orphans []OrphanedProject
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		dirName := e.Name()
		decoded := DecodePath(dirName)

		// Check if the decoded path exists on disk
		if _, err := os.Stat(decoded); err == nil {
			continue // path exists, not orphaned
		}

		fullDirPath := filepath.Join(projectsDir, dirName)

		// Count sessions
		sessionFiles, _ := filepath.Glob(filepath.Join(fullDirPath, "*.jsonl"))
		if len(sessionFiles) == 0 {
			continue // no sessions, skip
		}

		orphan := OrphanedProject{
			DirName:      dirName,
			DecodedPath:  decoded,
			FullDirPath:  fullDirPath,
			SessionCount: len(sessionFiles),
		}

		orphans = append(orphans, orphan)
	}

	return orphans, nil
}
