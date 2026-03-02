package editor

import (
	"fmt"
	"os"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// CleanAllResult holds the combined results of all cleanup operations.
type CleanAllResult struct {
	ProgressRemoved    int
	SnapshotsRemoved   int
	SidechainsRemoved  int
	TangentsRemoved    int
	FailedRetries      int
	StaleReadsRemoved  int
	ImagesReplaced     int
	SeparatorsStripped int
	OutputsTruncated   int
	TotalTokensSaved   int
	BytesBefore        int64
	BytesAfter         int64
}

// CleanAll runs all cleanup operations in optimal order with a single backup.
// Order: entry deletions first, then content surgery.
func CleanAll(path string) (*CleanAllResult, error) {
	result := &CleanAllResult{}

	// Get original size
	origInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	result.BytesBefore = origInfo.Size()

	// Create original backup as .bak.orig (the single undo point)
	origBak := path + ".bak.orig"
	if err := copyFileForCleanAll(path, origBak); err != nil {
		return nil, fmt.Errorf("create original backup: %w", err)
	}

	// Helper: clean intermediate .bak files between operations
	cleanIntermediate := func() {
		safecopy.Clean(path)
	}

	// Phase 1: Entry-level deletions
	// These use editor.Delete which creates .bak

	// 1a. Remove progress entries
	entries, err := jsonl.Parse(path)
	if err != nil {
		_ = restoreOriginal(path, origBak)
		return nil, fmt.Errorf("parse: %w", err)
	}

	toDelete := make(map[int]bool)
	for i, e := range entries {
		if e.Type == jsonl.TypeProgress {
			toDelete[i] = true
		}
	}
	if len(toDelete) > 0 {
		dr, err := Delete(path, toDelete)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("progress: %w", err)
		}
		result.ProgressRemoved = dr.EntriesRemoved
		cleanIntermediate()
	}

	// 1b. Remove snapshot entries
	entries, _ = jsonl.Parse(path)
	toDelete = make(map[int]bool)
	for i, e := range entries {
		if e.Type == jsonl.TypeFileHistorySnapshot {
			toDelete[i] = true
		}
	}
	if len(toDelete) > 0 {
		dr, err := Delete(path, toDelete)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("snapshots: %w", err)
		}
		result.SnapshotsRemoved = dr.EntriesRemoved
		cleanIntermediate()
	}

	// 1c. Remove sidechain entries
	entries, _ = jsonl.Parse(path)
	toDelete = make(map[int]bool)
	for i, e := range entries {
		if e.IsSidechain {
			toDelete[i] = true
		}
	}
	if len(toDelete) > 0 {
		dr, err := Delete(path, toDelete)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("sidechains: %w", err)
		}
		result.SidechainsRemoved = dr.EntriesRemoved
		cleanIntermediate()
	}

	// 1d. Remove cross-repo tangents
	entries, _ = jsonl.Parse(path)
	tangentResult := analyzer.FindTangents(entries)
	if len(tangentResult.Groups) > 0 {
		toDelete = tangentResult.AllTangentIndices()
		dr, err := Delete(path, toDelete)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("tangents: %w", err)
		}
		result.TangentsRemoved = dr.EntriesRemoved
		cleanIntermediate()
	}

	// 1e. Remove failed retries
	entries, _ = jsonl.Parse(path)
	retryResult := analyzer.FindFailedRetries(entries)
	if len(retryResult.Sequences) > 0 {
		rr, err := RemoveFailedRetries(path, retryResult)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("retries: %w", err)
		}
		result.FailedRetries = rr.FailedRemoved
		cleanIntermediate()
	}

	// 1f. Remove stale reads
	entries, _ = jsonl.Parse(path)
	dupResult := analyzer.FindDuplicateReads(entries)
	if len(dupResult.Groups) > 0 {
		dr, err := DeduplicateReads(path, dupResult)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("dedup: %w", err)
		}
		result.StaleReadsRemoved = dr.StaleReadsRemoved
		cleanIntermediate()
	}

	// Phase 2: Content surgery

	// 2a. Replace images
	ir, err := ReplaceImages(path)
	if err != nil {
		_ = restoreOriginal(path, origBak)
		return nil, fmt.Errorf("images: %w", err)
	}
	result.ImagesReplaced = ir.ImagesReplaced
	if ir.ImagesReplaced > 0 {
		cleanIntermediate()
	}

	// 2b. Strip separators
	sr, err := StripSeparators(path)
	if err != nil {
		_ = restoreOriginal(path, origBak)
		return nil, fmt.Errorf("separators: %w", err)
	}
	result.SeparatorsStripped = sr.LinesStripped
	if sr.LinesStripped > 0 {
		cleanIntermediate()
	}

	// 2c. Truncate outputs
	tr, err := TruncateOutputs(path, analyzer.LargeOutputThreshold, 10)
	if err != nil {
		_ = restoreOriginal(path, origBak)
		return nil, fmt.Errorf("truncate: %w", err)
	}
	result.OutputsTruncated = tr.OutputsTruncated
	if tr.OutputsTruncated > 0 {
		cleanIntermediate()
	}

	// Get final size
	finalInfo, err := os.Stat(path)
	if err != nil {
		_ = restoreOriginal(path, origBak)
		return nil, fmt.Errorf("stat final: %w", err)
	}
	result.BytesAfter = finalInfo.Size()
	result.TotalTokensSaved = int(result.BytesBefore-result.BytesAfter) / 4

	// Move .bak.orig to .bak (the single undo point)
	_ = safecopy.Clean(path) // remove any remaining intermediate
	if err := os.Rename(origBak, path+".bak"); err != nil {
		return nil, fmt.Errorf("finalize backup: %w", err)
	}

	return result, nil
}

// restoreOriginal restores from .bak.orig on failure.
func restoreOriginal(path, origBak string) error {
	_ = safecopy.Clean(path)
	return os.Rename(origBak, path)
}

// copyFileForCleanAll copies src to dst for backup purposes.
func copyFileForCleanAll(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
