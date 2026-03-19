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
	// Single parse for phases 1a-1d + 1g (whole-entry deletions).
	// Phases 1e-1f need separate passes (content surgery within entries).

	entries, err := jsonl.Parse(path)
	if err != nil {
		_ = restoreOriginal(path, origBak)
		return nil, fmt.Errorf("parse: %w", err)
	}

	// 1a-1d: Build merged delete set (no I/O)
	toDelete := make(map[int]bool)

	// 1a. Progress entries
	for i, e := range entries {
		if e.Type == jsonl.TypeProgress && !isKept(e.UUID) {
			toDelete[i] = true
			result.ProgressRemoved++
		}
	}

	// 1b. Snapshot entries
	for i, e := range entries {
		if e.Type == jsonl.TypeFileHistorySnapshot && !isKept(e.UUID) {
			toDelete[i] = true
			result.SnapshotsRemoved++
		}
	}

	// 1c. Sidechain entries
	sidechains := analyzer.DetectSidechains(entries)
	for idx := range analyzer.SidechainIndexSet(sidechains) {
		if idx < len(entries) && !isKept(entries[idx].UUID) {
			toDelete[idx] = true
			result.SidechainsRemoved++
		}
	}

	// 1d. Cross-repo tangents
	tangentResult := analyzer.FindTangents(entries)
	if len(tangentResult.Groups) > 0 {
		for idx := range tangentResult.AllTangentIndices() {
			if idx < len(entries) && !isKept(entries[idx].UUID) {
				toDelete[idx] = true
				result.TangentsRemoved++
			}
		}
	}

	// 1g. Orphan cascade — diagnose excluding 1a-1d delete set so orphans
	// created by those deletions are detected in this same pass.
	diagnosis := analyzer.DiagnoseExcluding(entries, toDelete)
	toTombstone := make(map[int]bool)
	cascadeInitial := make(map[int]bool)
	for _, issue := range diagnosis.Issues {
		idx := issue.EntryIndex
		if idx >= len(entries) || isKept(entries[idx].UUID) || toDelete[idx] {
			continue
		}
		switch issue.Kind {
		case analyzer.IssueOrphanedResult:
			if opts.Tombstone {
				toTombstone[idx] = true
			} else {
				cascadeInitial[idx] = true
			}
		case analyzer.IssueChainBroken:
			cascadeInitial[idx] = true
		}
	}
	// Expand cascade from orphan/chain-break initial set
	cascadeSet := analyzer.CascadeDeleteSet(entries, cascadeInitial, isKept)
	for idx := range cascadeSet {
		if !toDelete[idx] {
			toDelete[idx] = true
			result.OrphansRemoved++
		}
	}

	// Execute: tombstone first (in-place, preserves UUIDs), then single delete
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
		_, err := Delete(path, toDelete)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("delete: %w", err)
		}
		cleanIntermediate()
	}

	// 1e+1f. Content surgery: retries + stale reads in single ParseRaw pass
	entries, _ = jsonl.Parse(path)
	retryResult := analyzer.FindFailedRetries(entries)
	if len(retryResult.Sequences) > 0 {
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
	}

	dupResult := analyzer.FindDuplicateReads(entries)
	if len(dupResult.Groups) > 0 {
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
	}

	if len(retryResult.Sequences) > 0 || len(dupResult.Groups) > 0 {
		sr, err := ContentSurgery(path, retryResult, dupResult)
		if err != nil {
			_ = restoreOriginal(path, origBak)
			return nil, fmt.Errorf("surgery: %w", err)
		}
		result.FailedRetries = sr.FailedRetries
		result.StaleReadsRemoved = sr.StaleReadsRemoved
		if sr.BlocksRemoved > 0 || sr.EntriesRemoved > 0 {
			cleanIntermediate()
		}
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
	return renameOrCopy(origBak, path)
}

// copyFileForCleanAll copies src to dst for backup purposes.
func copyFileForCleanAll(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
