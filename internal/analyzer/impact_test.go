package analyzer

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestAutoSelectProgress(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1"},
		{
			Type: jsonl.TypeAssistant, UUID: "a1",
			Message: &jsonl.Message{
				Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{}}]`),
			},
		},
		{Type: jsonl.TypeProgress, UUID: "p1", ToolUseID: "toolu_1"},
		{Type: jsonl.TypeProgress, UUID: "p2", ToolUseID: "toolu_1"},
		{Type: jsonl.TypeProgress, UUID: "p3", ToolUseID: "toolu_other"},
		{Type: jsonl.TypeUser, UUID: "u2"},
	}

	selected := map[int]bool{1: true} // Select the assistant message
	expanded := AutoSelectProgress(entries, selected)

	// Should auto-select p1 (index 2) and p2 (index 3), but not p3 (index 4)
	if !expanded[2] {
		t.Error("expected progress p1 to be auto-selected")
	}
	if !expanded[3] {
		t.Error("expected progress p2 to be auto-selected")
	}
	if expanded[4] {
		t.Error("expected progress p3 to NOT be auto-selected")
	}
}

func TestAutoSelectProgress_NoToolUse(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", Message: &jsonl.Message{Content: json.RawMessage(`"text"`)}},
		{Type: jsonl.TypeProgress, UUID: "p1", ToolUseID: "toolu_1"},
	}

	selected := map[int]bool{0: true} // Select user message (no tool_use)
	expanded := AutoSelectProgress(entries, selected)

	if expanded[1] {
		t.Error("expected progress to NOT be selected when user message is selected")
	}
}

func TestPredictImpact_Basic(t *testing.T) {
	entries, err := jsonl.Parse(filepath.Join("..", "..", "testdata", "small_session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	stats := Analyze(entries)

	// Select the first user and assistant messages (indices 0, 1)
	selected := map[int]bool{0: true, 1: true}
	impact := PredictImpact(entries, selected, stats)

	if impact.SelectedCount < 2 {
		t.Errorf("expected at least 2 selected, got %d", impact.SelectedCount)
	}
	if impact.EstimatedTokenSaved <= 0 {
		t.Error("expected positive token savings")
	}
	if impact.NewContextPercent >= stats.UsagePercent {
		t.Error("expected reduced context percentage")
	}
}

func TestPredictImpact_WithCompaction(t *testing.T) {
	entries, err := jsonl.Parse(filepath.Join("..", "..", "testdata", "compaction.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	stats := Analyze(entries)

	// Select a pre-compaction message
	selected := map[int]bool{0: true} // First user message
	impact := PredictImpact(entries, selected, stats)

	// Should warn about pre-compaction message
	if len(impact.Warnings) == 0 {
		t.Error("expected warning about pre-compaction message")
	}
}

func TestPredictImpact_ChainRepairs(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1"},
		{Type: jsonl.TypeAssistant, UUID: "a1", ParentUUID: "u1",
			Message: &jsonl.Message{Content: json.RawMessage(`[{"type":"text","text":"hi"}]`)}},
		{Type: jsonl.TypeUser, UUID: "u2", ParentUUID: "a1"},
	}

	stats := &ContextStats{CurrentContextTokens: 10000, TokenGrowthRate: 1000}

	// Delete a1 — u2 needs repair
	selected := map[int]bool{1: true}
	impact := PredictImpact(entries, selected, stats)

	if impact.ChainRepairs != 1 {
		t.Errorf("expected 1 chain repair, got %d", impact.ChainRepairs)
	}
}

func TestPredictImpact_Empty(t *testing.T) {
	stats := &ContextStats{CurrentContextTokens: 10000}
	impact := PredictImpact(nil, nil, stats)
	if impact.SelectedCount != 0 {
		t.Errorf("expected 0 selected, got %d", impact.SelectedCount)
	}
}
