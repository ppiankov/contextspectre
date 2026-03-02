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
// A sequence is flagged only when the same tool name appears again within
// retryWindow entries and the original tool_result indicates an error.
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
			toolUseIdx, toolName := findToolUse(entries, i, b.ToolUseID)
			if toolUseIdx < 0 {
				continue
			}

			// Look for a retry of the same tool within the window
			retryIdx := findRetry(entries, i, toolName, retryWindow)
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
// Searches backwards from the tool_result entry.
func findToolUse(entries []jsonl.Entry, beforeIdx int, toolUseID string) (int, string) {
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
				return i, b.Name
			}
		}
	}
	return -1, ""
}

// findRetry looks for a subsequent tool_use of the same tool name within window entries.
func findRetry(entries []jsonl.Entry, afterIdx int, toolName string, window int) int {
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
			if b.Type == "tool_use" && b.Name == toolName {
				return i
			}
		}
	}
	return -1
}
