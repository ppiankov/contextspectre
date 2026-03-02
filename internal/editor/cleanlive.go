package editor

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// ErrRaceDetected is returned when the JSONL file changes during a live edit,
// indicating Claude Code wrote between operations.
var ErrRaceDetected = errors.New("race detected: file modified during live edit")

// ErrSessionNotIdle is returned when the session mtime is too recent to safely edit.
var ErrSessionNotIdle = errors.New("session not idle: mtime too recent")

// DefaultIdleThreshold is the minimum time since last modification before
// a live edit is considered safe.
const DefaultIdleThreshold = 2 * time.Second

// CleanLiveOpts configures a live cleanup pass.
type CleanLiveOpts struct {
	Aggressive bool          // include Tier 4-5 (images, separators, truncation)
	Threshold  time.Duration // idle threshold (default: DefaultIdleThreshold)
}

// CleanLiveResult holds the combined results of live cleanup operations.
type CleanLiveResult struct {
	ProgressRemoved    int
	SnapshotsRemoved   int
	ImagesReplaced     int // Tier 4, only with Aggressive
	SeparatorsStripped int // Tier 4, only with Aggressive
	OutputsTruncated   int // Tier 5, only with Aggressive
	TotalTokensSaved   int
	BytesBefore        int64
	BytesAfter         int64
}

// IsIdle returns true if the file's mtime is older than threshold.
// Also returns the current mtime for subsequent race detection.
func IsIdle(path string, threshold time.Duration) (bool, time.Time, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("stat: %w", err)
	}
	mtime := fi.ModTime()
	return time.Since(mtime) >= threshold, mtime, nil
}

// checkRace verifies the file mtime matches the expected value.
// Returns ErrRaceDetected on mismatch.
func checkRace(path string, expected time.Time) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat for race check: %w", err)
	}
	if !fi.ModTime().Equal(expected) {
		return ErrRaceDetected
	}
	return nil
}

// recordMtime returns the current mtime of the file.
func recordMtime(path string) (time.Time, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return fi.ModTime(), nil
}

// CleanLive runs safe cleanup operations on a potentially active session.
//
// Tier 1: progress removal
// Tier 2: snapshot removal
// With Aggressive:
// Tier 4: image replacement, separator stripping
// Tier 5: bash output truncation
//
// Between each sub-operation, the file mtime is checked. If it changed
// (meaning Claude Code wrote to it), the operation aborts and restores
// from the original backup.
//
// Tier 6-7 operations (sidechains, tangents, retries, dedup) are NEVER
// available in live mode regardless of flags.
func CleanLive(path string, opts CleanLiveOpts) (*CleanLiveResult, error) {
	threshold := opts.Threshold
	if threshold == 0 {
		threshold = DefaultIdleThreshold
	}

	// Step 1: Check idle
	idle, _, err := IsIdle(path, threshold)
	if err != nil {
		return nil, err
	}
	if !idle {
		return nil, ErrSessionNotIdle
	}

	result := &CleanLiveResult{}

	// Get original size
	origInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	result.BytesBefore = origInfo.Size()

	// Create .bak.orig (the single undo point, same pattern as CleanAll)
	origBak := path + ".bak.orig"
	if err := copyFileForCleanAll(path, origBak); err != nil {
		return nil, fmt.Errorf("create original backup: %w", err)
	}

	// Track "our" last known mtime after each write
	ourMtime, err := recordMtime(path)
	if err != nil {
		_ = restoreOriginal(path, origBak)
		return nil, err
	}

	cleanIntermediate := func() {
		safecopy.Clean(path)
	}

	abortAndRestore := func() {
		safecopy.Clean(path)
		_ = os.Rename(origBak, path)
	}

	// runStep wraps each sub-operation with race check and mtime tracking.
	runStep := func(name string, fn func() (bool, error)) error {
		// Pre-check: has Claude written since our last operation?
		if err := checkRace(path, ourMtime); err != nil {
			abortAndRestore()
			return fmt.Errorf("%s: %w", name, err)
		}

		changed, err := fn()
		if err != nil {
			abortAndRestore()
			return fmt.Errorf("%s: %w", name, err)
		}

		if changed {
			cleanIntermediate()
			ourMtime, err = recordMtime(path)
			if err != nil {
				abortAndRestore()
				return err
			}
		}
		return nil
	}

	// --- Tier 1: Remove progress entries ---
	if err := runStep("progress", func() (bool, error) {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return false, err
		}
		toDelete := make(map[int]bool)
		for i, e := range entries {
			if e.Type == jsonl.TypeProgress {
				toDelete[i] = true
			}
		}
		if len(toDelete) == 0 {
			return false, nil
		}
		dr, err := Delete(path, toDelete)
		if err != nil {
			return false, err
		}
		result.ProgressRemoved = dr.EntriesRemoved
		return true, nil
	}); err != nil {
		return nil, err
	}

	// --- Tier 2: Remove snapshot entries ---
	if err := runStep("snapshots", func() (bool, error) {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return false, err
		}
		toDelete := make(map[int]bool)
		for i, e := range entries {
			if e.Type == jsonl.TypeFileHistorySnapshot {
				toDelete[i] = true
			}
		}
		if len(toDelete) == 0 {
			return false, nil
		}
		dr, err := Delete(path, toDelete)
		if err != nil {
			return false, err
		}
		result.SnapshotsRemoved = dr.EntriesRemoved
		return true, nil
	}); err != nil {
		return nil, err
	}

	// --- Tier 4: Image replacement (aggressive only) ---
	if opts.Aggressive {
		if err := runStep("images", func() (bool, error) {
			ir, err := ReplaceImages(path)
			if err != nil {
				return false, err
			}
			result.ImagesReplaced = ir.ImagesReplaced
			return ir.ImagesReplaced > 0, nil
		}); err != nil {
			return nil, err
		}
	}

	// --- Tier 4: Separator stripping (aggressive only) ---
	if opts.Aggressive {
		if err := runStep("separators", func() (bool, error) {
			sr, err := StripSeparators(path)
			if err != nil {
				return false, err
			}
			result.SeparatorsStripped = sr.LinesStripped
			return sr.LinesStripped > 0, nil
		}); err != nil {
			return nil, err
		}
	}

	// --- Tier 5: Bash output truncation (aggressive only) ---
	if opts.Aggressive {
		if err := runStep("truncate", func() (bool, error) {
			tr, err := TruncateOutputs(path, analyzer.LargeOutputThreshold, 10)
			if err != nil {
				return false, err
			}
			result.OutputsTruncated = tr.OutputsTruncated
			return tr.OutputsTruncated > 0, nil
		}); err != nil {
			return nil, err
		}
	}

	// Final race check before finalizing
	if err := checkRace(path, ourMtime); err != nil {
		abortAndRestore()
		return nil, fmt.Errorf("final: %w", err)
	}

	// Get final size
	finalInfo, err := os.Stat(path)
	if err != nil {
		abortAndRestore()
		return nil, fmt.Errorf("stat final: %w", err)
	}
	result.BytesAfter = finalInfo.Size()
	result.TotalTokensSaved = int(result.BytesBefore-result.BytesAfter) / 4

	// Finalize: move .bak.orig to .bak (same pattern as CleanAll)
	safecopy.Clean(path) // remove any remaining intermediate
	if err := os.Rename(origBak, path+".bak"); err != nil {
		return nil, fmt.Errorf("finalize backup: %w", err)
	}

	return result, nil
}
