package editor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestFindOrphanedToolUses_None(t *testing.T) {
	entries := []jsonl.Entry{
		{
			Type: jsonl.TypeAssistant,
			UUID: "a1",
			Message: &jsonl.Message{
				Role: "assistant",
				Content: mustMarshal(t, []jsonl.ContentBlock{
					{Type: "tool_use", ID: "toolu_1", Name: "Read"},
				}),
			},
		},
		{
			Type: jsonl.TypeUser,
			UUID: "u1",
			Message: &jsonl.Message{
				Role: "user",
				Content: mustMarshal(t, []jsonl.ContentBlock{
					{Type: "tool_result", ToolUseID: "toolu_1", Content: mustMarshalRaw(t, "ok")},
				}),
			},
		},
	}

	orphans := FindOrphanedToolUses(entries)
	if len(orphans) != 0 {
		t.Fatalf("expected 0 orphans, got %d", len(orphans))
	}
}

func TestFindOrphanedToolUses_Found(t *testing.T) {
	entries := []jsonl.Entry{
		{
			Type: jsonl.TypeAssistant,
			UUID: "a1",
			Message: &jsonl.Message{
				Role: "assistant",
				Content: mustMarshal(t, []jsonl.ContentBlock{
					{Type: "tool_use", ID: "toolu_1", Name: "Read"},
					{Type: "tool_use", ID: "toolu_2", Name: "Bash"},
				}),
			},
		},
		{
			Type: jsonl.TypeUser,
			UUID: "u1",
			Message: &jsonl.Message{
				Role: "user",
				Content: mustMarshal(t, []jsonl.ContentBlock{
					{Type: "tool_result", ToolUseID: "toolu_1", Content: mustMarshalRaw(t, "ok")},
					// toolu_2 result is MISSING
				}),
			},
		},
	}

	orphans := FindOrphanedToolUses(entries)
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}
	if orphans[0].ToolUseID != "toolu_2" {
		t.Fatalf("expected orphan toolu_2, got %s", orphans[0].ToolUseID)
	}
	if orphans[0].ToolName != "Bash" {
		t.Fatalf("expected tool name Bash, got %s", orphans[0].ToolName)
	}
}

func TestRewire_InjectsSyntheticResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	// Assistant with tool_use but no matching tool_result.
	lines := []string{
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{}}]}}`,
		`{"type":"user","uuid":"u1","parentUuid":"a1","message":{"role":"user","content":"next question"}}`,
	}

	var data []byte
	for _, l := range lines {
		data = append(data, []byte(l+"\n")...)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Rewire(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Injected != 1 {
		t.Fatalf("expected 1 injection, got %d", result.Injected)
	}

	// Parse the result and verify the synthetic entry exists.
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after rewire, got %d", len(entries))
	}

	// The synthetic entry should be at index 1 (after the assistant).
	synth := entries[1]
	if synth.Type != jsonl.TypeUser {
		t.Fatalf("expected user type, got %s", synth.Type)
	}
	blocks, err := jsonl.ParseContentBlocks(synth.Message.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != "tool_result" {
		t.Fatalf("expected tool_result, got %s", blocks[0].Type)
	}
	if blocks[0].ToolUseID != "toolu_1" {
		t.Fatalf("expected toolu_1, got %s", blocks[0].ToolUseID)
	}
}

func TestRewire_NoOrphans(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{}}]}}`,
		`{"type":"user","uuid":"u1","parentUuid":"a1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}}`,
	}

	var data []byte
	for _, l := range lines {
		data = append(data, []byte(l+"\n")...)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Rewire(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Injected != 0 {
		t.Fatalf("expected 0 injections, got %d", result.Injected)
	}
}

func mustMarshal(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustMarshalRaw(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
