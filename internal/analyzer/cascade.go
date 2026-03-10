package analyzer

import (
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// CascadeDeleteSet expands an initial deletion set to include all entries
// that become orphaned or chain-broken as a result of the initial deletions.
// It operates entirely in memory — no I/O. Returns the fully resolved set.
func CascadeDeleteSet(entries []jsonl.Entry, initial map[int]bool, isKept func(string) bool) map[int]bool {
	if len(initial) == 0 {
		return initial
	}

	resolved := make(map[int]bool, len(initial))
	for idx := range initial {
		resolved[idx] = true
	}

	// Build reverse parent map: childrenOf[uuid] = indices whose parentUUID == uuid.
	childrenOf := make(map[string][]int, len(entries))
	for i, e := range entries {
		if e.ParentUUID != "" {
			childrenOf[e.ParentUUID] = append(childrenOf[e.ParentUUID], i)
		}
	}

	// Build tool_result ownership: toolResultOwner[toolUseID] = user entry index.
	toolResultOwner := make(map[string]int, 128)
	for i, e := range entries {
		if e.Type != jsonl.TypeUser || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID != "" {
				toolResultOwner[b.ToolUseID] = i
			}
		}
	}

	// BFS worklist expansion.
	worklist := make([]int, 0, len(initial))
	for idx := range initial {
		worklist = append(worklist, idx)
	}

	markDeleted := func(idx int) {
		if idx < 0 || idx >= len(entries) || resolved[idx] {
			return
		}
		if isKept(entries[idx].UUID) {
			return
		}
		resolved[idx] = true
		worklist = append(worklist, idx)
	}

	for len(worklist) > 0 {
		idx := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		e := entries[idx]

		// Rule 1: deleting an assistant entry orphans its tool_results.
		if e.Type == jsonl.TypeAssistant {
			for _, toolID := range e.ToolUseIDs() {
				if resultIdx, ok := toolResultOwner[toolID]; ok {
					markDeleted(resultIdx)
				}
			}
		}

		// Rule 2: deleting any entry chain-breaks its children.
		if e.UUID != "" {
			for _, childIdx := range childrenOf[e.UUID] {
				markDeleted(childIdx)
			}
		}
	}

	return resolved
}
