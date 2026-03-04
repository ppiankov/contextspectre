package analyzer

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

const maxSearchableSize = 50 * 1024 // 50KB — skip very large tool results

// SearchHit represents a single search match within a session.
type SearchHit struct {
	EntryIndex int
	Timestamp  time.Time
	Role       string // "user", "assistant", "tool_use", "tool_result"
	Snippet    string // match with surrounding context
	MatchStart int    // offset within snippet for highlighting
	MatchLen   int
}

// Search finds all occurrences of query in the given entries.
// If ignoreCase is true, matching is case-insensitive.
func Search(entries []jsonl.Entry, query string, ignoreCase bool) []SearchHit {
	if query == "" {
		return nil
	}

	searchQuery := query
	if ignoreCase {
		searchQuery = strings.ToLower(query)
	}

	var hits []SearchHit

	for i, e := range entries {
		// Skip noise
		switch e.Type {
		case jsonl.TypeProgress, jsonl.TypeFileHistorySnapshot, jsonl.TypeQueueOperation:
			continue
		}
		if e.IsSidechain {
			continue
		}
		if e.Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		role := string(e.Type)

		for _, b := range blocks {
			switch b.Type {
			case "text":
				if h := searchText(b.Text, searchQuery, ignoreCase, query); h != nil {
					h.EntryIndex = i
					h.Timestamp = e.Timestamp
					h.Role = role
					hits = append(hits, *h)
				}

			case "tool_use":
				// Search tool name
				if h := searchText(b.Name, searchQuery, ignoreCase, query); h != nil {
					h.EntryIndex = i
					h.Timestamp = e.Timestamp
					h.Role = "tool_use"
					hits = append(hits, *h)
					continue
				}
				// Search tool input (skip if too large)
				if len(b.Input) > 0 && len(b.Input) < maxSearchableSize {
					inputStr := string(b.Input)
					if h := searchText(inputStr, searchQuery, ignoreCase, query); h != nil {
						h.EntryIndex = i
						h.Timestamp = e.Timestamp
						h.Role = "tool_use"
						hits = append(hits, *h)
					}
				}

			case "tool_result":
				if b.Content == nil || len(b.Content) > maxSearchableSize {
					continue
				}
				var contentStr string
				if json.Unmarshal(b.Content, &contentStr) != nil {
					contentStr = string(b.Content)
				}
				if h := searchText(contentStr, searchQuery, ignoreCase, query); h != nil {
					h.EntryIndex = i
					h.Timestamp = e.Timestamp
					h.Role = "tool_result"
					hits = append(hits, *h)
				}

			case "image":
				continue
			}
		}
	}

	return hits
}

// searchText finds the first occurrence of query in text and builds a snippet.
// Returns nil if no match found.
func searchText(text, searchQuery string, ignoreCase bool, originalQuery string) *SearchHit {
	if text == "" {
		return nil
	}

	haystack := text
	if ignoreCase {
		haystack = strings.ToLower(text)
	}

	idx := strings.Index(haystack, searchQuery)
	if idx < 0 {
		return nil
	}

	snippet, matchStart := buildSnippet(text, idx, len(originalQuery), 200)
	return &SearchHit{
		Snippet:    snippet,
		MatchStart: matchStart,
		MatchLen:   len(originalQuery),
	}
}

// buildSnippet extracts a snippet of maxLen characters around the match position.
func buildSnippet(text string, matchIdx, matchLen, maxLen int) (string, int) {
	runes := []rune(text)
	totalLen := len(runes)

	if totalLen <= maxLen {
		return string(runes), matchIdx
	}

	// Center the match in the snippet
	contextBefore := (maxLen - matchLen) / 2
	if contextBefore < 0 {
		contextBefore = 0
	}

	start := matchIdx - contextBefore
	if start < 0 {
		start = 0
	}

	end := start + maxLen
	if end > totalLen {
		end = totalLen
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}

	snippet := string(runes[start:end])
	newMatchStart := matchIdx - start

	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "..."
		newMatchStart += 3
	}
	if end < totalLen {
		suffix = "..."
	}

	return prefix + snippet + suffix, newMatchStart
}
