package editor

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// ContentSurgeryResult holds the combined results of merged content surgery.
type ContentSurgeryResult struct {
	FailedRetries     int
	StaleReadsRemoved int
	EntriesRemoved    int
	BlocksRemoved     int
	ChainRepairs      int
}

// ContentSurgery performs block-level removal for both failed retries and stale reads
// in a single ParseRaw pass. This replaces separate RemoveFailedRetries + DeduplicateReads
// calls in CleanAll, halving the I/O for phases 1e+1f.
func ContentSurgery(path string, retryResult *analyzer.RetryResult, dupResult *analyzer.DuplicateReadResult) (*ContentSurgeryResult, error) {
	hasRetries := retryResult != nil && len(retryResult.Sequences) > 0
	hasDedup := dupResult != nil && len(dupResult.Groups) > 0

	if !hasRetries && !hasDedup {
		return &ContentSurgeryResult{}, nil
	}

	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	// Build combined removal maps
	assistantRemovals := make(map[int][]string)
	userRemovals := make(map[int][]string)

	// From failed retries
	if hasRetries {
		for _, s := range retryResult.Sequences {
			assistantRemovals[s.FailedToolUseIdx] = append(assistantRemovals[s.FailedToolUseIdx], s.FailedToolUseID)
			if s.FailedResultIdx >= 0 {
				userRemovals[s.FailedResultIdx] = append(userRemovals[s.FailedResultIdx], s.FailedToolUseID)
			}
		}
	}

	// From stale reads
	if hasDedup {
		for _, g := range dupResult.Groups {
			for _, sr := range g.StaleReads {
				assistantRemovals[sr.AssistantIdx] = append(assistantRemovals[sr.AssistantIdx], sr.ToolUseID)
				if sr.ResultIdx >= 0 {
					userRemovals[sr.ResultIdx] = append(userRemovals[sr.ResultIdx], sr.ToolUseID)
				}
			}
		}
	}

	result := &ContentSurgeryResult{}
	entriesToDelete := make(map[int]bool)

	// Remove tool_use blocks from assistant entries
	for idx, toolIDs := range assistantRemovals {
		if idx >= len(entries) || entries[idx].Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(entries[idx].Message.Content)
		if err != nil {
			continue
		}

		removeSet := make(map[string]bool)
		for _, id := range toolIDs {
			removeSet[id] = true
		}

		var kept []jsonl.ContentBlock
		removed := 0
		for _, b := range blocks {
			if b.Type == "tool_use" && removeSet[b.ID] {
				removed++
				continue
			}
			kept = append(kept, b)
		}

		if removed == 0 {
			continue
		}

		result.BlocksRemoved += removed

		if len(kept) == 0 {
			entriesToDelete[idx] = true
		} else {
			updated, err := reserializeContent(rawLines[idx], kept)
			if err != nil {
				continue
			}
			rawLines[idx] = updated
		}
	}

	// Remove tool_result blocks from user entries
	for idx, toolIDs := range userRemovals {
		if idx >= len(entries) || entries[idx].Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(entries[idx].Message.Content)
		if err != nil {
			continue
		}

		removeSet := make(map[string]bool)
		for _, id := range toolIDs {
			removeSet[id] = true
		}

		var kept []jsonl.ContentBlock
		removed := 0
		for _, b := range blocks {
			if b.Type == "tool_result" && removeSet[b.ToolUseID] {
				removed++
				continue
			}
			kept = append(kept, b)
		}

		if removed == 0 {
			continue
		}

		result.BlocksRemoved += removed

		if len(kept) == 0 {
			entriesToDelete[idx] = true
		} else {
			updated, err := reserializeContent(rawLines[idx], kept)
			if err != nil {
				continue
			}
			rawLines[idx] = updated
		}
	}

	// Delete emptied entries and repair chains
	if len(entriesToDelete) > 0 {
		deleteUUIDs := make(map[string]bool)
		parentRemap := make(map[string]string)
		for idx := range entriesToDelete {
			if idx < len(entries) && entries[idx].UUID != "" {
				deleteUUIDs[entries[idx].UUID] = true
				parentRemap[entries[idx].UUID] = entries[idx].ParentUUID
			}
		}

		var newLines [][]byte
		for i, e := range entries {
			if entriesToDelete[i] {
				result.EntriesRemoved++
				continue
			}

			if deleteUUIDs[e.ParentUUID] {
				newParent := resolveParent(e.ParentUUID, parentRemap)
				if newParent != e.ParentUUID {
					repaired, err := reparentEntry(rawLines[i], newParent)
					if err == nil {
						newLines = append(newLines, repaired)
						result.ChainRepairs++
						continue
					}
				}
			}

			newLines = append(newLines, rawLines[i])
		}
		rawLines = newLines
	}

	if result.BlocksRemoved == 0 && result.EntriesRemoved == 0 {
		return result, nil
	}

	if err := safecopy.CreateIfMissing(path); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}

	if err := jsonl.WriteLines(path, rawLines); err != nil {
		_ = safecopy.Restore(path)
		return nil, fmt.Errorf("write: %w", err)
	}

	// Count retries vs stale reads from the removal maps
	if hasRetries {
		for _, s := range retryResult.Sequences {
			result.FailedRetries++
			_ = s // counted per sequence
		}
	}
	if hasDedup {
		for _, g := range dupResult.Groups {
			result.StaleReadsRemoved += len(g.StaleReads)
		}
	}

	return result, nil
}
