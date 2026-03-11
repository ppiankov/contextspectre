package editor

import (
	"encoding/json"
	"fmt"

	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// CoalesceResult holds the result of a coalesce operation.
type CoalesceResult struct {
	GroupsMerged    int
	EntriesRemoved  int
	OrphansStripped int
	BytesBefore     int64
	BytesAfter      int64
}

// Coalesce merges adjacent same-role entries into single entries,
// combining their content blocks. This fixes API errors where tool_result
// blocks don't match the immediately preceding assistant's tool_use blocks
// (common in Claude for Mac sessions that split multi-tool calls).
//
// Rules:
//   - Only merges entries with same message.role (user+user, assistant+assistant)
//   - Non-message entries (system, queue-operation, progress) break merge groups
//   - Content blocks from later entries are appended to the first entry
//   - First entry's uuid/parentUuid/timestamp are preserved
//   - Next entry after a merged group gets reparented to the group's uuid
func Coalesce(path string) (*CoalesceResult, error) {
	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	if len(entries) == 0 {
		return &CoalesceResult{}, nil
	}

	// Create backup
	if err := safecopy.CreateIfMissing(path); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}

	result := &CoalesceResult{}
	for _, raw := range rawLines {
		result.BytesBefore += int64(len(raw))
	}

	// Build merged output lines.
	var outLines [][]byte
	// Track which indices are "consumed" into a group.
	consumed := make([]bool, len(entries))

	i := 0
	for i < len(entries) {
		role := entryRole(entries[i])

		// Non-mergeable entries pass through.
		if role == "" {
			outLines = append(outLines, rawLines[i])
			i++
			continue
		}

		// Find the end of a same-role group, treating system/queue-operation
		// entries as transparent (they don't break the group).
		groupStart := i
		var mergeIndices []int // indices of same-role entries to merge
		var floaters []int     // indices of non-message entries within the group
		mergeIndices = append(mergeIndices, i)
		j := i + 1
		for j < len(entries) {
			jr := entryRole(entries[j])
			if jr == role {
				mergeIndices = append(mergeIndices, j)
				j++
			} else if jr == "" && isTransparent(entries[j]) {
				floaters = append(floaters, j)
				j++
			} else {
				break
			}
		}
		groupEnd := j

		if len(mergeIndices) == 1 {
			// Single entry + possible floaters — emit in order.
			for k := groupStart; k < groupEnd; k++ {
				outLines = append(outLines, rawLines[k])
			}
			i = groupEnd
			continue
		}

		// Merge group: emit floaters first, then the merged entry.
		var mergeRaws [][]byte
		for _, idx := range mergeIndices {
			mergeRaws = append(mergeRaws, rawLines[idx])
		}
		merged, err := mergeGroup(mergeRaws)
		if err != nil {
			// On merge failure, keep entries as-is.
			for k := groupStart; k < groupEnd; k++ {
				outLines = append(outLines, rawLines[k])
			}
			i = groupEnd
			continue
		}

		// Emit floaters before the merged entry.
		for _, idx := range floaters {
			outLines = append(outLines, rawLines[idx])
		}
		outLines = append(outLines, merged)
		for _, idx := range mergeIndices[1:] {
			consumed[idx] = true
		}
		result.GroupsMerged++
		result.EntriesRemoved += len(mergeIndices) - 1

		// Reparent: the next entry after this group should point to
		// the merged entry's uuid (which is the first entry's uuid).
		if groupEnd < len(entries) && entries[mergeIndices[0]].UUID != "" {
			reparented, err := setParentUUID(rawLines[groupEnd], entries[mergeIndices[0]].UUID)
			if err == nil {
				rawLines[groupEnd] = reparented
			}
		}

		i = groupEnd
	}

	// Second pass: strip orphaned tool_result blocks.
	// After merging, a user entry may contain tool_result blocks whose
	// matching tool_use was deleted. Strip those blocks to prevent API errors.
	outLines, result.OrphansStripped = stripOrphanedToolResults(outLines)

	for _, raw := range outLines {
		result.BytesAfter += int64(len(raw))
	}

	if err := jsonl.WriteLines(path, outLines); err != nil {
		_ = safecopy.Restore(path)
		return nil, fmt.Errorf("write: %w", err)
	}

	return result, nil
}

// isTransparent returns true for entries that should not break a merge group.
// System and queue-operation entries are structural metadata, not conversation turns.
func isTransparent(e jsonl.Entry) bool {
	return e.Type == jsonl.TypeSystem || e.Type == jsonl.TypeQueueOperation
}

// entryRole returns the message role for mergeable entry types, or "" for non-mergeable.
func entryRole(e jsonl.Entry) string {
	if e.Type != jsonl.TypeUser && e.Type != jsonl.TypeAssistant {
		return ""
	}
	if e.Message == nil {
		return ""
	}
	return e.Message.Role
}

// mergeGroup combines content arrays from multiple raw JSON entries into one.
// The first entry is used as the base; content blocks from subsequent entries
// are appended to its content array.
func mergeGroup(rawLines [][]byte) ([]byte, error) {
	if len(rawLines) == 0 {
		return nil, fmt.Errorf("empty group")
	}
	if len(rawLines) == 1 {
		return rawLines[0], nil
	}

	// Parse the first entry as a generic map to preserve all fields.
	var base map[string]json.RawMessage
	if err := json.Unmarshal(rawLines[0], &base); err != nil {
		return nil, fmt.Errorf("unmarshal base: %w", err)
	}

	// Extract base message.
	msgRaw, ok := base["message"]
	if !ok {
		return rawLines[0], nil
	}

	var baseMsg map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &baseMsg); err != nil {
		return nil, fmt.Errorf("unmarshal base message: %w", err)
	}

	// Parse base content as array of raw blocks.
	var baseContent []json.RawMessage
	if contentRaw, ok := baseMsg["content"]; ok {
		if err := json.Unmarshal(contentRaw, &baseContent); err != nil {
			// Content might be a string, not an array — skip merge.
			return rawLines[0], nil
		}
	}

	// Append content blocks from subsequent entries.
	for _, raw := range rawLines[1:] {
		blocks, err := extractContentBlocks(raw)
		if err != nil {
			continue
		}
		baseContent = append(baseContent, blocks...)
	}

	// Reassemble.
	contentJSON, err := json.Marshal(baseContent)
	if err != nil {
		return nil, fmt.Errorf("marshal content: %w", err)
	}
	baseMsg["content"] = contentJSON

	msgJSON, err := json.Marshal(baseMsg)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}
	base["message"] = msgJSON

	return json.Marshal(base)
}

// extractContentBlocks pulls content blocks from a raw JSONL entry.
func extractContentBlocks(raw []byte) ([]json.RawMessage, error) {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, err
	}

	msgRaw, ok := entry["message"]
	if !ok {
		return nil, fmt.Errorf("no message")
	}

	var msg map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return nil, err
	}

	contentRaw, ok := msg["content"]
	if !ok {
		return nil, fmt.Errorf("no content")
	}

	var blocks []json.RawMessage
	if err := json.Unmarshal(contentRaw, &blocks); err != nil {
		return nil, err
	}

	return blocks, nil
}

// stripOrphanedToolResults scans the output lines for user entries whose
// tool_result blocks don't have a matching tool_use in the preceding assistant.
// Orphaned tool_result blocks are removed; the entry is kept if other blocks remain.
func stripOrphanedToolResults(lines [][]byte) ([][]byte, int) {
	// First pass: collect tool_use IDs from each assistant entry by line index.
	type blockInfo struct {
		Type      string `json:"type"`
		ID        string `json:"id,omitempty"`
		ToolUseID string `json:"tool_use_id,omitempty"`
	}

	stripped := 0
	prevAssistantToolUses := map[string]bool{}

	for i := 0; i < len(lines); i++ {
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(lines[i], &entry); err != nil {
			continue
		}

		msgRaw, ok := entry["message"]
		if !ok {
			continue
		}

		var msg map[string]json.RawMessage
		if err := json.Unmarshal(msgRaw, &msg); err != nil {
			continue
		}

		var role string
		if err := json.Unmarshal(msg["role"], &role); err != nil {
			continue
		}

		contentRaw, ok := msg["content"]
		if !ok {
			continue
		}

		var blocks []json.RawMessage
		if err := json.Unmarshal(contentRaw, &blocks); err != nil {
			continue
		}

		if role == "assistant" {
			prevAssistantToolUses = map[string]bool{}
			for _, b := range blocks {
				var bi blockInfo
				if err := json.Unmarshal(b, &bi); err == nil && bi.Type == "tool_use" && bi.ID != "" {
					prevAssistantToolUses[bi.ID] = true
				}
			}
			continue
		}

		if role != "user" {
			continue
		}

		// Check if any tool_result blocks are orphaned.
		hasOrphan := false
		for _, b := range blocks {
			var bi blockInfo
			if err := json.Unmarshal(b, &bi); err == nil && bi.Type == "tool_result" {
				if !prevAssistantToolUses[bi.ToolUseID] {
					hasOrphan = true
					break
				}
			}
		}

		if !hasOrphan {
			prevAssistantToolUses = map[string]bool{}
			continue
		}

		// Filter out orphaned tool_result blocks.
		var kept []json.RawMessage
		for _, b := range blocks {
			var bi blockInfo
			if err := json.Unmarshal(b, &bi); err == nil && bi.Type == "tool_result" {
				if !prevAssistantToolUses[bi.ToolUseID] {
					stripped++
					continue
				}
			}
			kept = append(kept, b)
		}

		if len(kept) == 0 {
			// All blocks were orphaned — keep a minimal text placeholder.
			placeholder, _ := json.Marshal([]map[string]string{{"type": "text", "text": "[coalesced]"}})
			msg["content"] = placeholder
		} else {
			newContent, _ := json.Marshal(kept)
			msg["content"] = newContent
		}

		newMsg, _ := json.Marshal(msg)
		entry["message"] = newMsg
		updated, err := json.Marshal(entry)
		if err == nil {
			lines[i] = updated
		}

		prevAssistantToolUses = map[string]bool{}
	}

	return lines, stripped
}

// setParentUUID returns a new raw JSON line with parentUuid set to the given value.
func setParentUUID(raw []byte, parentUUID string) ([]byte, error) {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, err
	}
	parentJSON, err := json.Marshal(parentUUID)
	if err != nil {
		return nil, err
	}
	entry["parentUuid"] = parentJSON
	return json.Marshal(entry)
}
