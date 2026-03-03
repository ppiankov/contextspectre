package analyzer

import "sort"

// CleanupItem represents a single category of cleanable content.
type CleanupItem struct {
	Category    string // "progress", "snapshots", "stale_reads", "images", "large_outputs", "failed_retries", "sidechains", "tangents"
	Label       string // human-readable: "Progress messages"
	Count       int    // items affected
	TokensSaved int    // estimated tokens recoverable
	TurnsGained int    // TokensSaved / TokenGrowthRate
}

// CleanupRecommendation aggregates all cleanable categories into a ranked list.
type CleanupRecommendation struct {
	Items            []CleanupItem // sorted by TokensSaved descending
	TotalTokens      int
	TotalTurnsGained int
	CurrentPercent   float64
	ProjectedPercent float64 // context usage after cleanup
}

// Recommend builds a ranked cleanup recommendation from existing analysis data.
func Recommend(stats *ContextStats, dupResult *DuplicateReadResult, retryResult *RetryResult, tangentResult *TangentResult) *CleanupRecommendation {
	var items []CleanupItem

	// Progress messages
	if stats.ProgressCount > 0 {
		items = append(items, CleanupItem{
			Category:    "progress",
			Label:       "Progress messages",
			Count:       stats.ProgressCount,
			TokensSaved: stats.ProgressTokens,
		})
	}

	// File-history snapshots
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

	// Images
	if stats.ImageCount > 0 {
		items = append(items, CleanupItem{
			Category:    "images",
			Label:       "Images",
			Count:       stats.ImageCount,
			TokensSaved: int(stats.ImageBytesTotal / 750),
		})
	}

	// Large Bash outputs
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

	rec := &CleanupRecommendation{
		Items:          items,
		CurrentPercent: stats.UsagePercent,
	}

	for _, item := range items {
		rec.TotalTokens += item.TokensSaved
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
