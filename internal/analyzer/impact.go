package analyzer

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// DeletionImpact predicts the effects of deleting selected messages.
type DeletionImpact struct {
	SelectedCount        int
	ProgressAutoRemoved  int
	EstimatedTokenSaved  int
	NewContextPercent    float64
	PredictedTurnsGained int
	ChainRepairs         int
	Warnings             []string
}

// PredictImpact analyzes the impact of deleting selected entries.
func PredictImpact(entries []jsonl.Entry, selected map[int]bool, stats *ContextStats) *DeletionImpact {
	impact := &DeletionImpact{}

	// Auto-select progress messages linked to selected tool_use
	expanded := AutoSelectProgress(entries, selected)

	// Count selections
	for idx := range expanded {
		if expanded[idx] {
			impact.SelectedCount++
			if idx < len(entries) && entries[idx].Type == jsonl.TypeProgress {
				if !selected[idx] {
					impact.ProgressAutoRemoved++
				}
			}
		}
	}

	// Estimate token savings
	for idx := range expanded {
		if !expanded[idx] || idx >= len(entries) {
			continue
		}
		impact.EstimatedTokenSaved += EstimateTokens(&entries[idx])
	}

	// Calculate new context percentage
	newTokens := stats.CurrentContextTokens - impact.EstimatedTokenSaved
	if newTokens < 0 {
		newTokens = 0
	}
	impact.NewContextPercent = float64(newTokens) / float64(ContextWindowSize) * 100

	// Predicted turns gained
	if stats.TokenGrowthRate > 0 {
		impact.PredictedTurnsGained = int(float64(impact.EstimatedTokenSaved) / stats.TokenGrowthRate)
	}

	// Count chain repairs needed
	deletedUUIDs := make(map[string]bool)
	for idx := range expanded {
		if expanded[idx] && idx < len(entries) && entries[idx].UUID != "" {
			deletedUUIDs[entries[idx].UUID] = true
		}
	}
	for _, e := range entries {
		if !deletedUUIDs[e.UUID] && deletedUUIDs[e.ParentUUID] {
			impact.ChainRepairs++
		}
	}

	// Warnings
	if stats.LastCompactionLine > 0 {
		preCompactionCount := 0
		for idx := range expanded {
			if expanded[idx] && idx < stats.LastCompactionLine {
				preCompactionCount++
			}
		}
		if preCompactionCount > 0 {
			impact.Warnings = append(impact.Warnings,
				fmt.Sprintf("%d selected messages are from before the last compaction — "+
					"already excluded from active context, deletion saves file size only", preCompactionCount))
		}
	}

	return impact
}

// AutoSelectProgress expands a selection to include progress messages
// linked to selected assistant messages via toolUseID.
func AutoSelectProgress(entries []jsonl.Entry, selected map[int]bool) map[int]bool {
	// Collect tool_use IDs from selected assistant messages
	selectedToolIDs := make(map[string]bool)
	for idx := range selected {
		if !selected[idx] || idx >= len(entries) {
			continue
		}
		for _, id := range entries[idx].ToolUseIDs() {
			selectedToolIDs[id] = true
		}
	}

	// Copy original selection
	result := make(map[int]bool, len(selected))
	for k, v := range selected {
		result[k] = v
	}

	// Auto-select linked progress messages
	if len(selectedToolIDs) > 0 {
		for i, e := range entries {
			if e.Type == jsonl.TypeProgress {
				if selectedToolIDs[e.ToolUseID] || selectedToolIDs[e.ParentToolUseID] {
					result[i] = true
				}
			}
		}
	}

	return result
}
