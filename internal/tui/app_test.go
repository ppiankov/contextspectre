package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ppiankov/contextspectre/internal/session"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects", "test-project")
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionData := `{"type":"user","uuid":"u1","parentUuid":"","timestamp":"2026-03-01T10:00:00Z","sessionId":"s1","message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","parentUuid":"u1","timestamp":"2026-03-01T10:00:01Z","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":50,"output_tokens":5,"cache_creation_input_tokens":1000,"cache_read_input_tokens":0}}}
`
	if err := os.WriteFile(filepath.Join(projectsDir, "test-session.jsonl"), []byte(sessionData), 0644); err != nil {
		t.Fatal(err)
	}
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
	m, ok := model.(AppModel)
	if !ok {
		t.Fatal("expected AppModel")
	}

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
	m, ok := model.(AppModel)
	if !ok {
		t.Fatal("expected AppModel")
	}

	if m.currentView != viewDetail {
		t.Errorf("expected detail view, got %d", m.currentView)
	}
	if len(m.detail.messages.entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m.detail.messages.entries))
	}
}

func TestApp_BackToSessions(t *testing.T) {
	dir := setupTestDir(t)
	app := NewApp(dir, "test")
	app.currentView = viewMessages

	model, _ := app.Update(backToSessionsMsg{})
	m, ok := model.(AppModel)
	if !ok {
		t.Fatal("expected AppModel")
	}

	if m.currentView != viewSessions {
		t.Errorf("expected sessions view, got %d", m.currentView)
	}
}

func TestApp_CancelDelete(t *testing.T) {
	dir := setupTestDir(t)
	app := NewApp(dir, "test")
	app.currentView = viewConfirm

	model, _ := app.Update(cancelDeleteMsg{})
	m, ok := model.(AppModel)
	if !ok {
		t.Fatal("expected AppModel")
	}

	if m.currentView != viewMessages {
		t.Errorf("expected messages view after cancel, got %d", m.currentView)
	}
}

func TestSessionsModel_Navigation(t *testing.T) {
	// Three sessions from different projects produce:
	// row 0: header "proj1"
	// row 1: s1 (selectable)
	// row 2: header "proj2"
	// row 3: s2 (selectable)
	// row 4: header "proj3"
	// row 5: s3 (selectable)
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1", Modified: time.Now().Add(-1 * time.Hour)},
		{SessionID: "s2", ProjectName: "proj2", Modified: time.Now().Add(-2 * time.Hour)},
		{SessionID: "s3", ProjectName: "proj3", Modified: time.Now().Add(-3 * time.Hour)},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	// Initial cursor on first selectable row (skip header)
	if m.cursor != 1 {
		t.Errorf("expected initial cursor 1, got %d", m.cursor)
	}

	// Move down — skip header to next session
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 3 {
		t.Errorf("expected cursor 3, got %d", m.cursor)
	}

	// Move down again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 5 {
		t.Errorf("expected cursor 5, got %d", m.cursor)
	}

	// Can't go past end
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 5 {
		t.Errorf("expected cursor to stay at 5, got %d", m.cursor)
	}

	// Move up — skip header
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 3 {
		t.Errorf("expected cursor 3, got %d", m.cursor)
	}
}

func TestSessionsModel_NavigationSameProject(t *testing.T) {
	// Two sessions from same project:
	// row 0: header "proj1" (2 sessions)
	// row 1: s1 (selectable)
	// row 2: s2 (selectable)
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1", Modified: time.Now().Add(-1 * time.Hour)},
		{SessionID: "s2", ProjectName: "proj1", Modified: time.Now().Add(-2 * time.Hour)},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	if m.cursor != 1 {
		t.Errorf("expected initial cursor 1, got %d", m.cursor)
	}

	// Move down — stays in same group, no header to skip
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("expected cursor 2, got %d", m.cursor)
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
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	if !strings.Contains(view, "proj1") {
		t.Error("expected project name in view")
	}
}

func TestSessionsModel_ViewTitle(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1"},
		{SessionID: "s2", ProjectName: "proj2"},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	view := m.View()
	if !strings.Contains(view, "contextspectre") {
		t.Error("expected title bar with 'contextspectre'")
	}
	if !strings.Contains(view, "2 sessions") {
		t.Error("expected session count in title")
	}
}

func TestSessionsModel_ViewGroupHeaders(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "myproject", Modified: time.Now()},
		{SessionID: "s2", ProjectName: "myproject", Modified: time.Now().Add(-1 * time.Hour)},
		{SessionID: "s3", ProjectName: "other", Modified: time.Now().Add(-2 * time.Hour)},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	view := m.View()
	if !strings.Contains(view, "myproject (2 sessions)") {
		t.Errorf("expected group header for myproject, got:\n%s", view)
	}
	if !strings.Contains(view, "other (1 sessions)") {
		t.Errorf("expected group header for other, got:\n%s", view)
	}
}

func TestSessionsModel_EmptyView(t *testing.T) {
	m := newSessionsModel(nil, nil, 0)
	m.width = 120
	m.height = 40

	view := m.View()
	if !strings.Contains(view, "No sessions") {
		t.Error("expected 'No sessions' message")
	}
}

func TestSessionsModel_ZeroWidth_NoPanic(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1", MessageCount: 10, FileSizeMB: 1.5},
	}
	m := newSessionsModel(sessions, nil, 0)

	view := m.View()
	if view != "" {
		t.Errorf("expected empty string for zero-width view, got %q", view)
	}
}

func TestSessionsModel_SearchEnterExit(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1"},
		{SessionID: "s2", ProjectName: "proj2"},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	// Enter search mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.searching {
		t.Error("expected searching=true after /")
	}

	// Exit search mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.searching {
		t.Error("expected searching=false after Esc")
	}
}

func TestSessionsModel_SearchFilter(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "logtap", Modified: time.Now()},
		{SessionID: "s2", ProjectName: "contextspectre", Modified: time.Now().Add(-1 * time.Hour)},
		{SessionID: "s3", ProjectName: "logtap", Modified: time.Now().Add(-2 * time.Hour)},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	// Enter search mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Type "log"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})

	if m.searchQuery != "log" {
		t.Errorf("expected searchQuery='log', got %q", m.searchQuery)
	}
	if len(m.filtered) != 2 {
		t.Errorf("expected 2 filtered sessions, got %d", len(m.filtered))
	}

	// Verify view shows match count
	view := m.View()
	if !strings.Contains(view, "2 matches") {
		t.Errorf("expected '2 matches' in view, got:\n%s", view)
	}
}

func TestSessionsModel_SearchNoResults(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1"},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	// Enter search and type nonexistent query
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z', 'z', 'z'}})

	if len(m.filtered) != 0 {
		t.Errorf("expected 0 filtered sessions, got %d", len(m.filtered))
	}
	if m.cursor != -1 {
		t.Errorf("expected cursor -1 for no results, got %d", m.cursor)
	}
}

func TestSessionsModel_SearchBackspace(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "logtap"},
		{SessionID: "s2", ProjectName: "contextspectre"},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l', 'o', 'g'}})
	if len(m.filtered) != 1 {
		t.Errorf("expected 1 match for 'log', got %d", len(m.filtered))
	}

	// Backspace to "lo"
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.searchQuery != "lo" {
		t.Errorf("expected 'lo', got %q", m.searchQuery)
	}
	if len(m.filtered) != 1 {
		t.Errorf("expected 1 match for 'lo', got %d", len(m.filtered))
	}
}

func TestSessionsModel_SearchByBranch(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1", GitBranch: "feat/auth"},
		{SessionID: "s2", ProjectName: "proj2", GitBranch: "main"},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a', 'u', 't', 'h'}})

	if len(m.filtered) != 1 {
		t.Errorf("expected 1 match for branch 'auth', got %d", len(m.filtered))
	}
}

func TestSessionsModel_QuitBlockedDuringSearch(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1"},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	// Enter search mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Verify searching is true (app.go uses this to block quit on 'q')
	if !m.searching {
		t.Error("expected searching=true")
	}

	// Type 'q' as search character
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if m.searchQuery != "q" {
		t.Errorf("expected searchQuery='q', got %q", m.searchQuery)
	}
}

func TestSessionsModel_DisplayRowsGrouping(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "alpha", Modified: time.Now()},
		{SessionID: "s2", ProjectName: "alpha", Modified: time.Now().Add(-1 * time.Hour)},
		{SessionID: "s3", ProjectName: "beta", Modified: time.Now().Add(-2 * time.Hour)},
	}
	m := newSessionsModel(sessions, nil, 0)

	// Expect: header(alpha) + s1 + s2 + header(beta) + s3 = 5 rows
	if len(m.displayRows) != 5 {
		t.Errorf("expected 5 display rows, got %d", len(m.displayRows))
	}

	// First row is header
	if !m.displayRows[0].isHeader {
		t.Error("expected first row to be header")
	}
	if m.displayRows[0].projectName != "alpha" {
		t.Errorf("expected 'alpha', got %q", m.displayRows[0].projectName)
	}
	if m.displayRows[0].sessionCount != 2 {
		t.Errorf("expected 2 sessions in alpha group, got %d", m.displayRows[0].sessionCount)
	}

	// Row 3 is beta header
	if !m.displayRows[3].isHeader {
		t.Error("expected row 3 to be header")
	}
	if m.displayRows[3].projectName != "beta" {
		t.Errorf("expected 'beta', got %q", m.displayRows[3].projectName)
	}
}

func TestSessionsModel_FooterHelp(t *testing.T) {
	sessions := []session.Info{
		{SessionID: "s1", ProjectName: "proj1"},
	}
	m := newSessionsModel(sessions, nil, 0)
	m.width = 120
	m.height = 40

	view := m.View()
	if !strings.Contains(view, "/ search") {
		t.Error("expected '/ search' in footer help")
	}
}

// containsStr is used by both app_test.go and messages_test.go.
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}
