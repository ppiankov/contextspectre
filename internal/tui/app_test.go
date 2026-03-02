package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ppiankov/contextspectre/internal/session"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects", "test-project")
	os.MkdirAll(projectsDir, 0755)

	sessionData := `{"type":"user","uuid":"u1","parentUuid":"","timestamp":"2026-03-01T10:00:00Z","sessionId":"s1","message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-03-01T10:00:01Z","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":50,"output_tokens":5,"cache_creation_input_tokens":1000,"cache_read_input_tokens":0}}}
`
	os.WriteFile(filepath.Join(projectsDir, "test-session.jsonl"), []byte(sessionData), 0644)
	return dir
}

func TestNewApp(t *testing.T) {
	dir := setupTestDir(t)
	app := NewApp(dir, "test")

	if app.currentView != viewSessions {
		t.Errorf("expected sessions view, got %d", app.currentView)
	}
	if len(app.sessions.sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(app.sessions.sessions))
	}
}

func TestApp_WindowResize(t *testing.T) {
	dir := setupTestDir(t)
	app := NewApp(dir, "test")

	model, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m := model.(AppModel)

	if m.width != 120 || m.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", m.width, m.height)
	}
	if m.sessions.width != 120 {
		t.Errorf("expected sessions width 120, got %d", m.sessions.width)
	}
}

func TestApp_OpenSession(t *testing.T) {
	dir := setupTestDir(t)
	app := NewApp(dir, "test")
	app.width = 120
	app.height = 40

	info := app.sessions.sessions[0]
	model, _ := app.Update(openSessionMsg{info: info})
	m := model.(AppModel)

	if m.currentView != viewMessages {
		t.Errorf("expected messages view, got %d", m.currentView)
	}
	if len(m.messages.entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m.messages.entries))
	}
}

func TestApp_BackToSessions(t *testing.T) {
	dir := setupTestDir(t)
	app := NewApp(dir, "test")
	app.currentView = viewMessages

	model, _ := app.Update(backToSessionsMsg{})
	m := model.(AppModel)

	if m.currentView != viewSessions {
		t.Errorf("expected sessions view, got %d", m.currentView)
	}
}

func TestApp_CancelDelete(t *testing.T) {
	dir := setupTestDir(t)
	app := NewApp(dir, "test")
	app.currentView = viewConfirm

	model, _ := app.Update(cancelDeleteMsg{})
	m := model.(AppModel)

	if m.currentView != viewMessages {
		t.Errorf("expected messages view after cancel, got %d", m.currentView)
	}
}

func TestSessionsModel_Navigation(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1"},
		{SessionID: "s2", ProjectName: "proj2"},
		{SessionID: "s3", ProjectName: "proj3"},
	}
	m := newSessionsModel(sessions)
	m.width = 120
	m.height = 40

	// Move down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.cursor)
	}

	// Move down again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("expected cursor 2, got %d", m.cursor)
	}

	// Can't go past end
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", m.cursor)
	}

	// Move up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.cursor)
	}
}

func TestSessionsModel_View(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1", MessageCount: 10, FileSizeMB: 1.5},
	}
	m := newSessionsModel(sessions)
	m.width = 120
	m.height = 40

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	if !containsStr(view, "proj1") {
		t.Error("expected project name in view")
	}
}

func TestSessionsModel_EmptyView(t *testing.T) {
	m := newSessionsModel(nil)
	m.width = 120
	m.height = 40

	view := m.View()
	if !containsStr(view, "No sessions") {
		t.Error("expected 'No sessions' message")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findStr(s, substr))
}

func findStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
