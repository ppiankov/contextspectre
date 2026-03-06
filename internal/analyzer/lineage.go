package analyzer

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// FileAction classifies what a tool_use does to a file.
type FileAction string

const (
	ActionRead   FileAction = "read"
	ActionWrite  FileAction = "write"
	ActionEdit   FileAction = "edit"
	ActionSearch FileAction = "search"
	ActionBash   FileAction = "bash"
)

// FileTouch records a single interaction with a file in a session.
type FileTouch struct {
	EntryIndex int        `json:"entry_index"`
	Timestamp  time.Time  `json:"timestamp"`
	Action     FileAction `json:"action"`
	ToolName   string     `json:"tool_name"`
	Epoch      int        `json:"epoch"`
	TurnCost   float64    `json:"turn_cost"`
}

// DecisionReference records a commit-point decision that mentions a file.
type DecisionReference struct {
	UUID       string    `json:"uuid"`
	Goal       string    `json:"goal"`
	Decisions  []string  `json:"decisions,omitempty"`
	Epoch      int       `json:"epoch"`
	EntryIndex int       `json:"entry_index"`
	Timestamp  time.Time `json:"timestamp"`
}

// SessionLineage holds all file touches and decision references for one session.
type SessionLineage struct {
	Touches   []FileTouch         `json:"touches"`
	Decisions []DecisionReference `json:"decisions"`
	TotalCost float64             `json:"total_cost"`
}

// ClassifyToolAction maps a tool_use name to a FileAction.
func ClassifyToolAction(toolName string) FileAction {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "read":
		return ActionRead
	case "write":
		return ActionWrite
	case "edit", "multiedit", "notebookedit":
		return ActionEdit
	case "grep", "glob":
		return ActionSearch
	default:
		return ActionBash
	}
}

// MatchesPathSuffix returns true if candidate ends with the query as a path suffix.
func MatchesPathSuffix(candidate, query string) bool {
	if candidate == "" || query == "" {
		return false
	}
	candidate = filepath.Clean(candidate)
	query = filepath.Clean(query)
	if candidate == query {
		return true
	}
	return strings.HasSuffix(candidate, string(filepath.Separator)+query)
}

// ExtractFileLineage scans entries for all touches of files matching query,
// computing per-turn costs and epoch assignment.
func ExtractFileLineage(entries []jsonl.Entry, compactions []CompactionEvent, query, model string) *SessionLineage {
	lineage := &SessionLineage{}
	turnCosts := computeLineageTurnCosts(entries, model)

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
			path := ExtractToolInputPath(b.Input)
			if path == "" || !MatchesPathSuffix(path, query) {
				continue
			}
			epoch := lineageEpochForEntry(i, compactions)
			cost := turnCosts[i]
			lineage.Touches = append(lineage.Touches, FileTouch{
				EntryIndex: i,
				Timestamp:  e.Timestamp,
				Action:     ClassifyToolAction(b.Name),
				ToolName:   b.Name,
				Epoch:      epoch,
				TurnCost:   cost,
			})
			lineage.TotalCost += cost
		}
	}

	return lineage
}

// MatchesDecisionLabel returns true if query appears in goal or decisions (case-insensitive).
func MatchesDecisionLabel(goal string, decisions []string, query string) bool {
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(goal), q) {
		return true
	}
	for _, d := range decisions {
		if strings.Contains(strings.ToLower(d), q) {
			return true
		}
	}
	return false
}

// MatchesDecisionHint returns true if query appears in any hint (case-insensitive).
func MatchesDecisionHint(hints []string, query string) bool {
	q := strings.ToLower(query)
	for _, h := range hints {
		if strings.Contains(strings.ToLower(h), q) {
			return true
		}
	}
	return false
}

// computeLineageTurnCosts computes the dollar cost for each assistant entry.
func computeLineageTurnCosts(entries []jsonl.Entry, model string) map[int]float64 {
	costs := make(map[int]float64, len(entries)/3)
	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil || e.Message.Usage == nil {
			continue
		}
		u := e.Message.Usage
		m := e.Message.Model
		if m == "" {
			m = model
		}
		costs[i] = QuickCost(u.InputTokens, u.OutputTokens,
			u.CacheCreationInputTokens, u.CacheReadInputTokens, m)
	}
	return costs
}

// lineageEpochForEntry maps an entry index to its epoch number.
// Duplicated from commands/marks.go to avoid circular import.
func lineageEpochForEntry(idx int, compactions []CompactionEvent) int {
	if idx < 0 || len(compactions) == 0 {
		return 0
	}
	epoch := 0
	for _, c := range compactions {
		if idx >= c.LineIndex {
			epoch++
		}
	}
	return epoch
}
