package analyzer

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// OversizedImageThreshold is the base64 data length above which an image is considered oversized.
// ~5 MB of base64 data ≈ 3.75 MB decoded image.
const OversizedImageThreshold = 5 * 1024 * 1024

// IssueKind classifies the type of session problem detected.
type IssueKind string

const (
	IssueFilterBlock    IssueKind = "filter_block"
	IssueOversizedImage IssueKind = "oversized_image"
	IssueOrphanedResult IssueKind = "orphaned_result"
	IssueMalformed      IssueKind = "malformed"
)

// Issue describes a single detected problem in a session.
type Issue struct {
	Kind        IssueKind
	EntryIndex  int
	Description string
	// RelatedIndex is set for filter_block (the triggering user message index).
	RelatedIndex int
}

// DiagnosisResult holds all detected issues.
type DiagnosisResult struct {
	Issues []Issue
}

// Diagnose scans session entries for common problems.
func Diagnose(entries []jsonl.Entry) *DiagnosisResult {
	result := &DiagnosisResult{}

	// Build tool_use ID set from assistant messages
	toolUseIDs := make(map[string]bool)
	for _, e := range entries {
		for _, id := range e.ToolUseIDs() {
			toolUseIDs[id] = true
		}
	}

	for i, e := range entries {
		// Content filter blocks: assistant with empty/error content after a user message
		if e.Type == jsonl.TypeAssistant && e.Message != nil {
			if isEmptyContent(e.Message.Content) {
				userIdx := findPrecedingUser(entries, i)
				result.Issues = append(result.Issues, Issue{
					Kind:         IssueFilterBlock,
					EntryIndex:   i,
					RelatedIndex: userIdx,
					Description:  "assistant response is empty (possible content filter block)",
				})
			}
		}

		// Oversized images
		if e.HasImages() {
			blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
			if err == nil {
				for _, b := range blocks {
					if b.Type == "image" && b.Source != nil && len(b.Source.Data) > OversizedImageThreshold {
						sizeMB := float64(len(b.Source.Data)*3/4) / 1024 / 1024
						result.Issues = append(result.Issues, Issue{
							Kind:        IssueOversizedImage,
							EntryIndex:  i,
							Description: fmt.Sprintf("oversized image: %.1f MB", sizeMB),
						})
					}
				}
			}
		}

		// Orphaned tool results: tool_result referencing non-existent tool_use
		if e.Type == jsonl.TypeUser && e.Message != nil {
			blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
			if err == nil {
				for _, b := range blocks {
					if b.Type == "tool_result" && b.ToolUseID != "" && !toolUseIDs[b.ToolUseID] {
						result.Issues = append(result.Issues, Issue{
							Kind:        IssueOrphanedResult,
							EntryIndex:  i,
							Description: fmt.Sprintf("tool_result references missing tool_use %s", b.ToolUseID),
						})
					}
				}
			}
		}
	}

	return result
}

// IssuesByIndex returns a map from entry index to issues affecting it.
func (d *DiagnosisResult) IssuesByIndex() map[int][]Issue {
	m := make(map[int][]Issue)
	for _, issue := range d.Issues {
		m[issue.EntryIndex] = append(m[issue.EntryIndex], issue)
		if issue.RelatedIndex >= 0 && issue.RelatedIndex != issue.EntryIndex {
			m[issue.RelatedIndex] = append(m[issue.RelatedIndex], issue)
		}
	}
	return m
}

func isEmptyContent(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	// Check for empty string: ""
	if string(raw) == `""` {
		return true
	}
	// Check for empty array: []
	if string(raw) == `[]` {
		return true
	}
	// Check for null
	if string(raw) == `null` {
		return true
	}
	return false
}

func findPrecedingUser(entries []jsonl.Entry, assistantIdx int) int {
	if assistantIdx <= 0 {
		return -1
	}
	// Walk backward to find the nearest user message
	for i := assistantIdx - 1; i >= 0; i-- {
		if entries[i].Type == jsonl.TypeUser {
			return i
		}
	}
	return -1
}
