package editor

import (
	"testing"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestDeduplicateReads_Basic(t *testing.T) {
	path := copyFixture(t, "duplicate_reads.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	dupResult := analyzer.FindDuplicateReads(entries)
	if len(dupResult.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(dupResult.Groups))
	}

	result, err := DeduplicateReads(path, dupResult)
	if err != nil {
		t.Fatalf("dedup: %v", err)
	}

	if result.StaleReadsRemoved != 1 {
		t.Errorf("expected 1 stale read removed, got %d", result.StaleReadsRemoved)
	}
	if result.EntriesRemoved < 1 {
		t.Errorf("expected at least 1 entry removed, got %d", result.EntriesRemoved)
	}
	if result.BytesAfter >= result.BytesBefore {
		t.Error("expected bytes after < bytes before")
	}

	// Verify the latest read is preserved
	after, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	// The latest read (toolu_r2) should still exist
	foundLatest := false
	for _, e := range after {
		if e.Message == nil {
			continue
		}
		blocks, _ := jsonl.ParseContentBlocks(e.Message.Content)
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID == "toolu_r2" {
				foundLatest = true
			}
			if b.Type == "tool_use" && b.ID == "toolu_r1" {
				t.Error("stale read toolu_r1 should have been removed")
			}
		}
	}
	if !foundLatest {
		t.Error("latest read toolu_r2 should be preserved")
	}
}

func TestDeduplicateReads_MixedContent(t *testing.T) {
	path := copyFixture(t, "mixed_dedup.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	dupResult := analyzer.FindDuplicateReads(entries)
	if len(dupResult.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(dupResult.Groups))
	}

	result, err := DeduplicateReads(path, dupResult)
	if err != nil {
		t.Fatalf("dedup: %v", err)
	}

	if result.StaleReadsRemoved != 1 {
		t.Errorf("expected 1 stale read removed, got %d", result.StaleReadsRemoved)
	}

	// The assistant message a1 had text + 2 tool_use blocks.
	// Only toolu_r1 (stale main.go read) should be removed.
	// toolu_r2 (util.go, not duplicated) should remain.
	after, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	foundR2 := false
	foundR3 := false
	for _, e := range after {
		if e.Message == nil {
			continue
		}
		blocks, _ := jsonl.ParseContentBlocks(e.Message.Content)
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID == "toolu_r1" {
				t.Error("stale read toolu_r1 should have been removed from mixed content")
			}
			if b.Type == "tool_use" && b.ID == "toolu_r2" {
				foundR2 = true
			}
			if b.Type == "tool_use" && b.ID == "toolu_r3" {
				foundR3 = true
			}
			if b.Type == "tool_result" && b.ToolUseID == "toolu_r1" {
				t.Error("stale tool_result for toolu_r1 should have been removed")
			}
		}
	}
	if !foundR2 {
		t.Error("non-duplicate toolu_r2 (util.go) should be preserved")
	}
	if !foundR3 {
		t.Error("latest read toolu_r3 (main.go) should be preserved")
	}

	// The text block in a1 should be preserved
	foundText := false
	for _, e := range after {
		if e.UUID == "a1" && e.Message != nil {
			blocks, _ := jsonl.ParseContentBlocks(e.Message.Content)
			for _, b := range blocks {
				if b.Type == "text" && b.Text == "Let me read both files." {
					foundText = true
				}
			}
		}
	}
	if !foundText {
		t.Error("text block in a1 should be preserved after surgery")
	}
}

func TestDeduplicateReads_NoDuplicates(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	dupResult := analyzer.FindDuplicateReads(entries)

	result, err := DeduplicateReads(path, dupResult)
	if err != nil {
		t.Fatalf("dedup: %v", err)
	}

	if result.StaleReadsRemoved != 0 {
		t.Errorf("expected 0 stale reads removed, got %d", result.StaleReadsRemoved)
	}
	if result.EntriesRemoved != 0 {
		t.Errorf("expected 0 entries removed, got %d", result.EntriesRemoved)
	}
}

func TestDeduplicateReads_NilResult(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	result, err := DeduplicateReads(path, nil)
	if err != nil {
		t.Fatalf("dedup: %v", err)
	}
	if result.StaleReadsRemoved != 0 {
		t.Errorf("expected 0, got %d", result.StaleReadsRemoved)
	}
}

func TestDeduplicateReads_ChainRepair(t *testing.T) {
	path := copyFixture(t, "duplicate_reads.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	dupResult := analyzer.FindDuplicateReads(entries)
	result, err := DeduplicateReads(path, dupResult)
	if err != nil {
		t.Fatalf("dedup: %v", err)
	}

	// Entries that pointed to deleted entries should be reparented
	after, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	// Check all parentUuid references point to existing entries
	uuids := make(map[string]bool)
	for _, e := range after {
		if e.UUID != "" {
			uuids[e.UUID] = true
		}
	}
	for _, e := range after {
		if e.ParentUUID != "" && !uuids[e.ParentUUID] {
			t.Errorf("orphaned parentUuid: entry %s points to missing %s", e.UUID, e.ParentUUID)
		}
	}

	_ = result // used above
}
