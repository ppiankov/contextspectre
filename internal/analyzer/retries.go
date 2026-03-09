package analyzer

import (
	"encoding/json"
	"strings"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// RetrySequence represents a failed tool attempt that was retried.
type RetrySequence struct {
	FailedToolUseIdx int    // index of assistant entry with failed tool_use
	FailedToolUseID  string // tool_use ID of the failed attempt
	FailedResultIdx  int    // index of user entry with error tool_result
	RetryToolUseIdx  int    // index of assistant entry with retry tool_use
	ToolName         string // tool name (e.g., "Bash", "Read")
	EstimatedTokens  int    // tokens in the failed attempt
}

// RetryResult summarizes all failed-then-retried sequences.
type RetryResult struct {
	Sequences   []RetrySequence
	TotalFailed int
	TotalTokens int
}

// AllFailedIndices returns all entry indices that are part of failed attempts.
func (r *RetryResult) AllFailedIndices() map[int]bool {
	m := make(map[int]bool)
	for _, s := range r.Sequences {
		m[s.FailedToolUseIdx] = true
		if s.FailedResultIdx >= 0 {
			m[s.FailedResultIdx] = true
		}
	}
	return m
}

// FindFailedRetries detects tool_use attempts that failed and were retried.
// A retry is identified by matching tool input signatures (not just tool name),
// so unrelated Bash commands are not mislabeled as retries.
func FindFailedRetries(entries []jsonl.Entry) *RetryResult {
	const retryWindow = 6

	result := &RetryResult{}

	for i, e := range entries {
		if e.Type != jsonl.TypeUser || e.Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		for _, b := range blocks {
			if b.Type != "tool_result" {
				continue
			}
			if !isErrorResult(b) {
				continue
			}

			// Find the original tool_use for this result
			toolUseIdx, toolName, toolInput := findToolUse(entries, i, b.ToolUseID)
			if toolUseIdx < 0 {
				continue
			}

			// Build signature and look for a retry with matching signature
			sig := retrySignature(toolName, toolInput)
			retryIdx := findRetry(entries, i, sig, retryWindow)
			if retryIdx < 0 {
				continue
			}

			seq := RetrySequence{
				FailedToolUseIdx: toolUseIdx,
				FailedToolUseID:  b.ToolUseID,
				FailedResultIdx:  i,
				RetryToolUseIdx:  retryIdx,
				ToolName:         toolName,
				EstimatedTokens:  entries[toolUseIdx].RawSize/4 + entries[i].RawSize/4,
			}

			result.Sequences = append(result.Sequences, seq)
			result.TotalFailed++
			result.TotalTokens += seq.EstimatedTokens
		}
	}

	return result
}

// retrySignature returns a canonical string identifying what a tool_use is attempting.
// File tools: "Read:/normalized/path". Bash: "Bash:<command>". Grep/Glob: "Grep:<pattern>:<path>".
// Fallback: tool name only.
func retrySignature(name string, input json.RawMessage) string {
	if isFileReadTool(name) || isFileWriteTool(name) {
		if p := NormalizePath(extractFilePath(input)); p != "" {
			return name + ":" + p
		}
	}
	if isBashLikeTool(name) {
		var fields struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(input, &fields); err == nil && fields.Command != "" {
			return "Bash:" + strings.TrimSpace(fields.Command)
		}
	}
	if isGrepLikeTool(name) {
		var fields struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if err := json.Unmarshal(input, &fields); err == nil && fields.Pattern != "" {
			return name + ":" + fields.Pattern + ":" + fields.Path
		}
	}
	return name
}

func isGrepLikeTool(name string) bool {
	switch name {
	case "Grep", "Glob", "grep", "glob", "search":
		return true
	}
	return false
}

// isErrorResult checks if a tool_result block indicates an error.
func isErrorResult(b jsonl.ContentBlock) bool {
	if b.IsError {
		return true
	}

	// Check content for error indicators
	var text string
	if json.Unmarshal(b.Content, &text) == nil {
		return containsErrorIndicator(text)
	}

	// Array content
	var blocks []jsonl.ContentBlock
	if json.Unmarshal(b.Content, &blocks) == nil {
		for _, cb := range blocks {
			if cb.Type == "text" && containsErrorIndicator(cb.Text) {
				return true
			}
		}
	}

	return false
}

// containsErrorIndicator checks text for common error patterns.
func containsErrorIndicator(text string) bool {
	lower := strings.ToLower(text)
	indicators := []string{
		"error:", "permission denied", "no such file",
		"command not found", "exit status", "fatal:",
		"panic:", "traceback", "exception:",
	}
	for _, ind := range indicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	return false
}

// findToolUse finds the assistant entry containing a specific tool_use ID.
// Returns entry index, tool name, and raw input for signature computation.
func findToolUse(entries []jsonl.Entry, beforeIdx int, toolUseID string) (int, string, json.RawMessage) {
	for i := beforeIdx - 1; i >= 0 && i >= beforeIdx-5; i-- {
		e := entries[i]
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID == toolUseID {
				return i, b.Name, b.Input
			}
		}
	}
	return -1, "", nil
}

// findRetry looks for a subsequent tool_use with matching signature within window entries.
func findRetry(entries []jsonl.Entry, afterIdx int, sig string, window int) int {
	for i := afterIdx + 1; i < len(entries) && i <= afterIdx+window; i++ {
		e := entries[i]
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && retrySignature(b.Name, b.Input) == sig {
				return i
			}
		}
	}
	return -1
}
