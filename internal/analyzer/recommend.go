package analyzer

import (
	"sort"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// CleanupItem represents a single category of cleanable content.
type CleanupItem struct {
	Category    string // "progress", "snapshots", "stale_reads", "images", "large_outputs", "failed_retries", "sidechains", "tangents"
	Label       string // human-readable: "Progress messages"
	Count       int    // items affected
	TokensSaved int    // estimated tokens recoverable (per-category, may overlap with other categories)
	TurnsGained int    // TokensSaved / TokenGrowthRate
}

// CleanupRecommendation aggregates all cleanable categories into a ranked list.
type CleanupRecommendation struct {
	Items            []CleanupItem // sorted by TokensSaved descending
	TotalTokens      int           // union-deduped total (entries counted once even if in multiple categories)
	TotalTurnsGained int
	OverlapTokens    int // sum(per-category) - TotalTokens; nonzero means entries were in multiple categories
	CurrentPercent   float64
	ProjectedPercent float64 // context usage after cleanup
}

// NoiseLedger tracks per-entry category membership for union-based deduplication.
type NoiseLedger struct {
	PerCategory   map[string]int // category -> sum of tokens (pre-union, for display)
	OverlapTokens int            // sum(PerCategory) - UnionTokens
	UnionTokens   int            // tokens counted from union(all entry indices)
}

// BuildNoiseLedger computes union-deduplicated noise totals from the four indexed
// noise categories (stale_reads, failed_retries, tangents, sidechains).
// An entry that appears in multiple categories is counted once in the union.
func BuildNoiseLedger(
	entries []jsonl.Entry,
	dupResult *DuplicateReadResult,
	retryResult *RetryResult,
	tangentResult *TangentResult,
	sidechainReport *SidechainReport,
) *NoiseLedger {
	ledger := &NoiseLedger{PerCategory: make(map[string]int)}
	seenUnion := make(map[int]int) // entryIdx -> rawTokens

	tag := func(idx int, cat string) {
		if idx < 0 || idx >= len(entries) {
			return
		}
		rawTokens := entries[idx].RawSize / 4
		ledger.PerCategory[cat] += rawTokens
		if _, ok := seenUnion[idx]; !ok {
			seenUnion[idx] = rawTokens
		}
	}

	if dupResult != nil {
		for _, g := range dupResult.Groups {
			for _, sr := range g.StaleReads {
				tag(sr.AssistantIdx, "stale_reads")
				if sr.ResultIdx >= 0 {
					tag(sr.ResultIdx, "stale_reads")
				}
			}
		}
	}

	if retryResult != nil {
		for _, s := range retryResult.Sequences {
			tag(s.FailedToolUseIdx, "failed_retries")
			if s.FailedResultIdx >= 0 {
				tag(s.FailedResultIdx, "failed_retries")
			}
		}
	}

	if tangentResult != nil {
		for _, g := range tangentResult.Groups {
			for _, idx := range g.EntryIndices {
				tag(idx, "tangents")
			}
		}
	}

	if sidechainReport != nil {
		for _, e := range sidechainReport.Entries {
			tag(e.EntryIndex, "sidechains")
		}
	}

	for _, rawTokens := range seenUnion {
		ledger.UnionTokens += rawTokens
	}

	catSum := 0
	for _, v := range ledger.PerCategory {
		catSum += v
	}
	ledger.OverlapTokens = catSum - ledger.UnionTokens

	return ledger
}

// Recommend builds a ranked cleanup recommendation from existing analysis data.
// The four indexed categories (stale_reads, failed_retries, tangents, sidechains)
// are union-deduplicated so entries counted in multiple categories don't inflate totals.
func Recommend(
	entries []jsonl.Entry,
	stats *ContextStats,
	dupResult *DuplicateReadResult,
	retryResult *RetryResult,
	tangentResult *TangentResult,
	sidechainReport *SidechainReport,
) *CleanupRecommendation {
	var items []CleanupItem

	// Progress messages (not entry-indexed — no overlap with indexed categories)
	if stats.ProgressCount > 0 {
		items = append(items, CleanupItem{
			Category:    "progress",
			Label:       "Progress messages",
			Count:       stats.ProgressCount,
			TokensSaved: stats.ProgressTokens,
		})
	}

	// File-history snapshots (not entry-indexed)
	if stats.SnapshotCount > 0 {
		items = append(items, CleanupItem{
			Category:    "snapshots",
			Label:       "File snapshots",
			Count:       stats.SnapshotCount,
			TokensSaved: int(stats.SnapshotBytesTotal / 4),
		})
	}

	// Stale duplicate reads
	if dupResult != nil && dupResult.TotalStale > 0 {
		items = append(items, CleanupItem{
			Category:    "stale_reads",
			Label:       "Stale reads",
			Count:       dupResult.TotalStale,
			TokensSaved: dupResult.TotalTokens,
		})
	}

	// Images (not entry-indexed)
	if stats.ImageCount > 0 {
		items = append(items, CleanupItem{
			Category:    "images",
			Label:       "Images",
			Count:       stats.ImageCount,
			TokensSaved: int(stats.ImageBytesTotal / 750),
		})
	}

	// Large Bash outputs (not entry-indexed)
	if stats.LargeOutputCount > 0 {
		items = append(items, CleanupItem{
			Category:    "large_outputs",
			Label:       "Large outputs",
			Count:       stats.LargeOutputCount,
			TokensSaved: stats.LargeOutputTokens,
		})
	}

	// Failed retries
	if retryResult != nil && retryResult.TotalFailed > 0 {
		items = append(items, CleanupItem{
			Category:    "failed_retries",
			Label:       "Failed retries",
			Count:       retryResult.TotalFailed,
			TokensSaved: retryResult.TotalTokens,
		})
	}

	// Sidechains
	if stats.SidechainCount > 0 {
		items = append(items, CleanupItem{
			Category:    "sidechains",
			Label:       "Sidechains",
			Count:       stats.SidechainCount,
			TokensSaved: stats.SidechainTokens,
		})
	}

	// Tangents
	if tangentResult != nil && tangentResult.TotalEntries > 0 {
		items = append(items, CleanupItem{
			Category:    "tangents",
			Label:       "Tangents",
			Count:       tangentResult.TotalEntries,
			TokensSaved: tangentResult.TotalTokens,
		})
	}

	if len(items) == 0 {
		return nil
	}

	// Calculate turns gained per item
	if stats.TokenGrowthRate > 0 {
		for i := range items {
			items[i].TurnsGained = int(float64(items[i].TokensSaved) / stats.TokenGrowthRate)
		}
	}

	// Sort by TokensSaved descending
	sort.Slice(items, func(i, j int) bool {
		return items[i].TokensSaved > items[j].TokensSaved
	})

	// Build noise ledger for union deduplication of indexed categories.
	ledger := BuildNoiseLedger(entries, dupResult, retryResult, tangentResult, sidechainReport)

	// Non-indexed category tokens (progress, snapshots, images, large_outputs)
	// These are different entry types and don't overlap with indexed categories.
	nonIndexedTokens := 0
	for _, item := range items {
		switch item.Category {
		case "progress", "snapshots", "images", "large_outputs":
			nonIndexedTokens += item.TokensSaved
		}
	}

	rec := &CleanupRecommendation{
		Items:          items,
		TotalTokens:    ledger.UnionTokens + nonIndexedTokens,
		OverlapTokens:  ledger.OverlapTokens,
		CurrentPercent: stats.UsagePercent,
	}

	for _, item := range items {
		rec.TotalTurnsGained += item.TurnsGained
	}

	// Projected usage after cleanup
	if ContextWindowSize > 0 {
		projected := stats.CurrentContextTokens - rec.TotalTokens
		if projected < 0 {
			projected = 0
		}
		rec.ProjectedPercent = float64(projected) / float64(ContextWindowSize) * 100
	}

	return rec
}
