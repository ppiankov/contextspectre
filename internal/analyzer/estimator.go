package analyzer

import (
	"encoding/json"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// Token estimation constants.
const (
	CharsPerToken       = 4   // ~4 characters per token for English text
	Base64BytesPerToken = 750 // ~750 bytes of base64 per image token
)

// EstimateTokens estimates the token count for a single entry.
func EstimateTokens(e *jsonl.Entry) int {
	if e.Message == nil {
		return 0
	}

	total := estimateContentTokens(e.Message.Content)

	// Add overhead for toolUseResult (top-level field, can be huge)
	if len(e.ToolUseResult) > 0 {
		total += len(e.ToolUseResult) / CharsPerToken
	}

	return total
}

func estimateContentTokens(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}

	// Try string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return len(s) / CharsPerToken
	}

	// Parse as content blocks
	blocks, err := jsonl.ParseContentBlocks(raw)
	if err != nil {
		return len(raw) / CharsPerToken
	}

	total := 0
	for _, b := range blocks {
		switch b.Type {
		case "text":
			total += len(b.Text) / CharsPerToken
		case "tool_use":
			total += len(b.Name) / CharsPerToken
			total += len(b.Input) / CharsPerToken
		case "tool_result":
			total += estimateToolResultTokens(b)
		case "image":
			if b.Source != nil {
				total += len(b.Source.Data) / Base64BytesPerToken
			}
		}
	}
	return total
}

func estimateToolResultTokens(b jsonl.ContentBlock) int {
	// Tool result content can be a string or array
	if len(b.Content) == 0 {
		return 0
	}
	var s string
	if err := json.Unmarshal(b.Content, &s); err == nil {
		return len(s) / CharsPerToken
	}
	return len(b.Content) / CharsPerToken
}

// EstimateImageBytes returns the approximate decoded size of all images in an entry.
func EstimateImageBytes(e *jsonl.Entry) int64 {
	if e.Message == nil {
		return 0
	}
	blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
	if err != nil {
		return 0
	}

	var total int64
	for _, b := range blocks {
		if b.Type == "image" && b.Source != nil {
			total += int64(len(b.Source.Data) * 3 / 4) // base64 to bytes
		}
	}
	return total
}
