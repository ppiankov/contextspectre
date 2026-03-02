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

// FindDuplicateReads scans entries for files read more than once.
// Returns groups of duplicate reads with stale indices marked.
func FindDuplicateReads(entries []jsonl.Entry) *DuplicateReadResult {
	// Map: file_path -> list of (assistant entry index, tool_use_id)
	type readInfo struct {
		assistantIdx int
		toolUseID    string
	}
	fileReads := make(map[string][]readInfo)

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
			if !isFileReadTool(b.Name) {
				continue
			}
			path := extractFilePath(b.Input)
			if path == "" {
				continue
			}
			fileReads[path] = append(fileReads[path], readInfo{
				assistantIdx: i,
				toolUseID:    b.ID,
			})
		}
	}

	result := &DuplicateReadResult{}

	for filePath, reads := range fileReads {
		if len(reads) < 2 {
			continue
		}

		group := DuplicateGroup{
			FilePath:    filePath,
			LatestIndex: reads[len(reads)-1].assistantIdx,
		}

		for _, r := range reads {
			group.ReadIndices = append(group.ReadIndices, r.assistantIdx)
		}

		// All but the last are stale
		for _, r := range reads[:len(reads)-1] {
			sr := StaleRead{
				AssistantIdx: r.assistantIdx,
				ToolUseID:    r.toolUseID,
				ResultIdx:    findToolResult(entries, r.assistantIdx, r.toolUseID),
			}
			group.StaleReads = append(group.StaleReads, sr)

			// Estimate tokens
			if sr.ResultIdx >= 0 {
				group.EstimatedTokens += entries[sr.ResultIdx].RawSize / 4
			}
			group.EstimatedTokens += entries[sr.AssistantIdx].RawSize / 4
		}

		result.Groups = append(result.Groups, group)
		result.TotalStale += len(reads) - 1
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
