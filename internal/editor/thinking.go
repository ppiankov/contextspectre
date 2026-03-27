package editor

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// CleanThinkingResult holds the result of thinking block cleanup.
type CleanThinkingResult struct {
	ThinkingRemoved   int
	SignaturesRemoved int
	BytesBefore       int64
	BytesAfter        int64
}

// CleanThinking removes thinking blocks and signature fields from assistant entries.
// These are extended-thinking artifacts that are never re-sent to the API.
func CleanThinking(path string) (*CleanThinkingResult, error) {
	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	result := &CleanThinkingResult{}
	for _, raw := range rawLines {
		result.BytesBefore += int64(len(raw))
	}

	changed := false
	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil || len(blocks) == 0 {
			continue
		}

		var kept []jsonl.ContentBlock
		modified := false
		for _, b := range blocks {
			if b.Type == "thinking" {
				result.ThinkingRemoved++
				modified = true
				continue
			}
			// Strip signature field from any block that has one.
			if hasSignature(rawLines[i], b) {
				result.SignaturesRemoved++
				modified = true
			}
			kept = append(kept, b)
		}
		if !modified {
			continue
		}
		if len(kept) == 0 {
			// Don't empty the content — keep a placeholder text block.
			kept = []jsonl.ContentBlock{{Type: "text", Text: "[thinking removed]"}}
		}
		updated, err := reserializeContent(rawLines[i], kept)
		if err != nil {
			continue
		}
		rawLines[i] = updated
		changed = true
	}

	if !changed {
		return result, nil
	}

	if err := safecopy.CreateIfMissing(path); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	if err := jsonl.WriteLines(path, rawLines); err != nil {
		_ = safecopy.Restore(path)
		return nil, fmt.Errorf("write: %w", err)
	}

	for _, raw := range rawLines {
		result.BytesAfter += int64(len(raw))
	}
	return result, nil
}

// hasSignature checks if a content block's raw JSON contains a "signature" field.
// We check the raw line because ContentBlock doesn't model the signature field.
func hasSignature(rawLine []byte, _ jsonl.ContentBlock) bool {
	// Quick byte scan — signature fields appear as "signature": in the JSON.
	// This is a heuristic but avoids full re-parse of every block.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rawLine, &raw); err != nil {
		return false
	}
	msgRaw, ok := raw["message"]
	if !ok {
		return false
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return false
	}
	contentRaw, ok := msg["content"]
	if !ok {
		return false
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(contentRaw, &blocks); err != nil {
		return false
	}
	for _, b := range blocks {
		if _, has := b["signature"]; has {
			return true
		}
	}
	return false
}

// CleanSystemRemindersResult holds the result of system reminder dedup.
type CleanSystemRemindersResult struct {
	RemindersRemoved int
	BytesBefore      int64
	BytesAfter       int64
}

// CleanSystemReminders deduplicates repeated system-reminder blocks across user messages.
// Keeps the last occurrence of each unique reminder, removes earlier duplicates.
func CleanSystemReminders(path string) (*CleanSystemRemindersResult, error) {
	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	result := &CleanSystemRemindersResult{}
	for _, raw := range rawLines {
		result.BytesBefore += int64(len(raw))
	}

	// Pass 1: Find all system-reminder blocks and their last occurrence.
	// A system-reminder is a text block containing <system-reminder>.
	type reminderLoc struct {
		entryIdx int
		blockIdx int
	}
	lastSeen := make(map[string]reminderLoc)  // hash → last location
	allLocs := make(map[string][]reminderLoc) // hash → all locations

	for i, e := range entries {
		if e.Type != jsonl.TypeUser || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for j, b := range blocks {
			if b.Type != "text" || len(b.Text) < 20 {
				continue
			}
			if !isSystemReminder(b.Text) {
				continue
			}
			h := hashText(b.Text)
			loc := reminderLoc{entryIdx: i, blockIdx: j}
			lastSeen[h] = loc
			allLocs[h] = append(allLocs[h], loc)
		}
	}

	// Pass 2: Mark blocks to remove (all except last occurrence).
	// Key: entryIdx → set of blockIdx to remove.
	toRemove := make(map[int]map[int]bool)
	for h, locs := range allLocs {
		if len(locs) < 2 {
			continue
		}
		last := lastSeen[h]
		for _, loc := range locs {
			if loc == last {
				continue
			}
			if toRemove[loc.entryIdx] == nil {
				toRemove[loc.entryIdx] = make(map[int]bool)
			}
			toRemove[loc.entryIdx][loc.blockIdx] = true
			result.RemindersRemoved++
		}
	}

	if result.RemindersRemoved == 0 {
		return result, nil
	}

	// Pass 3: Rewrite affected entries.
	changed := false
	for idx, blockSet := range toRemove {
		blocks, err := jsonl.ParseContentBlocks(entries[idx].Message.Content)
		if err != nil {
			continue
		}
		var kept []jsonl.ContentBlock
		for j, b := range blocks {
			if blockSet[j] {
				continue
			}
			kept = append(kept, b)
		}
		if len(kept) == 0 {
			kept = []jsonl.ContentBlock{{Type: "text", Text: ""}}
		}
		updated, err := reserializeContent(rawLines[idx], kept)
		if err != nil {
			continue
		}
		rawLines[idx] = updated
		changed = true
	}

	if !changed {
		return result, nil
	}

	if err := safecopy.CreateIfMissing(path); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	if err := jsonl.WriteLines(path, rawLines); err != nil {
		_ = safecopy.Restore(path)
		return nil, fmt.Errorf("write: %w", err)
	}

	for _, raw := range rawLines {
		result.BytesAfter += int64(len(raw))
	}
	return result, nil
}

// CleanMegaBlocksResult holds the result of mega block truncation.
type CleanMegaBlocksResult struct {
	BlocksTruncated int
	BytesBefore     int64
	BytesAfter      int64
}

// MegaBlockThreshold is the size in bytes above which a content block is truncated.
const MegaBlockThreshold = 100 * 1024 // 100KB

// CleanMegaBlocks truncates individual content blocks exceeding MegaBlockThreshold.
func CleanMegaBlocks(path string) (*CleanMegaBlocksResult, error) {
	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	result := &CleanMegaBlocksResult{}
	for _, raw := range rawLines {
		result.BytesBefore += int64(len(raw))
	}

	changed := false
	for i, e := range entries {
		if e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil || len(blocks) == 0 {
			continue
		}

		modified := false
		for j, b := range blocks {
			blockSize := blockSerializedSize(b)
			if blockSize <= MegaBlockThreshold {
				continue
			}

			// Truncate the text/content field.
			if b.Type == "text" && len(b.Text) > MegaBlockThreshold {
				blocks[j].Text = b.Text[:MegaBlockThreshold] + "\n\n[truncated by contextspectre — original " + fmt.Sprintf("%dKB", blockSize/1024) + "]"
				modified = true
				result.BlocksTruncated++
			} else if b.Type == "tool_result" && len(b.Content) > MegaBlockThreshold {
				truncated := b.Content[:MegaBlockThreshold]
				// Find a safe cut point (don't break mid-JSON).
				suffix := fmt.Sprintf(`"[truncated by contextspectre — original %dKB]"`, blockSize/1024)
				blocks[j].Content = json.RawMessage(truncated)
				// If content is a string, truncate the string.
				var s string
				if json.Unmarshal(b.Content, &s) == nil && len(s) > MegaBlockThreshold {
					truncatedStr := s[:MegaBlockThreshold] + fmt.Sprintf("\n\n[truncated by contextspectre — original %dKB]", blockSize/1024)
					blocks[j].Content, _ = json.Marshal(truncatedStr)
					modified = true
					result.BlocksTruncated++
				} else {
					// Non-string content — replace with truncation marker.
					blocks[j].Content = json.RawMessage(suffix)
					modified = true
					result.BlocksTruncated++
				}
			}
		}

		if !modified {
			continue
		}
		updated, err := reserializeContent(rawLines[i], blocks)
		if err != nil {
			continue
		}
		rawLines[i] = updated
		changed = true
	}

	if !changed {
		return result, nil
	}

	if err := safecopy.CreateIfMissing(path); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	if err := jsonl.WriteLines(path, rawLines); err != nil {
		_ = safecopy.Restore(path)
		return nil, fmt.Errorf("write: %w", err)
	}

	for _, raw := range rawLines {
		result.BytesAfter += int64(len(raw))
	}
	return result, nil
}

func isSystemReminder(text string) bool {
	return len(text) > 17 && (text[:17] == "<system-reminder>" || contains(text, "<system-reminder>"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h[:8])
}

func blockSerializedSize(b jsonl.ContentBlock) int {
	data, err := json.Marshal(b)
	if err != nil {
		return 0
	}
	return len(data)
}
