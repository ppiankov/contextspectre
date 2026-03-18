package editor

import (
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestIsIdle_OldFile(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")
	// Set mtime to 10 seconds ago
	past := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	idle, mtime, err := IsIdle(path, DefaultIdleThreshold)
	if err != nil {
		t.Fatalf("IsIdle: %v", err)
	}
	if !idle {
		t.Error("expected file to be idle")
	}
	if mtime.IsZero() {
		t.Error("expected non-zero mtime")
	}
}

func TestIsIdle_RecentFile(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")
	// Touch the file to make it very recent
	if err := os.Chtimes(path, time.Now(), time.Now()); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	idle, _, err := IsIdle(path, 5*time.Second)
	if err != nil {
		t.Fatalf("IsIdle: %v", err)
	}
	if idle {
		t.Error("expected file to not be idle")
	}
}

func TestCheckRace_NoRace(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")
	fi, _ := os.Stat(path)
	expected := fi.ModTime()

	err := checkRace(path, expected)
	if err != nil {
		t.Fatalf("expected no race, got: %v", err)
	}
}

func TestCheckRace_Detected(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")
	fi, _ := os.Stat(path)
	expected := fi.ModTime()

	// Modify the file to change mtime
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if _, err := f.Write([]byte("\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err := checkRace(path, expected)
	if !errors.Is(err, ErrRaceDetected) {
		t.Fatalf("expected ErrRaceDetected, got: %v", err)
	}
}

func TestCleanLive_Basic(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	// Set mtime to the past so IsIdle passes
	past := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	result, err := CleanLive(path, CleanLiveOpts{Threshold: 1 * time.Second})
	if err != nil {
		t.Fatalf("CleanLive: %v", err)
	}

	if result.ProgressRemoved != 2 {
		t.Errorf("expected 2 progress removed, got %d", result.ProgressRemoved)
	}
	if result.SnapshotsRemoved != 1 {
		t.Errorf("expected 1 snapshot removed, got %d", result.SnapshotsRemoved)
	}

	// Verify aggressive-only fields are zero
	if result.ImagesReplaced != 0 {
		t.Errorf("expected 0 images replaced, got %d", result.ImagesReplaced)
	}

	// Verify .bak exists, .bak.orig does not
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Error("expected .bak to exist")
	}
	if _, err := os.Stat(path + ".bak.orig"); !os.IsNotExist(err) {
		t.Error("expected .bak.orig to not exist")
	}

	// Verify no progress or snapshot entries remain
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, e := range entries {
		if e.Type == jsonl.TypeProgress {
			t.Error("found progress entry after live clean")
		}
		if e.Type == jsonl.TypeFileHistorySnapshot {
			t.Error("found snapshot entry after live clean")
		}
	}

	// Verify byte savings
	if result.BytesAfter >= result.BytesBefore {
		t.Error("expected file to shrink")
	}
	if result.TotalTokensSaved <= 0 {
		t.Error("expected positive token savings")
	}
}

func TestCleanLive_Aggressive(t *testing.T) {
	path := copyFixture(t, "with_images.jsonl")

	past := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	result, err := CleanLive(path, CleanLiveOpts{
		Aggressive: true,
		Threshold:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("CleanLive aggressive: %v", err)
	}

	if result.ImagesReplaced == 0 {
		t.Error("expected images to be replaced in aggressive mode")
	}
}

func TestCleanLive_NotIdle(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")
	// Touch the file to make mtime = now
	if err := os.Chtimes(path, time.Now(), time.Now()); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	_, err := CleanLive(path, CleanLiveOpts{Threshold: 5 * time.Second})
	if !errors.Is(err, ErrSessionNotIdle) {
		t.Fatalf("expected ErrSessionNotIdle, got: %v", err)
	}

	// Verify no backup was created
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("expected no .bak when session not idle")
	}
	if _, err := os.Stat(path + ".bak.orig"); !os.IsNotExist(err) {
		t.Error("expected no .bak.orig when session not idle")
	}
}

func TestCleanLive_RaceDetected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file locking semantics differ on Windows")
	}
	path := copyFixture(t, "small_session.jsonl")
	origData, _ := os.ReadFile(path)

	past := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// Run first: verify basic CleanLive works (establishes baseline)
	result, err := CleanLive(path, CleanLiveOpts{Threshold: 1 * time.Second})
	if err != nil {
		t.Fatalf("baseline CleanLive: %v", err)
	}
	if result.ProgressRemoved == 0 && result.SnapshotsRemoved == 0 {
		t.Skip("fixture has nothing to clean, can't test race")
	}

	// Restore original and test race detection via checkRace directly
	if err := os.WriteFile(path, origData, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_ = os.Remove(path + ".bak")

	fi, _ := os.Stat(path)
	origMtime := fi.ModTime()

	// Simulate Claude writing by appending
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if _, err := f.Write([]byte("\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err = checkRace(path, origMtime)
	if !errors.Is(err, ErrRaceDetected) {
		t.Fatalf("expected ErrRaceDetected, got: %v", err)
	}
}

func TestCleanLive_NoChanges(t *testing.T) {
	// tangent_session has only user/assistant entries — nothing for live to clean
	path := copyFixture(t, "tangent_session.jsonl")

	past := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	result, err := CleanLive(path, CleanLiveOpts{Threshold: 1 * time.Second})
	if err != nil {
		t.Fatalf("CleanLive: %v", err)
	}

	if result.ProgressRemoved != 0 {
		t.Errorf("expected 0 progress, got %d", result.ProgressRemoved)
	}
	if result.SnapshotsRemoved != 0 {
		t.Errorf("expected 0 snapshots, got %d", result.SnapshotsRemoved)
	}
}

func TestCleanLive_Tier6NeverRuns(t *testing.T) {
	// tangent_session has cross-repo tangent entries
	path := copyFixture(t, "tangent_session.jsonl")

	past := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	entriesBefore, _ := jsonl.Parse(path)
	countBefore := len(entriesBefore)

	// Run with aggressive — still should not remove tangents
	_, err := CleanLive(path, CleanLiveOpts{
		Aggressive: true,
		Threshold:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("CleanLive: %v", err)
	}

	entriesAfter, _ := jsonl.Parse(path)
	if len(entriesAfter) != countBefore {
		t.Errorf("expected %d entries (tangents preserved), got %d", countBefore, len(entriesAfter))
	}
}
