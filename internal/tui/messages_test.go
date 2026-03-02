package tui

import (
	"encoding/json"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
)

func testMessagesModel() messagesModel {
	entries := []jsonl.Entry{
		{
			Type: jsonl.TypeUser, UUID: "u1",
			Timestamp: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			Message:   &jsonl.Message{Content: json.RawMessage(`"hello world"`)},
		},
		{
			Type: jsonl.TypeAssistant, UUID: "a1", ParentUUID: "u1",
			Timestamp: time.Date(2026, 3, 1, 10, 0, 1, 0, time.UTC),
			Message: &jsonl.Message{
				Content: json.RawMessage(`[{"type":"text","text":"Hi there!"}]`),
				Usage:   &jsonl.Usage{InputTokens: 100, CacheCreationInputTokens: 5000, OutputTokens: 10},
			},
		},
		{
			Type: jsonl.TypeProgress, UUID: "p1", ParentUUID: "a1",
			Timestamp: time.Date(2026, 3, 1, 10, 0, 2, 0, time.UTC),
			ToolUseID: "toolu_1",
		},
		{
			Type: jsonl.TypeUser, UUID: "u2", ParentUUID: "a1",
			Timestamp: time.Date(2026, 3, 1, 10, 1, 0, 0, time.UTC),
			Message:   &jsonl.Message{Content: json.RawMessage(`"what next?"`)},
		},
	}

	stats := analyzer.Analyze(entries)

	return messagesModel{
		session:  session.Info{SessionID: "test-123", ProjectName: "testproj"},
		entries:  entries,
		stats:    stats,
		selected: make(map[int]bool),
		width:    120,
		height:   40,
	}
}

func TestMessagesModel_Navigation(t *testing.T) {
	m := testMessagesModel()

	// Move down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1, got %d", m.cursor)
	}

	// Move up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", m.cursor)
	}
}

func TestMessagesModel_Select(t *testing.T) {
	m := testMessagesModel()

	// Select first entry with space
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.selected[0] {
		t.Error("expected entry 0 to be selected")
	}

	// Deselect
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.selected[0] {
		t.Error("expected entry 0 to be deselected")
	}
}

func TestMessagesModel_SelectAllProgress(t *testing.T) {
	m := testMessagesModel()

	// Press x to select all progress
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if !m.selected[2] {
		t.Error("expected progress message at index 2 to be selected")
	}
	if m.selected[0] || m.selected[1] || m.selected[3] {
		t.Error("expected only progress messages to be selected")
	}
}

func TestMessagesModel_ActiveReadOnly(t *testing.T) {
	m := testMessagesModel()
	m.isActive = true

	// Space should not select
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if len(m.selected) > 0 {
		t.Error("expected no selection in active mode")
	}
}

func TestMessagesModel_Impact(t *testing.T) {
	m := testMessagesModel()

	// Select two entries
	m.selected[0] = true
	m.selected[1] = true
	m.updateImpact()

	if m.impact == nil {
		t.Fatal("expected impact to be calculated")
	}
	if m.impact.SelectedCount < 2 {
		t.Errorf("expected at least 2 selected, got %d", m.impact.SelectedCount)
	}
	if m.impact.EstimatedTokenSaved <= 0 {
		t.Error("expected positive token savings")
	}
}

func TestMessagesModel_ImpactEmpty(t *testing.T) {
	m := testMessagesModel()
	m.updateImpact()
	if m.impact != nil {
		t.Error("expected nil impact when nothing selected")
	}
}

func TestMessagesModel_View(t *testing.T) {
	m := testMessagesModel()
	view := m.View()

	if view == "" {
		t.Error("expected non-empty view")
	}
	if !containsStr(view, "Context:") {
		t.Error("expected context meter in view")
	}
	if !containsStr(view, "testproj") {
		t.Error("expected project name in view")
	}
}

func TestMessagesModel_ContextMeter(t *testing.T) {
	m := testMessagesModel()
	meter := m.renderContextMeter()

	if !containsStr(meter, "Context:") {
		t.Error("expected 'Context:' in meter")
	}
	if !containsStr(meter, "Compactions:") {
		t.Error("expected 'Compactions:' in meter")
	}
}

func TestMessagesModel_ImpactBar(t *testing.T) {
	m := testMessagesModel()

	// No selection
	bar := m.renderImpactBar()
	if !containsStr(bar, "Selected: 0") {
		t.Error("expected 'Selected: 0' in empty impact bar")
	}

	// With selection
	m.selected[0] = true
	m.updateImpact()
	bar = m.renderImpactBar()
	if !containsStr(bar, "Selected:") {
		t.Error("expected 'Selected:' in impact bar")
	}
}

func TestTypeIcon(t *testing.T) {
	tests := []struct {
		typ  jsonl.MessageType
		want string
	}{
		{jsonl.TypeUser, "user"},
		{jsonl.TypeAssistant, "assistant"},
		{jsonl.TypeProgress, "progress"},
		{jsonl.TypeFileHistorySnapshot, "snapshot"},
	}
	for _, tt := range tests {
		got := typeIcon(tt.typ)
		if got != tt.want {
			t.Errorf("typeIcon(%s) = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestFormatTokensShort(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{500, "~500"},
		{1500, "~1.5K"},
		{10000, "~10.0K"},
	}
	for _, tt := range tests {
		got := formatTokensShort(tt.n)
		if got != tt.want {
			t.Errorf("formatTokensShort(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestFormatTokensFull(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{500, "500"},
		{1500, "1,500"},
		{200000, "200,000"},
	}
	for _, tt := range tests {
		got := formatTokensFull(tt.n)
		if got != tt.want {
			t.Errorf("formatTokensFull(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestShortUUID(t *testing.T) {
	if got := shortUUID("88595b64-49ce-4e3d-a315-aa789b30f677"); got != "88595b64" {
		t.Errorf("expected 88595b64, got %s", got)
	}
	if got := shortUUID("short"); got != "short" {
		t.Errorf("expected short, got %s", got)
	}
}
