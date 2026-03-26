package editor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// RewireResult holds the result of a rewire operation.
type RewireResult struct {
	Injected    int
	BytesBefore int64
	BytesAfter  int64
}

// OrphanedToolUse describes a tool_use block without a matching tool_result.
type OrphanedToolUse struct {
	EntryIndex int
	ToolUseID  string
	ToolName   string
	ParentUUID string // UUID of the assistant entry containing this tool_use
}

// FindOrphanedToolUses scans entries and returns tool_use blocks without matching tool_results.
func FindOrphanedToolUses(entries []jsonl.Entry) []OrphanedToolUse {
	// Collect all tool_result IDs from user messages.
	resultIDs := make(map[string]bool)
	for _, e := range entries {
		if e.Type != jsonl.TypeUser || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID != "" {
				resultIDs[b.ToolUseID] = true
			}
		}
	}

	// Find tool_use blocks without results.
	var orphans []OrphanedToolUse
	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID != "" && !resultIDs[b.ID] {
				orphans = append(orphans, OrphanedToolUse{
					EntryIndex: i,
					ToolUseID:  b.ID,
					ToolName:   b.Name,
					ParentUUID: e.UUID,
				})
			}
		}
	}
	return orphans
}

// Rewire injects synthetic tool_result entries for orphaned tool_use blocks.
// Groups orphaned tool_uses by their parent assistant entry and injects one
// user entry per assistant entry, placed immediately after it.
func Rewire(path string) (*RewireResult, error) {
	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	orphans := FindOrphanedToolUses(entries)
	if len(orphans) == 0 {
		return &RewireResult{}, nil
	}

	// Create backup.
	if err := safecopy.CreateIfMissing(path); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}

	result := &RewireResult{}
	for _, raw := range rawLines {
		result.BytesBefore += int64(len(raw))
	}

	// Group orphans by entry index (multiple tool_use in same assistant message).
	grouped := make(map[int][]OrphanedToolUse)
	for _, o := range orphans {
		grouped[o.EntryIndex] = append(grouped[o.EntryIndex], o)
	}

	// Build new lines: copy originals and inject synthetic tool_results after each affected assistant entry.
	var newLines [][]byte
	for i, raw := range rawLines {
		newLines = append(newLines, raw)

		orphansHere, ok := grouped[i]
		if !ok {
			continue
		}

		// Build synthetic tool_result content blocks.
		var blocks []syntheticToolResult
		for _, o := range orphansHere {
			blocks = append(blocks, syntheticToolResult{
				Type:      "tool_result",
				ToolUseID: o.ToolUseID,
				Content:   "result unavailable — recovered by contextspectre rewire",
			})
			result.Injected++
		}

		// Build the synthetic user entry.
		synth := syntheticEntry{
			Type:       "user",
			ParentUUID: entries[i].UUID,
			Timestamp:  entries[i].Timestamp.Add(time.Millisecond),
			Message: syntheticMessage{
				Role:    "user",
				Content: blocks,
			},
		}

		synthJSON, err := json.Marshal(synth)
		if err != nil {
			return nil, fmt.Errorf("marshal synthetic entry: %w", err)
		}
		newLines = append(newLines, synthJSON)
	}

	// Write back.
	if err := jsonl.WriteLines(path, newLines); err != nil {
		_ = safecopy.Restore(path) // best-effort restore on failure
		return nil, fmt.Errorf("write: %w", err)
	}

	for _, line := range newLines {
		result.BytesAfter += int64(len(line))
	}

	return result, nil
}

type syntheticEntry struct {
	Type       string           `json:"type"`
	ParentUUID string           `json:"parentUuid,omitempty"`
	Timestamp  time.Time        `json:"timestamp"`
	Message    syntheticMessage `json:"message"`
}

type syntheticMessage struct {
	Role    string                `json:"role"`
	Content []syntheticToolResult `json:"content"`
}

type syntheticToolResult struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}
