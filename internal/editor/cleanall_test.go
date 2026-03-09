package editor

import (
	"os"
	"testing"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestCleanAll_Basic(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	result, err := CleanAll(path, CleanAllOpts{})
	if err != nil {
		t.Fatalf("clean all: %v", err)
	}

	// small_session.jsonl has 2 progress and 1 file-history-snapshot
	if result.ProgressRemoved != 2 {
		t.Errorf("expected 2 progress removed, got %d", result.ProgressRemoved)
	}
	if result.SnapshotsRemoved != 1 {
		t.Errorf("expected 1 snapshot removed, got %d", result.SnapshotsRemoved)
	}
	if result.BytesAfter >= result.BytesBefore {
		t.Error("expected bytes after < bytes before")
	}

	// Backup should exist (single undo point)
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Error("expected backup to exist after clean all")
	}

	// .bak.orig should NOT exist (cleaned up)
	if _, err := os.Stat(path + ".bak.orig"); !os.IsNotExist(err) {
		t.Error("expected .bak.orig to be cleaned up")
	}

	// Verify no progress or snapshot entries remain
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	for _, e := range entries {
		if e.Type == jsonl.TypeProgress {
			t.Error("expected no progress entries after clean all")
		}
		if e.Type == jsonl.TypeFileHistorySnapshot {
			t.Error("expected no snapshot entries after clean all")
		}
	}
}

func TestCleanAll_OrphanCascade(t *testing.T) {
	path := copyFixture(t, "orphan_cascade.jsonl")

	result, err := CleanAll(path, CleanAllOpts{})
	if err != nil {
		t.Fatalf("clean all: %v", err)
	}

	// Tangent entries should have been removed
	if result.TangentsRemoved == 0 {
		t.Error("expected tangent entries to be removed")
	}

	// After clean, fix --apply should find NO issues (the core WO-113 guarantee)
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	diagnosis := analyzer.Diagnose(entries)
	if len(diagnosis.Issues) > 0 {
		for _, issue := range diagnosis.Issues {
			t.Errorf("unexpected issue after clean: [%s] line %d: %s",
				issue.Kind, entries[issue.EntryIndex].LineNumber, issue.Description)
		}
	}

	// Chain should be healthy
	integrity := analyzer.CheckIntegrity(entries)
	if !integrity.Healthy {
		t.Errorf("chain unhealthy after clean: %d issues", len(integrity.Issues))
	}
}

func TestCleanAll_NoOrphansAfterTangentRemoval(t *testing.T) {
	path := copyFixture(t, "tangent_session.jsonl")

	_, err := CleanAll(path, CleanAllOpts{})
	if err != nil {
		t.Fatalf("clean all: %v", err)
	}

	// Verify: no orphaned tool_results remain
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	// Build tool_use ID set
	toolUseIDs := make(map[string]bool)
	for _, e := range entries {
		for _, id := range e.ToolUseIDs() {
			toolUseIDs[id] = true
		}
	}

	// Check for orphaned tool_results
	for _, e := range entries {
		if e.Type != jsonl.TypeUser || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID != "" && !toolUseIDs[b.ToolUseID] {
				t.Errorf("orphaned tool_result referencing %s in entry %s", b.ToolUseID, e.UUID)
			}
		}
	}
}

func TestCleanAll_UndoRestoresOriginal(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	// Count original entries
	origEntries, _ := jsonl.Parse(path)
	origCount := len(origEntries)

	_, err := CleanAll(path, CleanAllOpts{})
	if err != nil {
		t.Fatalf("clean all: %v", err)
	}

	// Entries should be fewer now
	afterEntries, _ := jsonl.Parse(path)
	if len(afterEntries) >= origCount {
		t.Error("expected fewer entries after clean all")
	}

	// Undo: restore from .bak
	if err := os.Rename(path+".bak", path); err != nil {
		t.Fatalf("undo: %v", err)
	}

	// Should be back to original count
	restored, _ := jsonl.Parse(path)
	if len(restored) != origCount {
		t.Errorf("expected %d entries after undo, got %d", origCount, len(restored))
	}
}

func TestCleanAll_Tombstone(t *testing.T) {
	path := copyFixture(t, "orphan_cascade.jsonl")

	// Count entries before
	beforeEntries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	beforeCount := len(beforeEntries)

	result, err := CleanAll(path, CleanAllOpts{Tombstone: true})
	if err != nil {
		t.Fatalf("clean all tombstone: %v", err)
	}

	// With tombstone mode, orphans should be tombstoned not deleted
	if result.TangentsRemoved == 0 {
		t.Error("expected tangent entries to be removed")
	}

	// After clean with tombstone, should have no issues
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	diagnosis := analyzer.Diagnose(entries)
	if len(diagnosis.Issues) > 0 {
		for _, issue := range diagnosis.Issues {
			t.Errorf("unexpected issue after tombstone clean: [%s] line %d: %s",
				issue.Kind, entries[issue.EntryIndex].LineNumber, issue.Description)
		}
	}

	// Tombstone mode preserves entry count better than delete mode
	// (orphans are replaced, not removed)
	afterCount := len(entries)
	if result.OrphansTombstoned > 0 {
		// With tombstone, fewer entries should be removed overall
		// compared to what would have been removed without tombstone
		t.Logf("tombstoned=%d, orphans_removed=%d, before=%d, after=%d",
			result.OrphansTombstoned, result.OrphansRemoved, beforeCount, afterCount)
	}
}
