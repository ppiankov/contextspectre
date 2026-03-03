package editor

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// RetryCleanResult holds the result of a failed retry cleanup operation.
type RetryCleanResult struct {
	FailedRemoved  int
	EntriesRemoved int
	BlocksRemoved  int
	ChainRepairs   int
	BytesBefore    int64
	BytesAfter     int64
}

// RemoveFailedRetries removes failed tool attempts that were superseded by retries.
// For each failed sequence:
//   - Remove the tool_use block from the assistant message (or whole entry if only block)
//   - Remove the error tool_result block from the user message (or whole entry)
//
// Always creates a backup before modifying.
func RemoveFailedRetries(path string, retryResult *analyzer.RetryResult) (*RetryCleanResult, error) {
	if retryResult == nil || len(retryResult.Sequences) == 0 {
		return &RetryCleanResult{}, nil
	}

	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	// Build removal sets: assistant entries → tool_use IDs, user entries → tool_use IDs
	assistantRemovals := make(map[int][]string)
	userRemovals := make(map[int][]string)

	for _, s := range retryResult.Sequences {
		assistantRemovals[s.FailedToolUseIdx] = append(assistantRemovals[s.FailedToolUseIdx], s.FailedToolUseID)
		if s.FailedResultIdx >= 0 {
			userRemovals[s.FailedResultIdx] = append(userRemovals[s.FailedResultIdx], s.FailedToolUseID)
		}
	}

	result := &RetryCleanResult{}
	for _, raw := range rawLines {
		result.BytesBefore += int64(len(raw))
	}

	entriesToDelete := make(map[int]bool)

	// Remove failed tool_use blocks from assistant entries
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
		result.FailedRemoved += removed

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

	// Remove error tool_result blocks from user entries
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

	// Delete full entries and repair chains
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

	for _, raw := range rawLines {
		result.BytesAfter += int64(len(raw))
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

	return result, nil
}
