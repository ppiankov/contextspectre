package analyzer

import (
	"encoding/json"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// StaleRead holds details about a single stale file read for cleanup.
type StaleRead struct {
	AssistantIdx int    // index of the assistant entry with the tool_use
	ToolUseID    string // tool_use ID for content surgery
	ResultIdx    int    // index of the tool_result entry, -1 if not found
}

// DuplicateGroup represents a file that was read multiple times.
type DuplicateGroup struct {
	FilePath        string
	ReadIndices     []int       // all entry indices containing tool_use for this file read
	LatestIndex     int         // the most recent read (to keep)
	StaleReads      []StaleRead // older reads with details for cleanup
	EstimatedTokens int         // total tokens across stale reads
}

// StaleIndices returns all entry indices that are stale (assistant + tool_result).
func (g DuplicateGroup) StaleIndices() []int {
	var indices []int
	for _, sr := range g.StaleReads {
		indices = append(indices, sr.AssistantIdx)
		if sr.ResultIdx >= 0 {
			indices = append(indices, sr.ResultIdx)
		}
	}
	return indices
}

// DuplicateReadResult summarizes all duplicate reads in a session.
type DuplicateReadResult struct {
	Groups      []DuplicateGroup
	TotalStale  int
	TotalTokens int
	UniqueFiles int
}

// AllStaleIndices returns every stale entry index across all groups.
func (r *DuplicateReadResult) AllStaleIndices() map[int]bool {
	m := make(map[int]bool)
	for _, g := range r.Groups {
		for _, idx := range g.StaleIndices() {
			m[idx] = true
		}
	}
	return m
}

// fileEvent is a read or write to a specific file, in entry order.
type fileEvent struct {
	entryIdx  int
	toolUseID string // non-empty for reads
	isWrite   bool
}

// FindDuplicateReads scans entries for files read more than once without
// intervening writes. A read after a write to the same file is fresh (the file
// may have changed), not stale.
func FindDuplicateReads(entries []jsonl.Entry) *DuplicateReadResult {
	// Collect all file events (reads and writes) per normalized path.
	fileEvents := make(map[string][]fileEvent)

	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			path := NormalizePath(extractFilePath(b.Input))
			if path == "" {
				continue
			}
			if isFileReadTool(b.Name) {
				fileEvents[path] = append(fileEvents[path], fileEvent{
					entryIdx:  i,
					toolUseID: b.ID,
				})
			} else if isFileWriteTool(b.Name) {
				fileEvents[path] = append(fileEvents[path], fileEvent{
					entryIdx: i,
					isWrite:  true,
				})
			}
		}
	}

	result := &DuplicateReadResult{}

	for filePath, events := range fileEvents {
		// Scan left-to-right: write clears lastRead, read with lastRead set → lastRead is stale.
		var staleReads []StaleRead
		var readIndices []int
		var lastRead *fileEvent
		latestReadIdx := -1

		for i := range events {
			ev := &events[i]
			if ev.isWrite {
				lastRead = nil
				continue
			}
			// It's a read
			readIndices = append(readIndices, ev.entryIdx)
			if lastRead != nil {
				sr := StaleRead{
					AssistantIdx: lastRead.entryIdx,
					ToolUseID:    lastRead.toolUseID,
					ResultIdx:    findToolResult(entries, lastRead.entryIdx, lastRead.toolUseID),
				}
				staleReads = append(staleReads, sr)
			}
			lastRead = ev
			latestReadIdx = ev.entryIdx
		}

		if len(staleReads) == 0 {
			continue
		}

		group := DuplicateGroup{
			FilePath:    filePath,
			ReadIndices: readIndices,
			LatestIndex: latestReadIdx,
			StaleReads:  staleReads,
		}

		for _, sr := range staleReads {
			if sr.ResultIdx >= 0 {
				group.EstimatedTokens += entries[sr.ResultIdx].RawSize / 4
			}
			group.EstimatedTokens += entries[sr.AssistantIdx].RawSize / 4
		}

		result.Groups = append(result.Groups, group)
		result.TotalStale += len(staleReads)
		result.TotalTokens += group.EstimatedTokens
		result.UniqueFiles++
	}

	return result
}

// isFileReadTool returns true for tool names that read file contents.
func isFileReadTool(name string) bool {
	switch name {
	case "Read", "View", "read_file", "ReadFile":
		return true
	}
	return false
}

// extractFilePath extracts the file path from a tool_use input.
func extractFilePath(input json.RawMessage) string {
	var fields struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	if fields.FilePath != "" {
		return fields.FilePath
	}
	return fields.Path
}

// findToolResult finds the tool_result entry for a given tool_use.
func findToolResult(entries []jsonl.Entry, afterIdx int, toolUseID string) int {
	for i := afterIdx + 1; i < len(entries) && i < afterIdx+5; i++ {
		e := entries[i]
		if e.Type != jsonl.TypeUser || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID == toolUseID {
				return i
			}
		}
	}
	return -1
}
