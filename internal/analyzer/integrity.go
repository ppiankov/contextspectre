package analyzer

import (
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// IntegrityIssueKind classifies chain integrity problems.
type IntegrityIssueKind string

const (
	IntegrityMissingParent    IntegrityIssueKind = "missing_parent"
	IntegrityBadChainStart    IntegrityIssueKind = "bad_chain_start"
	IntegrityChainUnreachable IntegrityIssueKind = "chain_unreachable"
)

// IntegrityIssue describes a single chain integrity problem.
type IntegrityIssue struct {
	Kind       IntegrityIssueKind `json:"kind"`
	EntryIndex int                `json:"entry_index"`
	LineNumber int                `json:"line_number"`
	Detail     string             `json:"detail"`
}

// IntegrityReport holds the results of a chain integrity check.
type IntegrityReport struct {
	Healthy        bool             `json:"healthy"`
	ActiveChainLen int              `json:"active_chain_length"`
	Issues         []IntegrityIssue `json:"issues,omitempty"`
	// BrokenAtIndex is the entry index where the active chain breaks.
	// -1 if chain is healthy or no entries.
	BrokenAtIndex int `json:"broken_at_index"`
}

// CheckIntegrity walks the active parent chain from the last entry and
// detects structural problems that would prevent session resume.
func CheckIntegrity(entries []jsonl.Entry) *IntegrityReport {
	report := &IntegrityReport{
		Healthy:       true,
		BrokenAtIndex: -1,
	}

	if len(entries) == 0 {
		return report
	}

	// Build UUID → entry index map.
	uuidIndex := make(map[string]int, len(entries))
	for i, e := range entries {
		if e.UUID != "" {
			uuidIndex[e.UUID] = i
		}
	}

	// Find the last entry with a UUID (skip queue-operations etc.).
	lastIdx := -1
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].UUID != "" {
			lastIdx = i
			break
		}
	}
	if lastIdx < 0 {
		return report
	}

	// Walk the active parent chain backward.
	chain := make([]int, 0, 64) // entry indices in the active chain
	visited := make(map[string]bool)
	current := lastIdx

	for current >= 0 {
		e := entries[current]
		if e.UUID != "" {
			if visited[e.UUID] {
				break // cycle protection
			}
			visited[e.UUID] = true
		}
		chain = append(chain, current)

		parent := e.ParentUUID
		if parent == "" {
			break // root of chain
		}
		parentIdx, exists := uuidIndex[parent]
		if !exists {
			// Broken chain — parent UUID doesn't exist in the file.
			report.Healthy = false
			report.BrokenAtIndex = current
			report.Issues = append(report.Issues, IntegrityIssue{
				Kind:       IntegrityMissingParent,
				EntryIndex: current,
				LineNumber: entries[current].LineNumber,
				Detail:     "parent UUID not found: " + parent[:minLen(len(parent), 12)],
			})
			break
		}
		current = parentIdx
	}

	report.ActiveChainLen = len(chain)

	// Reverse chain to oldest-first order for validation.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}

	// Check 1: Active chain starts with the right message type.
	// API requires first message to be system or user, not assistant.
	// Report ALL consecutive assistant entries at the start so they are
	// removed in a single repair pass (prevents infinite peel-one-at-a-time loop).
	if len(chain) > 0 && entries[chain[0]].Type == jsonl.TypeAssistant {
		report.Healthy = false
		for k := 0; k < len(chain); k++ {
			if entries[chain[k]].Type != jsonl.TypeAssistant {
				break
			}
			report.Issues = append(report.Issues, IntegrityIssue{
				Kind:       IntegrityBadChainStart,
				EntryIndex: chain[k],
				LineNumber: entries[chain[k]].LineNumber,
				Detail:     "active chain starts with assistant message (API requires user or system first)",
			})
		}
	}

	return report
}

// CheckIntegrityLight performs a fast integrity check using only UUIDs and parent
// references — no content parsing. Suitable for watch mode.
func CheckIntegrityLight(entries []jsonl.Entry) bool {
	if len(entries) == 0 {
		return true
	}

	// Build UUID set.
	uuids := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.UUID != "" {
			uuids[e.UUID] = true
		}
	}

	// Find last entry with UUID.
	lastIdx := -1
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].UUID != "" {
			lastIdx = i
			break
		}
	}
	if lastIdx < 0 {
		return true
	}

	// Walk 5 hops — enough to detect immediate breaks.
	current := lastIdx
	visited := make(map[string]bool)
	for hops := 0; hops < 5; hops++ {
		e := entries[current]
		if e.UUID != "" {
			if visited[e.UUID] {
				return false // cycle
			}
			visited[e.UUID] = true
		}
		parent := e.ParentUUID
		if parent == "" {
			break // root
		}
		if !uuids[parent] {
			return false // broken chain
		}
		// Find parent index.
		found := false
		for i, pe := range entries {
			if pe.UUID == parent {
				current = i
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
