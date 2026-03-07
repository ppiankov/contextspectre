package analyzer

import (
	"sort"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// ReasoningState captures the structural reasoning state at a point in a session.
type ReasoningState struct {
	Epoch      int            `json:"epoch"`
	TurnRange  [2]int         `json:"turn_range"` // [start, end]
	Files      []string       `json:"files"`      // files touched
	ToolCounts map[string]int `json:"tool_counts"`
	Tokens     int            `json:"tokens"`
	WriteFiles []string       `json:"write_files"` // files written/edited
	ReadFiles  []string       `json:"read_files"`  // files read
}

// ReasoningDiff holds the structured comparison between two reasoning states.
type ReasoningDiff struct {
	From         ReasoningState `json:"from"`
	To           ReasoningState `json:"to"`
	FilesAdded   []string       `json:"files_added"`
	FilesDropped []string       `json:"files_dropped"`
	FilesKept    []string       `json:"files_kept"`
	ToolShifts   []ToolShift    `json:"tool_shifts"`
	ScopeChange  string         `json:"scope_change"` // expanded, contracted, shifted, stable
	TokenDelta   int            `json:"token_delta"`
}

// ToolShift describes a change in tool usage pattern between two states.
type ToolShift struct {
	Tool      string `json:"tool"`
	FromCount int    `json:"from_count"`
	ToCount   int    `json:"to_count"`
	Direction string `json:"direction"` // increased, decreased, new, dropped
}

// ExtractEpochState extracts the reasoning state for a specific epoch.
func ExtractEpochState(entries []jsonl.Entry, compactions []CompactionEvent, epochIdx int) *ReasoningState {
	bounds := buildEpochBounds(entries, compactions)
	if epochIdx < 0 || epochIdx >= len(bounds) {
		return nil
	}

	numEpochs := len(bounds)
	eb := bounds[epochIdx]
	state := &ReasoningState{
		Epoch:      epochIdx,
		TurnRange:  [2]int{eb.startTurn, eb.endTurn},
		ToolCounts: make(map[string]int),
	}

	fileSet := make(map[string]bool)
	writeSet := make(map[string]bool)
	readSet := make(map[string]bool)

	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}

		epoch := epochIndexForEntry(i, compactions)
		if epoch >= numEpochs {
			epoch = numEpochs - 1
		}
		if epoch != epochIdx {
			continue
		}

		// Count tokens.
		tokens := 0
		if e.Message.Usage != nil {
			tokens = e.Message.Usage.OutputTokens
		}
		if tokens == 0 {
			tokens = e.RawSize / 4
		}
		state.Tokens += tokens

		// Track tool usage and files.
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" {
				state.ToolCounts[b.Name]++
				path := ExtractToolInputPath(b.Input)
				if path != "" {
					fileSet[path] = true
					action := ClassifyToolAction(b.Name)
					switch action {
					case ActionWrite, ActionEdit:
						writeSet[path] = true
					case ActionRead:
						readSet[path] = true
					}
				}
			}
		}
	}

	state.Files = sortedKeys(fileSet)
	state.WriteFiles = sortedKeys(writeSet)
	state.ReadFiles = sortedKeys(readSet)
	return state
}

// ComputeReasoningDiff compares two reasoning states and produces a structured diff.
func ComputeReasoningDiff(from, to *ReasoningState) *ReasoningDiff {
	diff := &ReasoningDiff{
		From:       *from,
		To:         *to,
		TokenDelta: to.Tokens - from.Tokens,
	}

	fromFiles := toSet(from.Files)
	toFiles := toSet(to.Files)

	for _, f := range to.Files {
		if !fromFiles[f] {
			diff.FilesAdded = append(diff.FilesAdded, f)
		} else {
			diff.FilesKept = append(diff.FilesKept, f)
		}
	}
	for _, f := range from.Files {
		if !toFiles[f] {
			diff.FilesDropped = append(diff.FilesDropped, f)
		}
	}

	// Classify scope change.
	added := len(diff.FilesAdded)
	dropped := len(diff.FilesDropped)
	kept := len(diff.FilesKept)

	switch {
	case added > 0 && dropped == 0:
		diff.ScopeChange = "expanded"
	case dropped > 0 && added == 0:
		diff.ScopeChange = "contracted"
	case added > 0 && dropped > 0:
		if kept == 0 {
			diff.ScopeChange = "shifted"
		} else {
			diff.ScopeChange = "shifted"
		}
	default:
		diff.ScopeChange = "stable"
	}

	// Compute tool shifts.
	allTools := make(map[string]bool)
	for t := range from.ToolCounts {
		allTools[t] = true
	}
	for t := range to.ToolCounts {
		allTools[t] = true
	}
	for t := range allTools {
		fc := from.ToolCounts[t]
		tc := to.ToolCounts[t]
		if fc == tc {
			continue
		}
		var dir string
		switch {
		case fc == 0:
			dir = "new"
		case tc == 0:
			dir = "dropped"
		case tc > fc:
			dir = "increased"
		default:
			dir = "decreased"
		}
		diff.ToolShifts = append(diff.ToolShifts, ToolShift{
			Tool:      t,
			FromCount: fc,
			ToCount:   tc,
			Direction: dir,
		})
	}
	// Sort tool shifts by magnitude of change.
	sort.Slice(diff.ToolShifts, func(i, j int) bool {
		di := abs(diff.ToolShifts[i].ToCount - diff.ToolShifts[i].FromCount)
		dj := abs(diff.ToolShifts[j].ToCount - diff.ToolShifts[j].FromCount)
		return di > dj
	})

	return diff
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
