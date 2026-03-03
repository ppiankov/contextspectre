package editor

import (
	"fmt"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// constraintKeywords are structural indicators of constraints in text.
var constraintKeywords = []string{
	"must", "cannot", "should not", "required", "never", "always",
}

// ExtractCanonicalState scans entries[0..cursorIdx] and builds a CommitPoint
// with extracted decisions, constraints, questions, and file references.
func ExtractCanonicalState(entries []jsonl.Entry, cursorIdx int) CommitPoint {
	if cursorIdx >= len(entries) {
		cursorIdx = len(entries) - 1
	}

	cp := CommitPoint{
		UUID:      entries[cursorIdx].UUID,
		Timestamp: time.Now(),
	}

	fileSet := make(map[string]bool)

	for i := 0; i <= cursorIdx; i++ {
		e := entries[i]
		if e.Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		switch e.Type {
		case jsonl.TypeUser:
			for _, b := range blocks {
				if b.Type != "text" {
					continue
				}
				text := strings.TrimSpace(b.Text)
				// Goal: first user message text
				if cp.Goal == "" && text != "" {
					cp.Goal = analyzer.TruncateHint(text, 120)
				}
				// Questions
				if strings.HasSuffix(text, "?") && len(cp.Questions) < 10 {
					cp.Questions = append(cp.Questions, analyzer.TruncateHint(text, 120))
				}
			}

		case jsonl.TypeAssistant:
			for _, b := range blocks {
				if b.Type == "tool_use" {
					path := analyzer.ExtractToolInputPath(b.Input)
					if path != "" {
						fileSet[path] = true
					}
				}
				if b.Type == "text" {
					// Decisions
					if len(cp.Decisions) < 10 {
						if hint := analyzer.ExtractDecisionHint(b.Text); hint != "" {
							cp.Decisions = append(cp.Decisions, hint)
						}
					}
					// Constraints
					if len(cp.Constraints) < 10 {
						if hint := extractConstraintHint(b.Text); hint != "" {
							cp.Constraints = append(cp.Constraints, hint)
						}
					}
				}
			}
		}
	}

	for path := range fileSet {
		cp.Files = append(cp.Files, path)
	}

	return cp
}

// extractConstraintHint returns a truncated hint if the text contains constraint keywords.
func extractConstraintHint(text string) string {
	lower := strings.ToLower(text)
	for _, kw := range constraintKeywords {
		idx := strings.Index(lower, kw)
		if idx >= 0 {
			start := idx
			if start > 20 {
				start = idx - 20
			}
			snippet := text[start:]
			return analyzer.TruncateHint(snippet, 120)
		}
	}
	return ""
}

// CollapseResult holds the result of a collapse operation.
type CollapseResult struct {
	EntriesRemoved int
	ChainRepairs   int
	BytesBefore    int64
	BytesAfter     int64
}

// Collapse deletes CANDIDATE entries above a commit point, preserving KEEP entries.
// Creates a backup before modifying. Removes the commit point from the sidecar after collapse.
func Collapse(path string, commitPointUUID string) (*CollapseResult, error) {
	entries, err := jsonl.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	markers, err := LoadMarkers(path)
	if err != nil {
		return nil, fmt.Errorf("load markers: %w", err)
	}

	// Find commit point entry by UUID
	commitIdx := -1
	for i, e := range entries {
		if e.UUID == commitPointUUID {
			commitIdx = i
			break
		}
	}
	if commitIdx < 0 {
		return nil, fmt.Errorf("commit point %s not found", commitPointUUID)
	}

	// Build toDelete: CANDIDATE entries above the commit point
	toDelete := make(map[int]bool)
	for i := 0; i < commitIdx; i++ {
		uuid := entries[i].UUID
		if uuid != "" && markers.Get(uuid) == MarkerCandidate {
			toDelete[i] = true
		}
	}

	if len(toDelete) == 0 {
		return &CollapseResult{}, nil
	}

	dr, err := Delete(path, toDelete)
	if err != nil {
		return nil, fmt.Errorf("delete: %w", err)
	}

	// Clean up sidecar: remove markers for deleted entries, remove commit point
	for i := 0; i < commitIdx; i++ {
		uuid := entries[i].UUID
		if uuid != "" && toDelete[i] {
			markers.Clear(uuid)
		}
	}
	markers.RemoveCommitPoint(commitPointUUID)
	_ = SaveMarkers(path, markers)

	return &CollapseResult{
		EntriesRemoved: dr.EntriesRemoved,
		ChainRepairs:   dr.ChainRepairs,
		BytesBefore:    dr.BytesBefore,
		BytesAfter:     dr.BytesAfter,
	}, nil
}

// CollapsePreview returns the count of CANDIDATE entries above a commit point
// without modifying anything (dry-run).
func CollapsePreview(path string, commitPointUUID string) (int, error) {
	entries, err := jsonl.Parse(path)
	if err != nil {
		return 0, fmt.Errorf("parse: %w", err)
	}

	markers, err := LoadMarkers(path)
	if err != nil {
		return 0, fmt.Errorf("load markers: %w", err)
	}

	commitIdx := -1
	for i, e := range entries {
		if e.UUID == commitPointUUID {
			commitIdx = i
			break
		}
	}
	if commitIdx < 0 {
		return 0, fmt.Errorf("commit point %s not found", commitPointUUID)
	}

	count := 0
	for i := 0; i < commitIdx; i++ {
		uuid := entries[i].UUID
		if uuid != "" && markers.Get(uuid) == MarkerCandidate {
			count++
		}
	}
	return count, nil
}
