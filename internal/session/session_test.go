package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInfo_IsActive(t *testing.T) {
	active := Info{Modified: time.Now()}
	if !active.IsActive() {
		t.Error("expected recent session to be active")
	}

	old := Info{Modified: time.Now().Add(-5 * time.Minute)}
	if old.IsActive() {
		t.Error("expected old session to not be active")
	}
}

func TestProjectNameFromDir(t *testing.T) {
	tests := []struct {
		dir  string
		want string
	}{
		{"/home/user/.claude/projects/-Users-user-dev-myproject", "myproject"},
		{"/home/user/.claude/projects/-Users-dev-myproject", "myproject"},
		{"/home/user/.claude/projects/simple", "simple"},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			got := ProjectNameFromDir(tt.dir)
			if got != tt.want {
				t.Errorf("ProjectNameFromDir(%q) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverer_ListProjects(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")
	mustMkdirAll(t, filepath.Join(projectsDir, "project-a"))
	mustMkdirAll(t, filepath.Join(projectsDir, "project-b"))
	// Create a file (should be ignored)
	mustWriteFile(t, filepath.Join(projectsDir, "not-a-dir.txt"), []byte("x"))

	d := &Discoverer{ClaudeDir: dir}
	projects, err := d.ListProjects()
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestDiscoverer_ListSessions_FromGlob(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "projects", "test-project")
	mustMkdirAll(t, projectDir)

	// Create a minimal JSONL file
	sessionData := `{"type":"user","uuid":"u1","timestamp":"2026-03-01T10:00:00Z","sessionId":"s1","message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-03-01T10:00:01Z","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":50,"output_tokens":5,"cache_creation_input_tokens":1000,"cache_read_input_tokens":0}}}
`
	mustWriteFile(t, filepath.Join(projectDir, "test-session.jsonl"), []byte(sessionData))

	d := &Discoverer{ClaudeDir: dir}
	sessions, err := d.ListSessions(projectDir)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "test-session" {
		t.Errorf("expected session ID 'test-session', got %q", sessions[0].SessionID)
	}
	if sessions[0].ContextStats == nil {
		t.Fatal("expected context stats")
	}
	if sessions[0].ContextStats.ContextTokens != 1050 {
		t.Errorf("expected 1050 context tokens, got %d", sessions[0].ContextStats.ContextTokens)
	}
}

func TestDiscoverer_ListSessions_FromIndex(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "projects", "test-project")
	mustMkdirAll(t, projectDir)

	// Create session file
	sessionPath := filepath.Join(projectDir, "abc-123.jsonl")
	sessionData := `{"type":"user","uuid":"u1","timestamp":"2026-03-01T10:00:00Z","message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-03-01T10:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":50,"output_tokens":5,"cache_creation_input_tokens":2000,"cache_read_input_tokens":0}}}
`
	mustWriteFile(t, sessionPath, []byte(sessionData))

	// Create sessions-index.json
	idx := sessionsIndex{
		Version: 1,
		Entries: []indexEntry{{
			SessionID:    "abc-123",
			FullPath:     sessionPath,
			FirstPrompt:  "hello",
			MessageCount: 2,
			Created:      "2026-03-01T10:00:00Z",
			Modified:     "2026-03-01T10:00:01Z",
			GitBranch:    "main",
			ProjectPath:  "/dev/test",
		}},
	}
	idxData, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(projectDir, "sessions-index.json"), idxData)

	d := &Discoverer{ClaudeDir: dir}
	sessions, err := d.ListSessions(projectDir)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].FirstPrompt != "hello" {
		t.Errorf("expected first prompt 'hello', got %q", sessions[0].FirstPrompt)
	}
	if sessions[0].GitBranch != "main" {
		t.Errorf("expected branch 'main', got %q", sessions[0].GitBranch)
	}
}

func TestDiscoverer_ListAllSessions(t *testing.T) {
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects")

	// Two projects with one session each
	for _, name := range []string{"proj-a", "proj-b"} {
		pdir := filepath.Join(projectsDir, name)
		mustMkdirAll(t, pdir)
		data := `{"type":"user","uuid":"u1","timestamp":"2026-03-01T10:00:00Z","message":{"role":"user","content":"hello"}}
`
		mustWriteFile(t, filepath.Join(pdir, "sess.jsonl"), []byte(data))
	}

	d := &Discoverer{ClaudeDir: dir}
	all, err := d.ListAllSessions()
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(all))
	}
}

func TestDiscoverer_ListProjects_NoDir(t *testing.T) {
	d := &Discoverer{ClaudeDir: "/nonexistent"}
	_, err := d.ListProjects()
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestDefaultClaudeDir(t *testing.T) {
	dir := DefaultClaudeDir()
	if dir == "" {
		t.Skip("no home dir")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
}
