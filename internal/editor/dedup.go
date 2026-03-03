package editor

import (
	"encoding/json"
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// DedupResult holds the result of a duplicate read deduplication operation.
type DedupResult struct {
	StaleReadsRemoved int
	EntriesRemoved    int
	BlocksRemoved     int
	ChainRepairs      int
	BytesBefore       int64
	BytesAfter        int64
}

// DeduplicateReads removes stale file read tool_use/tool_result pairs.
// For each stale read:
//   - If assistant message has only that tool_use → delete entire entry
//   - If assistant message has other blocks → remove just that tool_use block
//   - Remove the corresponding tool_result block from user message
//   - If user message only has that tool_result → delete entire entry
//
// Always creates a backup before modifying.
func DeduplicateReads(path string, dupResult *analyzer.DuplicateReadResult) (*DedupResult, error) {
	if dupResult == nil || len(dupResult.Groups) == 0 {
		return &DedupResult{}, nil
	}

	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	// Build sets of stale tool_use IDs per assistant entry, and per user entry
	// assistantRemovals[entryIdx] = list of tool_use IDs to remove
	assistantRemovals := make(map[int][]string)
	// userRemovals[entryIdx] = list of tool_use_ids whose tool_results should be removed
	userRemovals := make(map[int][]string)

	for _, g := range dupResult.Groups {
		for _, sr := range g.StaleReads {
			assistantRemovals[sr.AssistantIdx] = append(assistantRemovals[sr.AssistantIdx], sr.ToolUseID)
			if sr.ResultIdx >= 0 {
				userRemovals[sr.ResultIdx] = append(userRemovals[sr.ResultIdx], sr.ToolUseID)
			}
		}
	}

	result := &DedupResult{}
	for _, raw := range rawLines {
		result.BytesBefore += int64(len(raw))
	}

	// Process content surgery: modify entries in place
	entriesToDelete := make(map[int]bool)

	// Handle assistant entries: remove stale tool_use blocks
	for idx, toolIDs := range assistantRemovals {
		if idx >= len(entries) {
			continue
		}
		e := entries[idx]
		if e.Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		removeSet := make(map[string]bool)
		for _, id := range toolIDs {
			removeSet[id] = true
		}

		// Filter out stale tool_use blocks
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
		result.StaleReadsRemoved += removed

		if len(kept) == 0 {
			// Entire entry should be deleted
			entriesToDelete[idx] = true
		} else {
			// Content surgery: reserialize with remaining blocks
			updated, err := reserializeContent(rawLines[idx], kept)
			if err != nil {
				continue
			}
			rawLines[idx] = updated
		}
	}

	// Handle user entries: remove stale tool_result blocks
	for idx, toolIDs := range userRemovals {
		if idx >= len(entries) {
			continue
		}
		e := entries[idx]
		if e.Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
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
		// Build parent remap for chain repair
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

			// Repair parentUuid chain
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

	// Only write if something changed
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

// reparentEntry updates the parentUuid field in a raw JSONL line.
func reparentEntry(rawLine []byte, newParent string) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rawLine, &raw); err != nil {
		return nil, err
	}
	newParentJSON, err := json.Marshal(newParent)
	if err != nil {
		return nil, err
	}
	raw["parentUuid"] = newParentJSON
	return json.Marshal(raw)
}
