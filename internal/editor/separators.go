package editor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// Separator detection patterns.
var (
	// Box-drawing horizontal runs (20+ chars)
	reBoxDrawing = regexp.MustCompile(`[─━═╌╍╴╶╸╺]{20,}`)
	// ASCII separator runs (40+ chars to avoid false positives)
	reASCIISep = regexp.MustCompile(`[-=~_]{40,}`)
)

// StripResult holds the outcome of separator stripping.
type StripResult struct {
	LinesStripped    int
	MessagesModified int
	CharsSaved       int
}

// StripSeparators removes decorative separator lines from assistant messages.
// Only modifies assistant entries. Creates a backup before writing.
func StripSeparators(path string) (*StripResult, error) {
	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	result := &StripResult{}
	modified := false

	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		lineModified := false
		for j := range blocks {
			if blocks[j].Type != "text" || blocks[j].Text == "" {
				continue
			}

			cleaned, linesRemoved, charsSaved := stripSeparatorLines(blocks[j].Text)
			if linesRemoved > 0 {
				blocks[j].Text = cleaned
				result.LinesStripped += linesRemoved
				result.CharsSaved += charsSaved
				lineModified = true
			}
		}

		if lineModified {
			updated, err := reserializeContent(rawLines[i], blocks)
			if err != nil {
				continue
			}
			rawLines[i] = updated
			result.MessagesModified++
			modified = true
		}
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

// CountSeparatorTokens estimates the token cost of separators in entries.
func CountSeparatorTokens(entries []jsonl.Entry) int {
	total := 0
	for _, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "text" {
				_, _, chars := stripSeparatorLines(b.Text)
				total += chars / 4 // ~4 chars per token
			}
		}
	}
	return total
}

// stripSeparatorLines removes full separator lines from text.
// Returns cleaned text, lines removed count, and characters saved.
func stripSeparatorLines(text string) (string, int, int) {
	lines := strings.Split(text, "\n")
	var kept []string
	removed := 0
	charsSaved := 0

	for _, line := range lines {
		if isSeparatorLine(line) {
			removed++
			charsSaved += len(line)
		} else {
			kept = append(kept, line)
		}
	}

	if removed == 0 {
		return text, 0, 0
	}

	return strings.Join(kept, "\n"), removed, charsSaved
}

// isSeparatorLine returns true if a line is purely decorative separator content.
func isSeparatorLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false // don't strip blank lines
	}
	// Box-drawing: 20+ chars of box-drawing characters only
	if reBoxDrawing.MatchString(trimmed) && len(trimmed) == len(reBoxDrawing.FindString(trimmed)) {
		return true
	}
	// ASCII: 40+ chars of separator characters only
	if reASCIISep.MatchString(trimmed) && len(trimmed) == len(reASCIISep.FindString(trimmed)) {
		return true
	}
	return false
}
