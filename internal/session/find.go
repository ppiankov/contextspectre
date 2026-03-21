package session

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FindResult holds the result of finding a session by ID.
type FindResult struct {
	SessionID   string // full UUID
	ProjectDir  string // encoded project directory name
	ProjectPath string // decoded filesystem path
	FullPath    string // full path to the JSONL file
}

// FindByID searches all project directories for a session matching the given
// full UUID or prefix. Returns an error if not found or ambiguous.
func FindByID(claudeDir, id string) (*FindResult, error) {
	projectsDir := filepath.Join(claudeDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("read projects dir: %w", err)
	}

	isFullUUID := len(id) == 36 && strings.Count(id, "-") == 4

	var matches []FindResult

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		projDir := filepath.Join(projectsDir, e.Name())

		if isFullUUID {
			candidate := filepath.Join(projDir, id+".jsonl")
			if _, err := os.Stat(candidate); err == nil {
				matches = append(matches, FindResult{
					SessionID:   id,
					ProjectDir:  e.Name(),
					ProjectPath: DecodePath(e.Name()),
					FullPath:    candidate,
				})
			}
			continue
		}

		// Prefix match
		files, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			name := f.Name()
			if strings.HasPrefix(name, id) && strings.HasSuffix(name, ".jsonl") && !strings.Contains(name, ".bak") {
				fullID := strings.TrimSuffix(name, ".jsonl")
				matches = append(matches, FindResult{
					SessionID:   fullID,
					ProjectDir:  e.Name(),
					ProjectPath: DecodePath(e.Name()),
					FullPath:    filepath.Join(projDir, name),
				})
			}
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if len(matches) > 1 {
		var locs []string
		for _, m := range matches {
			locs = append(locs, fmt.Sprintf("  %s in %s", m.SessionID, m.ProjectPath))
		}
		return nil, fmt.Errorf("ambiguous ID %q matches %d sessions:\n%s", id, len(matches), strings.Join(locs, "\n"))
	}

	return &matches[0], nil
}

// MoveResult holds the result of moving a session.
type MoveResult struct {
	SessionID    string
	FromProject  string // decoded source path
	ToProject    string // decoded target path
	NewPath      string // new JSONL file path
	IndexUpdated bool
}

// MoveSession moves a single session JSONL file from its current project
// directory to the project directory for targetPath. Creates the target
// directory if it doesn't exist. Updates sessions-index.json in both dirs.
func MoveSession(claudeDir string, found *FindResult, targetPath string) (*MoveResult, error) {
	targetDirName := EncodePath(targetPath)
	projectsDir := filepath.Join(claudeDir, "projects")
	targetDir := filepath.Join(projectsDir, targetDirName)

	// Don't move to same location
	if found.ProjectDir == targetDirName {
		return nil, fmt.Errorf("session is already in project %s", targetPath)
	}

	// Create target directory if needed
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("create target dir: %w", err)
	}

	filename := filepath.Base(found.FullPath)
	newPath := filepath.Join(targetDir, filename)

	// Check for conflict
	if _, err := os.Stat(newPath); err == nil {
		return nil, fmt.Errorf("session already exists in target: %s", newPath)
	}

	// Move the file
	if err := os.Rename(found.FullPath, newPath); err != nil {
		return nil, fmt.Errorf("move session file: %w", err)
	}

	result := &MoveResult{
		SessionID:   found.SessionID,
		FromProject: found.ProjectPath,
		ToProject:   targetPath,
		NewPath:     newPath,
	}

	// Remove from source sessions-index.json
	srcIndexPath := filepath.Join(projectsDir, found.ProjectDir, "sessions-index.json")
	removeFromIndex(srcIndexPath, found.SessionID)

	// Add to target sessions-index.json (update fullPath and projectPath)
	dstIndexPath := filepath.Join(targetDir, "sessions-index.json")
	if addToIndex(dstIndexPath, found.SessionID, newPath, targetPath) == nil {
		result.IndexUpdated = true
	}

	return result, nil
}

// CopySession copies a session JSONL file to the project directory for
// targetPath. The original file is preserved. Creates the target directory
// if it doesn't exist. Updates sessions-index.json in the target.
func CopySession(claudeDir string, found *FindResult, targetPath string) (*MoveResult, error) {
	targetDirName := EncodePath(targetPath)
	projectsDir := filepath.Join(claudeDir, "projects")
	targetDir := filepath.Join(projectsDir, targetDirName)

	if found.ProjectDir == targetDirName {
		return nil, fmt.Errorf("session is already in project %s", targetPath)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("create target dir: %w", err)
	}

	filename := filepath.Base(found.FullPath)
	newPath := filepath.Join(targetDir, filename)

	if _, err := os.Stat(newPath); err == nil {
		return nil, fmt.Errorf("session already exists in target: %s", newPath)
	}

	if err := copyFile(found.FullPath, newPath); err != nil {
		return nil, fmt.Errorf("copy session file: %w", err)
	}

	result := &MoveResult{
		SessionID:   found.SessionID,
		FromProject: found.ProjectPath,
		ToProject:   targetPath,
		NewPath:     newPath,
	}

	dstIndexPath := filepath.Join(targetDir, "sessions-index.json")
	if addToIndex(dstIndexPath, found.SessionID, newPath, targetPath) == nil {
		result.IndexUpdated = true
	}

	return result, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

// removeFromIndex removes an entry from sessions-index.json by session ID.
func removeFromIndex(indexPath, sessionID string) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return
	}
	var idx sessionsIndex
	if json.Unmarshal(data, &idx) != nil {
		return
	}

	filtered := idx.Entries[:0]
	for _, e := range idx.Entries {
		if e.SessionID != sessionID {
			filtered = append(filtered, e)
		}
	}
	idx.Entries = filtered

	newData, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return
	}
	tmpPath := indexPath + ".tmp"
	if os.WriteFile(tmpPath, newData, 0644) == nil {
		_ = os.Rename(tmpPath, indexPath)
	}
}

// addToIndex adds or updates an entry in sessions-index.json.
func addToIndex(indexPath, sessionID, fullPath, projectPath string) error {
	var idx sessionsIndex

	data, err := os.ReadFile(indexPath)
	if err == nil {
		_ = json.Unmarshal(data, &idx)
	}
	if idx.Version == 0 {
		idx.Version = 1
	}

	// Check if already present
	for i, e := range idx.Entries {
		if e.SessionID == sessionID {
			idx.Entries[i].FullPath = fullPath
			idx.Entries[i].ProjectPath = projectPath
			return writeIndex(indexPath, &idx)
		}
	}

	// Add new entry
	idx.Entries = append(idx.Entries, indexEntry{
		SessionID:   sessionID,
		FullPath:    fullPath,
		ProjectPath: projectPath,
	})
	return writeIndex(indexPath, &idx)
}

func writeIndex(indexPath string, idx *sessionsIndex) error {
	newData, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := indexPath + ".tmp"
	if err := os.WriteFile(tmpPath, newData, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, indexPath)
}

// FindSessionsForCWD searches all projects for sessions whose project path
// contains the given directory name. Returns matches from projects OTHER than
// the exact CWD-encoded directory, to find "misplaced" sessions.
func FindSessionsForCWD(claudeDir, cwd string) []FindResult {
	projectsDir := filepath.Join(claudeDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}

	exactDir := EncodePath(cwd)
	cwdBase := filepath.Base(cwd)

	var results []FindResult

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirName := e.Name()
		if dirName == exactDir {
			continue // skip the exact match (already checked by caller)
		}

		decoded := DecodePath(dirName)

		// Match if the CWD is a subdirectory of this project's decoded path,
		// or if this project's path is a parent of CWD
		if !strings.HasPrefix(cwd, decoded) && !strings.Contains(dirName, cwdBase) {
			continue
		}

		projDir := filepath.Join(projectsDir, dirName)
		files, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			name := f.Name()
			if strings.HasSuffix(name, ".jsonl") && !strings.Contains(name, ".bak") {
				fullID := strings.TrimSuffix(name, ".jsonl")
				results = append(results, FindResult{
					SessionID:   fullID,
					ProjectDir:  dirName,
					ProjectPath: decoded,
					FullPath:    filepath.Join(projDir, name),
				})
			}
		}
	}

	return results
}
