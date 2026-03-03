package editor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// TruncateResult holds the result of an output truncation operation.
type TruncateResult struct {
	OutputsTruncated int
	TokensSaved      int
	BytesBefore      int64
	BytesAfter       int64
}

// TruncateOutputs truncates large Bash tool_result content to first+last keepLines lines.
// threshold is the minimum byte size to trigger truncation.
// keepLines is the number of lines to keep at the start and end.
func TruncateOutputs(path string, threshold, keepLines int) (*TruncateResult, error) {
	if threshold <= 0 {
		threshold = analyzer.LargeOutputThreshold
	}
	if keepLines <= 0 {
		keepLines = 10
	}

	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	// Collect Bash tool_use IDs
	bashIDs := make(map[string]bool)
	for _, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && isBashTool(b.Name) {
				bashIDs[b.ID] = true
			}
		}
	}

	result := &TruncateResult{}
	for _, raw := range rawLines {
		result.BytesBefore += int64(len(raw))
	}

	modified := false

	for i, e := range entries {
		if e.Type != jsonl.TypeUser || e.Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		lineModified := false
		for j := range blocks {
			b := &blocks[j]
			if b.Type != "tool_result" || !bashIDs[b.ToolUseID] {
				continue
			}

			// Extract text content
			text, isString := extractToolResultText(b.Content)
			if text == "" || len(text) < threshold {
				continue
			}

			// Truncate
			truncated, linesSaved := truncateText(text, keepLines)
			if linesSaved == 0 {
				continue
			}

			result.OutputsTruncated++
			result.TokensSaved += (len(text) - len(truncated)) / 4

			// Write back
			if isString {
				newContent, _ := json.Marshal(truncated)
				b.Content = newContent
			} else {
				// Array content — replace the text block
				var contentBlocks []jsonl.ContentBlock
				if json.Unmarshal(b.Content, &contentBlocks) == nil {
					for k := range contentBlocks {
						if contentBlocks[k].Type == "text" {
							contentBlocks[k].Text = truncated
						}
					}
					b.Content, _ = json.Marshal(contentBlocks)
				}
			}
			lineModified = true
		}

		if lineModified {
			updated, err := reserializeContent(rawLines[i], blocks)
			if err != nil {
				continue
			}
			rawLines[i] = updated
			modified = true
		}
	}

	for _, raw := range rawLines {
		result.BytesAfter += int64(len(raw))
	}

	if !modified {
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

// isBashTool returns true for tool names that execute shell commands.
func isBashTool(name string) bool {
	switch name {
	case "Bash", "bash", "execute_command", "run_command":
		return true
	}
	return false
}

// extractToolResultText extracts text from tool_result content.
// Returns the text and whether it was a plain string (vs array).
func extractToolResultText(content json.RawMessage) (string, bool) {
	if content == nil {
		return "", false
	}
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s, true
	}
	// Try array of content blocks
	var blocks []jsonl.ContentBlock
	if json.Unmarshal(content, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return b.Text, false
			}
		}
	}
	return "", false
}

// truncateText keeps first and last keepLines lines with a truncation marker.
// Returns the truncated text and the number of lines removed.
func truncateText(text string, keepLines int) (string, int) {
	lines := strings.Split(text, "\n")
	totalLines := len(lines)

	// Need more than 2*keepLines lines to truncate
	if totalLines <= keepLines*2+1 {
		return text, 0
	}

	removed := totalLines - keepLines*2
	head := strings.Join(lines[:keepLines], "\n")
	tail := strings.Join(lines[totalLines-keepLines:], "\n")

	return head + "\n" +
		fmt.Sprintf("[... %d lines truncated by contextspectre ...]", removed) + "\n" +
		tail, removed
}
