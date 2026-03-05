package analyzer

import (
	"sort"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// SidechainReason is a structural reason an entry is considered sidechain noise.
type SidechainReason string

const (
	SidechainFlagged      SidechainReason = "flagged_sidechain"
	SidechainMissingParent SidechainReason = "missing_parent"
	SidechainOrphanResult  SidechainReason = "orphaned_tool_result"
)

// SidechainEntry describes a single sidechain entry.
type SidechainEntry struct {
	EntryIndex      int
	LineNumber      int
	UUID            string
	ParentUUID      string
	ToolUseID       string
	TokenCost       int
	Preview         string
	Reasons         []SidechainReason
	Classification  string // "repairable" or "prune-only"
	ReconnectParent string // nearest surviving ancestor UUID (if repairable)
}

// SidechainReport summarizes sidechain detections for a session.
type SidechainReport struct {
	Entries         []SidechainEntry
	TotalEntries    int
	TotalTokens     int
	GroupCount      int
	RepairableCount int
	PruneOnlyCount  int
}

type sidechainAccum struct {
	entry            SidechainEntry
	reasonSet        map[SidechainReason]bool
	hasMissingParent bool
	hasOrphanResult  bool
}

// DetectSidechains finds structurally orphaned entries:
// - entries explicitly marked isSidechain
// - entries whose parentUuid points to a missing UUID
// - tool_result blocks that reference missing tool_use IDs
func DetectSidechains(entries []jsonl.Entry) *SidechainReport {
	report := &SidechainReport{}
	if len(entries) == 0 {
		return report
	}

	uuidSet := make(map[string]bool, len(entries))
	for i := range entries {
		if entries[i].UUID != "" {
			uuidSet[entries[i].UUID] = true
		}
	}

	toolUseIDs := make(map[string]bool)
	for i := range entries {
		for _, id := range entries[i].ToolUseIDs() {
			toolUseIDs[id] = true
		}
	}

	accByIndex := make(map[int]*sidechainAccum)
	for i := range entries {
		e := entries[i]
		if e.IsSidechain {
			sc := getOrCreateSidechain(accByIndex, entries, i)
			sc.reasonSet[SidechainFlagged] = true
		}

		if e.ParentUUID != "" && !uuidSet[e.ParentUUID] {
			sc := getOrCreateSidechain(accByIndex, entries, i)
			sc.reasonSet[SidechainMissingParent] = true
			sc.hasMissingParent = true
		}

		if e.Type != jsonl.TypeUser || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID != "" && !toolUseIDs[b.ToolUseID] {
				sc := getOrCreateSidechain(accByIndex, entries, i)
				sc.reasonSet[SidechainOrphanResult] = true
				sc.hasOrphanResult = true
				if sc.entry.ToolUseID == "" {
					sc.entry.ToolUseID = b.ToolUseID
				}
			}
		}
	}

	indices := make([]int, 0, len(accByIndex))
	for idx := range accByIndex {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	prevIdx := -2
	for _, idx := range indices {
		sc := accByIndex[idx]
		reasons := make([]SidechainReason, 0, len(sc.reasonSet))
		for reason := range sc.reasonSet {
			reasons = append(reasons, reason)
		}
		sort.Slice(reasons, func(i, j int) bool { return reasons[i] < reasons[j] })
		sc.entry.Reasons = reasons

		reconnect := nearestReconnectParent(entries, idx, uuidSet)
		sc.entry.ReconnectParent = reconnect
		if sc.hasOrphanResult || reconnect == "" {
			sc.entry.Classification = "prune-only"
			report.PruneOnlyCount++
		} else {
			sc.entry.Classification = "repairable"
			report.RepairableCount++
		}

		report.TotalTokens += sc.entry.TokenCost
		report.Entries = append(report.Entries, sc.entry)
		if idx != prevIdx+1 {
			report.GroupCount++
		}
		prevIdx = idx
	}

	report.TotalEntries = len(report.Entries)
	return report
}

// SidechainIndexSet converts sidechain report entries into a deletion index set.
func SidechainIndexSet(report *SidechainReport) map[int]bool {
	set := make(map[int]bool)
	if report == nil {
		return set
	}
	for _, e := range report.Entries {
		set[e.EntryIndex] = true
	}
	return set
}

func getOrCreateSidechain(acc map[int]*sidechainAccum, entries []jsonl.Entry, idx int) *sidechainAccum {
	if existing, ok := acc[idx]; ok {
		return existing
	}
	e := entries[idx]
	sc := &sidechainAccum{
		entry: SidechainEntry{
			EntryIndex: idx,
			LineNumber: e.LineNumber,
			UUID:       e.UUID,
			ParentUUID: e.ParentUUID,
			TokenCost:  e.RawSize / 4,
			Preview:    e.ContentPreview(80),
		},
		reasonSet: make(map[SidechainReason]bool),
	}
	acc[idx] = sc
	return sc
}

func nearestReconnectParent(entries []jsonl.Entry, idx int, uuidSet map[string]bool) string {
	for i := idx - 1; i >= 0; i-- {
		if entries[i].UUID == "" {
			continue
		}
		parent := entries[i].ParentUUID
		if parent == "" || uuidSet[parent] {
			return entries[i].UUID
		}
	}
	return ""
}
