package editor

import (
	"os"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestCleanAll_Basic(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	result, err := CleanAll(path)
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

func TestCleanAll_UndoRestoresOriginal(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	// Count original entries
	origEntries, _ := jsonl.Parse(path)
	origCount := len(origEntries)

	_, err := CleanAll(path)
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
