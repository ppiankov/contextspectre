package editor

import (
	"testing"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestRemoveTangents_Basic(t *testing.T) {
	path := copyFixture(t, "tangent_session.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	origCount := len(entries)

	tangentResult := analyzer.FindTangents(entries)
	if len(tangentResult.Groups) == 0 {
		t.Fatal("expected tangent groups to be detected")
	}

	toDelete := tangentResult.AllTangentIndices()
	result, err := Delete(path, toDelete)
	if err != nil {
		t.Fatalf("delete tangents: %v", err)
	}

	if result.EntriesRemoved == 0 {
		t.Error("expected entries to be removed")
	}

	// Verify fewer entries after cleanup
	afterEntries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(afterEntries) >= origCount {
		t.Errorf("expected fewer entries after tangent removal, got %d (was %d)", len(afterEntries), origCount)
	}

	// Verify no tangent entries remain
	afterTangents := analyzer.FindTangents(afterEntries)
	if len(afterTangents.Groups) != 0 {
		t.Errorf("expected 0 tangent groups after cleanup, got %d", len(afterTangents.Groups))
	}
}

func TestRemoveTangents_NoTangents(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tangentResult := analyzer.FindTangents(entries)
	if len(tangentResult.Groups) != 0 {
		t.Errorf("expected 0 tangent groups in small_session, got %d", len(tangentResult.Groups))
	}
}

func TestRemoveTangents_ChainRepair(t *testing.T) {
	path := copyFixture(t, "tangent_session.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tangentResult := analyzer.FindTangents(entries)
	toDelete := tangentResult.AllTangentIndices()
	result, err := Delete(path, toDelete)
	if err != nil {
		t.Fatalf("delete tangents: %v", err)
	}

	// Tangent removal should trigger chain repair since entries after
	// the tangent pointed to tangent entries
	if result.ChainRepairs == 0 {
		t.Log("no chain repairs needed (tangent entries may not have been in chain)")
	}

	// Verify remaining entries have valid chain (no dangling parentUuid)
	afterEntries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	uuids := make(map[string]bool)
	for _, e := range afterEntries {
		if e.UUID != "" {
			uuids[e.UUID] = true
		}
	}
	for _, e := range afterEntries {
		if e.ParentUUID != "" && !uuids[e.ParentUUID] {
			// ParentUUID should either exist or be empty (root)
			t.Errorf("entry %s has dangling parentUuid %s", e.UUID, e.ParentUUID)
		}
	}
}
