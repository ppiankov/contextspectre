package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupRelocateFixture(t *testing.T) (claudeDir, oldPath, newPath string) {
	t.Helper()

	claudeDir = t.TempDir()
	oldPath = "/Users/testuser/dev/oldproject"
	newPath = "/Users/testuser/dev/repos/newproject"

	oldDirName := EncodePath(oldPath)
	projectsDir := filepath.Join(claudeDir, "projects")
	oldDir := filepath.Join(projectsDir, oldDirName)

	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a JSONL session file
	session1 := filepath.Join(oldDir, "abc123.jsonl")
	lines := []string{
		`{"type":"user","uuid":"u1","cwd":"/Users/testuser/dev/oldproject","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":"hi"}}`,
	}
	data := []byte(lines[0] + "\n" + lines[1])
	if err := os.WriteFile(session1, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create sessions-index.json
	idx := sessionsIndex{
		Version:      1,
		OriginalPath: oldPath,
		Entries: []indexEntry{
			{
				SessionID:   "abc123",
				FullPath:    filepath.Join(oldDir, "abc123.jsonl"),
				ProjectPath: oldPath,
			},
		},
	}
	idxData, _ := json.MarshalIndent(idx, "", "  ")
	if err := os.WriteFile(filepath.Join(oldDir, "sessions-index.json"), idxData, 0644); err != nil {
		t.Fatal(err)
	}

	return claudeDir, oldPath, newPath
}

func TestPlanRelocate(t *testing.T) {
	claudeDir, oldPath, newPath := setupRelocateFixture(t)

	plan, err := PlanRelocate(claudeDir, oldPath, newPath)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if !plan.OldDirExists {
		t.Error("expected old directory to exist")
	}
	if plan.NewDirExists {
		t.Error("expected new directory to not exist yet")
	}
	if plan.SessionCount != 1 {
		t.Errorf("expected 1 session, got %d", plan.SessionCount)
	}
	if plan.IndexEntries != 1 {
		t.Errorf("expected 1 index entry, got %d", plan.IndexEntries)
	}
	if plan.OldDirName != EncodePath(oldPath) {
		t.Errorf("wrong old dir name: %s", plan.OldDirName)
	}
	if plan.NewDirName != EncodePath(newPath) {
		t.Errorf("wrong new dir name: %s", plan.NewDirName)
	}
}

func TestRelocate_Basic(t *testing.T) {
	claudeDir, oldPath, newPath := setupRelocateFixture(t)

	result, err := Relocate(claudeDir, oldPath, newPath, false)
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}

	if result.SessionsFound != 1 {
		t.Errorf("expected 1 session, got %d", result.SessionsFound)
	}
	if !result.IndexUpdated {
		t.Error("expected index to be updated")
	}

	// Verify old directory is gone
	oldDir := filepath.Join(claudeDir, "projects", EncodePath(oldPath))
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("expected old directory to be removed")
	}

	// Verify new directory exists
	newDir := filepath.Join(claudeDir, "projects", EncodePath(newPath))
	if _, err := os.Stat(newDir); err != nil {
		t.Errorf("expected new directory to exist: %v", err)
	}

	// Verify sessions-index.json was updated
	idxPath := filepath.Join(newDir, "sessions-index.json")
	data, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var idx sessionsIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("parse index: %v", err)
	}
	if idx.OriginalPath != newPath {
		t.Errorf("expected originalPath=%q, got %q", newPath, idx.OriginalPath)
	}
	if idx.Entries[0].ProjectPath != newPath {
		t.Errorf("expected entry projectPath=%q, got %q", newPath, idx.Entries[0].ProjectPath)
	}
}

func TestRelocate_WithCWDUpdate(t *testing.T) {
	claudeDir, oldPath, newPath := setupRelocateFixture(t)

	result, err := Relocate(claudeDir, oldPath, newPath, true)
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}

	if result.CWDUpdated == 0 {
		t.Error("expected CWD entries to be updated")
	}

	// Verify JSONL CWD was updated
	newDir := filepath.Join(claudeDir, "projects", EncodePath(newPath))
	sessionPath := filepath.Join(newDir, "abc123.jsonl")
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}

	// Check that the old path no longer appears
	if contains(string(data), oldPath) {
		t.Error("expected old path to be replaced in JSONL")
	}
}

func TestRelocate_ConflictErrors(t *testing.T) {
	claudeDir, oldPath, newPath := setupRelocateFixture(t)

	// Create new directory to cause conflict
	newDir := filepath.Join(claudeDir, "projects", EncodePath(newPath))
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Relocate(claudeDir, oldPath, newPath, false)
	if err == nil {
		t.Error("expected error for conflicting target directory")
	}
}

func TestRelocate_MissingSource(t *testing.T) {
	claudeDir := t.TempDir()
	projectsDir := filepath.Join(claudeDir, "projects")
	os.MkdirAll(projectsDir, 0755)

	_, err := Relocate(claudeDir, "/nonexistent/path", "/new/path", false)
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
