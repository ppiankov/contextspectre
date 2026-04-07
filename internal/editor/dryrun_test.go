package editor

import (
	"os"
	"testing"
	"time"
)

func TestDryRunCleanLive_NoWrite(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	// Set mtime in the past so idle check passes
	past := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}

	fi1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mtimeBefore := fi1.ModTime()
	sizeBefore := fi1.Size()

	result, err := CleanLive(path, CleanLiveOpts{DryRun: true})
	if err != nil {
		t.Fatalf("DryRunCleanLive: %v", err)
	}

	// File must not have been modified
	fi2, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !fi2.ModTime().Equal(mtimeBefore) {
		t.Errorf("file mtime changed: %v → %v", mtimeBefore, fi2.ModTime())
	}
	if fi2.Size() != sizeBefore {
		t.Errorf("file size changed: %d → %d", sizeBefore, fi2.Size())
	}

	// Result should report reclaimable entries
	if result.BytesBefore == 0 {
		t.Error("expected non-zero BytesBefore")
	}

	// No .bak or .bak.orig should exist
	for _, suffix := range []string{".bak", ".bak.orig"} {
		if _, err := os.Stat(path + suffix); err == nil {
			t.Errorf("backup file %s should not exist in dry-run", suffix)
		}
	}

	// No .lock file should remain held (it's created but released)
	// The .lock file itself may exist on disk — that's fine
}

func TestDryRunCleanLive_CountsProgress(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	result, err := CleanLive(path, CleanLiveOpts{DryRun: true})
	if err != nil {
		t.Fatalf("DryRunCleanLive: %v", err)
	}

	// small_session.jsonl has progress entries
	if result.ProgressRemoved == 0 {
		t.Error("expected progress entries to be counted")
	}
}

func TestDryRunCleanLive_DoesNotRequireIdle(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")
	// Touch the file to make it very recent — dry-run should still work
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}

	// DryRun should NOT return ErrSessionNotIdle
	result, err := CleanLive(path, CleanLiveOpts{DryRun: true})
	if err != nil {
		t.Fatalf("expected dry-run to succeed on non-idle file, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
