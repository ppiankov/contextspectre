package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupFindFixture(t *testing.T) string {
	t.Helper()
	claudeDir := t.TempDir()
	projectsDir := filepath.Join(claudeDir, "projects")

	// Create two project dirs with sessions
	proj1 := filepath.Join(projectsDir, "-Users-test-dev-project1")
	proj2 := filepath.Join(projectsDir, "-Users-test-dev-project2")
	if err := os.MkdirAll(proj1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(proj2, 0755); err != nil {
		t.Fatal(err)
	}

	// Session in project1
	if err := os.WriteFile(
		filepath.Join(proj1, "aaaa1111-bbbb-cccc-dddd-eeee11111111.jsonl"),
		[]byte(`{"type":"human"}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Session in project2
	if err := os.WriteFile(
		filepath.Join(proj2, "aaaa2222-bbbb-cccc-dddd-eeee22222222.jsonl"),
		[]byte(`{"type":"human"}`), 0644); err != nil {
		t.Fatal(err)
	}

	return claudeDir
}

func TestFindByID_ExactMatch(t *testing.T) {
	claudeDir := setupFindFixture(t)

	result, err := FindByID(claudeDir, "aaaa1111-bbbb-cccc-dddd-eeee11111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != "aaaa1111-bbbb-cccc-dddd-eeee11111111" {
		t.Errorf("got session ID %s, want aaaa1111-bbbb-cccc-dddd-eeee11111111", result.SessionID)
	}
	if !strings.Contains(result.ProjectDir, "project1") {
		t.Errorf("got project dir %s, want project1", result.ProjectDir)
	}
}

func TestFindByID_PrefixMatch(t *testing.T) {
	claudeDir := setupFindFixture(t)

	result, err := FindByID(claudeDir, "aaaa1111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != "aaaa1111-bbbb-cccc-dddd-eeee11111111" {
		t.Errorf("got session ID %s, want full UUID", result.SessionID)
	}
}

func TestFindByID_NotFound(t *testing.T) {
	claudeDir := setupFindFixture(t)

	_, err := FindByID(claudeDir, "zzzz0000")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestFindByID_Ambiguous(t *testing.T) {
	claudeDir := setupFindFixture(t)

	// Prefix "aaaa" matches both sessions
	_, err := FindByID(claudeDir, "aaaa")
	if err == nil {
		t.Fatal("expected error for ambiguous prefix")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected 'ambiguous' error, got: %v", err)
	}
}

func TestMoveSession_Basic(t *testing.T) {
	claudeDir := setupFindFixture(t)

	found, err := FindByID(claudeDir, "aaaa1111-bbbb-cccc-dddd-eeee11111111")
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	result, err := MoveSession(claudeDir, found, "/Users/test/dev/project3")
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	if result.FromProject != "/Users/test/dev/project1" {
		t.Errorf("from = %s, want /Users/test/dev/project1", result.FromProject)
	}
	if result.ToProject != "/Users/test/dev/project3" {
		t.Errorf("to = %s, want /Users/test/dev/project3", result.ToProject)
	}

	// Verify file moved
	if _, err := os.Stat(found.FullPath); !os.IsNotExist(err) {
		t.Error("source file should no longer exist")
	}
	if _, err := os.Stat(result.NewPath); err != nil {
		t.Errorf("target file should exist: %v", err)
	}
}

func TestMoveSession_SameProject(t *testing.T) {
	claudeDir := setupFindFixture(t)

	found, err := FindByID(claudeDir, "aaaa1111-bbbb-cccc-dddd-eeee11111111")
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	_, err = MoveSession(claudeDir, found, "/Users/test/dev/project1")
	if err == nil {
		t.Fatal("expected error when moving to same project")
	}
	if !strings.Contains(err.Error(), "already in project") {
		t.Errorf("expected 'already in project' error, got: %v", err)
	}
}

func TestMoveSession_Conflict(t *testing.T) {
	claudeDir := setupFindFixture(t)

	// Find first, before creating the conflict
	found, err := FindByID(claudeDir, "aaaa1111-bbbb-cccc-dddd-eeee11111111")
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	// Now put a file with the same name in project2 to create the conflict
	proj2 := filepath.Join(claudeDir, "projects", "-Users-test-dev-project2")
	conflictPath := filepath.Join(proj2, "aaaa1111-bbbb-cccc-dddd-eeee11111111.jsonl")
	if err := os.WriteFile(conflictPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = MoveSession(claudeDir, found, "/Users/test/dev/project2")
	if err == nil {
		t.Fatal("expected error for conflicting session")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

func TestCopySession_Basic(t *testing.T) {
	claudeDir := setupFindFixture(t)

	found, err := FindByID(claudeDir, "aaaa1111-bbbb-cccc-dddd-eeee11111111")
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	result, err := CopySession(claudeDir, found, "/Users/test/dev/project3")
	if err != nil {
		t.Fatalf("copy: %v", err)
	}

	if result.ToProject != "/Users/test/dev/project3" {
		t.Errorf("to = %s, want /Users/test/dev/project3", result.ToProject)
	}

	// Original must still exist
	if _, err := os.Stat(found.FullPath); err != nil {
		t.Errorf("original file should still exist: %v", err)
	}
	// Copy must exist
	if _, err := os.Stat(result.NewPath); err != nil {
		t.Errorf("copied file should exist: %v", err)
	}

	// Contents must match
	orig, _ := os.ReadFile(found.FullPath)
	copied, _ := os.ReadFile(result.NewPath)
	if string(orig) != string(copied) {
		t.Error("copied file contents differ from original")
	}
}

func TestFindSessionsForCWD(t *testing.T) {
	claudeDir := setupFindFixture(t)

	// Searching for sessions matching "project1" from a subdir
	// The fixture has -Users-test-dev-project1 which decodes to /Users/test/dev/project1
	results := FindSessionsForCWD(claudeDir, "/Users/test/dev/project1/subdir")
	// Should find sessions in parent project (project1 is a parent of project1/subdir)
	if len(results) == 0 {
		t.Fatal("expected to find sessions in parent project")
	}

	found := false
	for _, r := range results {
		if r.SessionID == "aaaa1111-bbbb-cccc-dddd-eeee11111111" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find aaaa1111 session")
	}
}
