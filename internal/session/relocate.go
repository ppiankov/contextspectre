package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RelocateResult holds the result of a session relocation operation.
type RelocateResult struct {
	OldDirName    string
	NewDirName    string
	SessionsFound int
	IndexUpdated  bool
	CWDUpdated    int // entries with CWD rewritten
	DryRun        bool
}

// RelocatePlan describes what a relocation would do without executing it.
type RelocatePlan struct {
	OldPath      string
	NewPath      string
	OldDirName   string
	NewDirName   string
	SessionCount int
	IndexEntries int
	OldDirExists bool
	NewDirExists bool
}

// PlanRelocate creates a dry-run plan for relocating sessions.
func PlanRelocate(claudeDir, fromPath, toPath string) (*RelocatePlan, error) {
	oldDirName := EncodePath(fromPath)
	newDirName := EncodePath(toPath)

	projectsDir := filepath.Join(claudeDir, "projects")
	oldDir := filepath.Join(projectsDir, oldDirName)
	newDir := filepath.Join(projectsDir, newDirName)

	plan := &RelocatePlan{
		OldPath:    fromPath,
		NewPath:    toPath,
		OldDirName: oldDirName,
		NewDirName: newDirName,
	}

	// Check old directory exists
	if _, err := os.Stat(oldDir); err == nil {
		plan.OldDirExists = true
	}

	// Check new directory exists (conflict)
	if _, err := os.Stat(newDir); err == nil {
		plan.NewDirExists = true
	}

	// Count sessions
	sessionFiles, _ := filepath.Glob(filepath.Join(oldDir, "*.jsonl"))
	plan.SessionCount = len(sessionFiles)

	// Count index entries
	indexPath := filepath.Join(oldDir, "sessions-index.json")
	if data, err := os.ReadFile(indexPath); err == nil {
		var idx sessionsIndex
		if json.Unmarshal(data, &idx) == nil {
			plan.IndexEntries = len(idx.Entries)
		}
	}

	return plan, nil
}

// Relocate moves sessions from one project path to another.
// This renames the project directory and updates sessions-index.json.
// If updateCWD is true, also rewrites CWD fields in JSONL entries.
func Relocate(claudeDir, fromPath, toPath string, updateCWD bool) (*RelocateResult, error) {
	oldDirName := EncodePath(fromPath)
	newDirName := EncodePath(toPath)

	projectsDir := filepath.Join(claudeDir, "projects")
	oldDir := filepath.Join(projectsDir, oldDirName)
	newDir := filepath.Join(projectsDir, newDirName)

	result := &RelocateResult{
		OldDirName: oldDirName,
		NewDirName: newDirName,
	}

	// Verify old directory exists
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("source directory not found: %s", oldDir)
	}

	// Check for conflicts
	if _, err := os.Stat(newDir); err == nil {
		return nil, fmt.Errorf("target directory already exists: %s (use --merge for merging, not yet implemented)", newDir)
	}

	// Count sessions
	sessionFiles, _ := filepath.Glob(filepath.Join(oldDir, "*.jsonl"))
	result.SessionsFound = len(sessionFiles)

	// Step 1: Rename directory
	if err := os.Rename(oldDir, newDir); err != nil {
		return nil, fmt.Errorf("rename directory: %w", err)
	}

	// Step 2: Update sessions-index.json
	indexPath := filepath.Join(newDir, "sessions-index.json")
	if err := updateSessionsIndex(indexPath, fromPath, toPath, oldDirName, newDirName); err != nil {
		// Non-fatal: index might not exist
		if !os.IsNotExist(err) {
			// Attempt to roll back
			_ = os.Rename(newDir, oldDir)
			return nil, fmt.Errorf("update sessions-index: %w", err)
		}
	} else {
		result.IndexUpdated = true
	}

	// Step 3: Optionally update CWD in JSONL files
	if updateCWD {
		for _, oldPath := range sessionFiles {
			newPath := filepath.Join(newDir, filepath.Base(oldPath))
			count, err := rewriteCWD(newPath, fromPath, toPath)
			if err != nil {
				continue // non-fatal, log but continue
			}
			result.CWDUpdated += count
		}
	}

	return result, nil
}

// updateSessionsIndex rewrites the sessions-index.json with new paths.
func updateSessionsIndex(indexPath, fromPath, toPath, oldDirName, newDirName string) error {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	var idx sessionsIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return fmt.Errorf("parse index: %w", err)
	}

	// Update originalPath
	if idx.OriginalPath == fromPath {
		idx.OriginalPath = toPath
	}

	// Update each entry
	for i := range idx.Entries {
		if idx.Entries[i].ProjectPath == fromPath {
			idx.Entries[i].ProjectPath = toPath
		}
		// Update fullPath: replace old directory name with new
		idx.Entries[i].FullPath = strings.Replace(
			idx.Entries[i].FullPath, oldDirName, newDirName, 1)
	}

	// Atomic write: temp file + rename
	newData, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	tmpPath := indexPath + ".tmp"
	if err := os.WriteFile(tmpPath, newData, 0644); err != nil {
		return fmt.Errorf("write temp index: %w", err)
	}
	return os.Rename(tmpPath, indexPath)
}

// rewriteCWD updates CWD fields in a JSONL file, replacing fromPath with toPath.
// Returns the number of entries modified.
func rewriteCWD(path, fromPath, toPath string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	var newLines [][]byte
	modified := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)

		// Quick check: does this line contain the old path?
		if !strings.Contains(string(lineCopy), fromPath) {
			newLines = append(newLines, lineCopy)
			continue
		}

		// Parse, update CWD, re-serialize
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(lineCopy, &raw); err != nil {
			newLines = append(newLines, lineCopy)
			continue
		}

		var cwd string
		if cwdRaw, ok := raw["cwd"]; ok {
			if json.Unmarshal(cwdRaw, &cwd) == nil && cwd == fromPath {
				newCWD, _ := json.Marshal(toPath)
				raw["cwd"] = newCWD
				updated, err := json.Marshal(raw)
				if err != nil {
					newLines = append(newLines, lineCopy)
					continue
				}
				newLines = append(newLines, updated)
				modified++
				continue
			}
		}

		newLines = append(newLines, lineCopy)
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	if modified == 0 {
		return 0, nil
	}

	// Write back atomically
	tmpPath := path + ".tmp"
	tmpF, err := os.Create(tmpPath)
	if err != nil {
		return 0, err
	}

	w := bufio.NewWriter(tmpF)
	for i, line := range newLines {
		if _, err := w.Write(line); err != nil {
			tmpF.Close()
			os.Remove(tmpPath)
			return 0, err
		}
		if i < len(newLines)-1 {
			w.WriteByte('\n')
		}
	}
	if err := w.Flush(); err != nil {
		tmpF.Close()
		os.Remove(tmpPath)
		return 0, err
	}
	tmpF.Close()

	return modified, os.Rename(tmpPath, path)
}
