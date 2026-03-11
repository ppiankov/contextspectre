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
	OrphansRemoved     int
	OrphansTombstoned  int
	ImagesReplaced     int
	SeparatorsStripped int
	OutputsTruncated   int
	CoalesceMerged     int
	CoalesceOrphans    int
	TotalTokensSaved   int
	KeepSkipped        int
	BytesBefore        int64
	BytesAfter         int64
}

// CleanAllOpts configures a CleanAll run.
type CleanAllOpts struct {
	Tombstone bool // replace orphans with placeholders instead of deleting
}

// CleanAll runs all cleanup operations in optimal order with a single backup.
// Order: entry deletions first, then content surgery.
func CleanAll(path string, opts CleanAllOpts) (*CleanAllResult, error) {
	result := &CleanAllResult{}

	// Load markers to respect KEEP entries
	markers, _ := LoadMarkers(path)

	// Get original size
	origInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	result.BytesBefore = origInfo.Size()

	// Clean stale .bak from prior operations before starting
	_ = safecopy.Clean(path)

	// Create original backup as .bak.orig (the single undo point)
	origBak := path + ".bak.orig"
	if err := copyFileForCleanAll(path, origBak); err != nil {
		return nil, fmt.Errorf("create original backup: %w", err)
	}

	// Helper: clean intermediate .bak files between operations
	cleanIntermediate := func() {
		_ = safecopy.Clean(path)
	}

	// Helper: check if entry is protected by KEEP marker
	isKept := func(uuid string) bool {
		return markers.IsKeep(uuid)
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
		if e.Type == jsonl.TypeProgress && !isKept(e.UUID) {
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
		if e.Type == jsonl.TypeFileHistorySnapshot && !isKept(e.UUID) {
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
	sidechains := analyzer.DetectSidechains(entries)
	toDelete = analyzer.SidechainIndexSet(sidechains)
	for idx := range toDelete {
		if idx < len(entries) && isKept(entries[idx].UUID) {
			delete(toDelete, idx)
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
		for idx := range toDelete {
			if idx < len(entries) && isKept(entries[idx].UUID) {
				delete(toDelete, idx)
			}
		}
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
		// Filter out sequences where either entry is KEEP-marked
		var filteredSeqs []analyzer.RetrySequence
		for _, seq := range retryResult.Sequences {
			if seq.FailedToolUseIdx < len(entries) && isKept(entries[seq.FailedToolUseIdx].UUID) {
				continue
			}
			if seq.FailedResultIdx >= 0 && seq.FailedResultIdx < len(entries) && isKept(entries[seq.FailedResultIdx].UUID) {
				continue
			}
			filteredSeqs = append(filteredSeqs, seq)
		}
		retryResult.Sequences = filteredSeqs
		if len(retryResult.Sequences) > 0 {
			rr, err := RemoveFailedRetries(path, retryResult)
			if err != nil {
				_ = restoreOriginal(path, origBak)
				return nil, fmt.Errorf("retries: %w", err)
			}
			result.FailedRetries = rr.FailedRemoved
			cleanIntermediate()
		}
	}

	// 1f. Remove stale reads
	entries, _ = jsonl.Parse(path)
	dupResult := analyzer.FindDuplicateReads(entries)
	if len(dupResult.Groups) > 0 {
		// Filter out stale reads where either entry is KEEP-marked
		var filteredGroups []analyzer.DuplicateGroup
		for _, g := range dupResult.Groups {
			var filteredReads []analyzer.StaleRead
			for _, sr := range g.StaleReads {
				if sr.AssistantIdx < len(entries) && isKept(entries[sr.AssistantIdx].UUID) {
					continue
				}
				if sr.ResultIdx >= 0 && sr.ResultIdx < len(entries) && isKept(entries[sr.ResultIdx].UUID) {
					continue
				}
				filteredReads = append(filteredReads, sr)
			}
			if len(filteredReads) > 0 {
				g.StaleReads = filteredReads
				filteredGroups = append(filteredGroups, g)
			}
		}
		dupResult.Groups = filteredGroups
		if len(dupResult.Groups) > 0 {
			dr, err := DeduplicateReads(path, dupResult)
			if err != nil {
				_ = restoreOriginal(path, origBak)
				return nil, fmt.Errorf("dedup: %w", err)
			}
			result.StaleReadsRemoved = dr.StaleReadsRemoved
			cleanIntermediate()
		}
	}

	// 1g. Orphan cascade — resolve orphaned tool_results and chain breaks
	// created by prior deletions (especially tangent removal in 1d).
	// Pre-computes the full transitive closure in memory via BFS graph
	// traversal, then executes a single Delete() call.
	entries, err = jsonl.Parse(path)
	if err != nil {
		_ = restoreOriginal(path, origBak)
		return nil, fmt.Errorf("cascade parse: %w", err)
	}
	diagnosis := analyzer.Diagnose(entries)
	toDelete = make(map[int]bool)
	toTombstone := make(map[int]bool)
	for _, issue := range diagnosis.Issues {
		switch issue.Kind {
		case analyzer.IssueOrphanedResult:
			if issue.EntryIndex < len(entries) && !isKept(entries[issue.EntryIndex].UUID) {
				if opts.Tombstone {
					toTombstone[issue.EntryIndex] = true
				} else {
					toDelete[issue.EntryIndex] = true
				}
			}
		case analyzer.IssueChainBroken:
			if issue.EntryIndex < len(entries) && !isKept(entries[issue.EntryIndex].UUID) {
				toDelete[issue.EntryIndex] = true
			}
		}
	}
	// Expand delete set to full transitive closure (orphan + chain cascades).
	toDelete = analyzer.CascadeDeleteSet(entries, toDelete, isKept)
	if len(toTombstone) > 0 {
		tsResult, err := Tombstone(path, toTombstone)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("cascade tombstone: %w", err)
		}
		result.OrphansTombstoned += tsResult.EntriesTombstoned
		cleanIntermediate()
	}
	if len(toDelete) > 0 {
		dr, err := Delete(path, toDelete)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("cascade: %w", err)
		}
		result.OrphansRemoved += dr.EntriesRemoved
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

	// 2d. Coalesce adjacent same-role entries (fixes Mac session API errors).
	cr, err := Coalesce(path)
	if err != nil {
		_ = restoreOriginal(path, origBak)
		return nil, fmt.Errorf("coalesce: %w", err)
	}
	result.CoalesceMerged = cr.EntriesRemoved
	result.CoalesceOrphans = cr.OrphansStripped
	if cr.EntriesRemoved > 0 {
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
