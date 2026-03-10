package analyzer

import (
	"encoding/json"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func makeEntry(typ jsonl.MessageType, uuid, parentUUID string) jsonl.Entry {
	return jsonl.Entry{Type: typ, UUID: uuid, ParentUUID: parentUUID}
}

func makeAssistantWithToolUse(uuid, parentUUID, toolID string) jsonl.Entry {
	content, _ := json.Marshal([]jsonl.ContentBlock{{Type: "tool_use", ID: toolID, Name: "test"}})
	return jsonl.Entry{
		Type:       jsonl.TypeAssistant,
		UUID:       uuid,
		ParentUUID: parentUUID,
		Message:    &jsonl.Message{Role: "assistant", Content: content},
	}
}

func makeUserWithToolResult(uuid, parentUUID, toolUseID string) jsonl.Entry {
	content, _ := json.Marshal([]jsonl.ContentBlock{{Type: "tool_result", ToolUseID: toolUseID}})
	return jsonl.Entry{
		Type:       jsonl.TypeUser,
		UUID:       uuid,
		ParentUUID: parentUUID,
		Message:    &jsonl.Message{Role: "user", Content: content},
	}
}

func noKeep(_ string) bool { return false }

func TestCascadeDeleteSet_Linear(t *testing.T) {
	// Chain: A → B → C. Delete A, expect B and C to cascade.
	entries := []jsonl.Entry{
		makeEntry(jsonl.TypeUser, "a", ""),
		makeEntry(jsonl.TypeAssistant, "b", "a"),
		makeEntry(jsonl.TypeUser, "c", "b"),
	}
	initial := map[int]bool{0: true} // delete A

	result := CascadeDeleteSet(entries, initial, noKeep)

	if len(result) != 3 {
		t.Errorf("expected 3 deletions, got %d", len(result))
	}
	for i := 0; i < 3; i++ {
		if !result[i] {
			t.Errorf("entry %d should be in delete set", i)
		}
	}
}

func TestCascadeDeleteSet_ToolOrphan(t *testing.T) {
	// Assistant has tool_use "t1", user has tool_result "t1".
	// Delete assistant, expect user to cascade.
	entries := []jsonl.Entry{
		makeEntry(jsonl.TypeUser, "u1", ""),
		makeAssistantWithToolUse("a1", "u1", "t1"),
		makeUserWithToolResult("u2", "a1", "t1"),
		makeEntry(jsonl.TypeAssistant, "a2", "u2"),
	}
	initial := map[int]bool{1: true} // delete assistant a1

	result := CascadeDeleteSet(entries, initial, noKeep)

	// a1 deleted → u2 orphaned (tool_result t1) AND chain-broken (parent a1)
	// u2 deleted → a2 chain-broken (parent u2)
	if len(result) != 3 {
		t.Errorf("expected 3 deletions (a1, u2, a2), got %d", len(result))
	}
	if !result[1] || !result[2] || !result[3] {
		t.Errorf("expected entries 1, 2, 3 in delete set, got %v", result)
	}
}

func TestCascadeDeleteSet_KeepGuard(t *testing.T) {
	// Chain: A → B → C. Delete A, but B is KEEP-marked. C should NOT cascade.
	entries := []jsonl.Entry{
		makeEntry(jsonl.TypeUser, "a", ""),
		makeEntry(jsonl.TypeAssistant, "b", "a"),
		makeEntry(jsonl.TypeUser, "c", "b"),
	}
	initial := map[int]bool{0: true}
	isKept := func(uuid string) bool { return uuid == "b" }

	result := CascadeDeleteSet(entries, initial, isKept)

	if len(result) != 1 {
		t.Errorf("expected 1 deletion (only A), got %d: %v", len(result), result)
	}
	if !result[0] {
		t.Error("entry 0 should be in delete set")
	}
}

func TestCascadeDeleteSet_Empty(t *testing.T) {
	entries := []jsonl.Entry{
		makeEntry(jsonl.TypeUser, "a", ""),
	}
	result := CascadeDeleteSet(entries, map[int]bool{}, noKeep)
	if len(result) != 0 {
		t.Errorf("expected 0 deletions, got %d", len(result))
	}
}

func TestCascadeDeleteSet_NoExpansion(t *testing.T) {
	// Delete an entry with no children and no tool_uses.
	entries := []jsonl.Entry{
		makeEntry(jsonl.TypeUser, "a", ""),
		makeEntry(jsonl.TypeAssistant, "b", "a"),
		makeEntry(jsonl.TypeUser, "c", "b"),
	}
	initial := map[int]bool{2: true} // delete C (leaf, no children)

	result := CascadeDeleteSet(entries, initial, noKeep)

	if len(result) != 1 {
		t.Errorf("expected 1 deletion (only C), got %d", len(result))
	}
}
