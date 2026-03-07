package analyzer

import (
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// HalfLifeResult holds the reasoning half-life analysis for a session.
type HalfLifeResult struct {
	HalfLife       int             `json:"half_life"`        // turns until 50% of tokens are dead
	DeadContextPct float64         `json:"dead_context_pct"` // % of tokens dead at session end
	TotalTurns     int             `json:"total_turns"`
	TotalTokens    int             `json:"total_tokens"`
	Epochs         []HalfLifeEpoch `json:"epochs"`
}

// HalfLifeEpoch describes one reasoning epoch and its decay contribution.
type HalfLifeEpoch struct {
	Index       int     `json:"index"`
	StartTurn   int     `json:"start_turn"`
	EndTurn     int     `json:"end_turn"`
	Turns       int     `json:"turns"`
	Tokens      int     `json:"tokens"`
	DeadPct     float64 `json:"dead_pct"`     // estimated % of this epoch's tokens dead at session end
	FileTouches int     `json:"file_touches"` // unique files touched in this epoch
	FileDecay   float64 `json:"file_decay"`   // fraction of files never touched again after this epoch
}

// ComputeHalfLife estimates the reasoning half-life of a session.
// Uses proxy signals: file touch decay across epochs, compaction boundaries.
func ComputeHalfLife(entries []jsonl.Entry, compactions []CompactionEvent) *HalfLifeResult {
	turns := countAssistantTurns(entries)
	if turns < 2 {
		return &HalfLifeResult{TotalTurns: turns}
	}

	// Build epoch boundaries from compactions.
	epochBounds := buildEpochBounds(entries, compactions)
	numEpochs := len(epochBounds)

	// Collect file touches per epoch.
	epochFiles := make([]map[string]bool, numEpochs)
	epochTokens := make([]int, numEpochs)
	for i := range epochBounds {
		epochFiles[i] = make(map[string]bool)
	}

	totalTokens := 0
	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}

		epoch := epochIndexForEntry(i, compactions)
		if epoch >= numEpochs {
			epoch = numEpochs - 1
		}

		// Count tokens for this turn.
		tokens := 0
		if e.Message.Usage != nil {
			tokens = e.Message.Usage.OutputTokens
		}
		if tokens == 0 {
			tokens = e.RawSize / 4
		}
		epochTokens[epoch] += tokens
		totalTokens += tokens

		// Track file touches.
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" {
				path := ExtractToolInputPath(b.Input)
				if path != "" {
					epochFiles[epoch][path] = true
				}
			}
		}
	}

	// Compute file decay per epoch: what fraction of files touched in this epoch
	// are never touched again in any later epoch.
	lastEpoch := numEpochs - 1

	epochs := make([]HalfLifeEpoch, numEpochs)
	// Build set of files touched after each epoch.
	afterFiles := make([]map[string]bool, len(epochBounds))
	for i := range epochBounds {
		afterFiles[i] = make(map[string]bool)
	}
	for i := lastEpoch; i >= 1; i-- {
		for f := range epochFiles[i] {
			// All previous epochs get this file marked as "touched after".
			for j := 0; j < i; j++ {
				afterFiles[j][f] = true
			}
		}
	}

	for i, eb := range epochBounds {
		touched := len(epochFiles[i])
		decay := 0.0
		if i < lastEpoch && touched > 0 {
			// Files touched in this epoch but never again.
			abandoned := 0
			for f := range epochFiles[i] {
				if !afterFiles[i][f] {
					abandoned++
				}
			}
			decay = float64(abandoned) / float64(touched)
		}

		// Estimate dead tokens: early epochs with high file decay → more dead.
		deadPct := 0.0
		if i < lastEpoch {
			// Heuristic: dead% = file_decay weighted by epoch age.
			age := float64(lastEpoch-i) / float64(lastEpoch)
			deadPct = decay * age * 100
			if deadPct > 100 {
				deadPct = 100
			}
		}

		epochs[i] = HalfLifeEpoch{
			Index:       i,
			StartTurn:   eb.startTurn,
			EndTurn:     eb.endTurn,
			Turns:       eb.endTurn - eb.startTurn,
			Tokens:      epochTokens[i],
			DeadPct:     deadPct,
			FileTouches: touched,
			FileDecay:   decay,
		}
	}

	// Compute overall dead context % and half-life.
	totalDead := 0
	for i, ep := range epochs {
		dead := int(float64(epochTokens[i]) * ep.DeadPct / 100)
		totalDead += dead
	}

	deadPct := 0.0
	if totalTokens > 0 {
		deadPct = float64(totalDead) / float64(totalTokens) * 100
	}

	// Half-life: find the turn at which cumulative dead tokens reach 50%.
	halfTarget := totalTokens / 2
	cumulative := 0
	halfLife := turns // default: full session length if never reaches 50%
	for i, ep := range epochs {
		dead := int(float64(epochTokens[i]) * ep.DeadPct / 100)
		cumulative += dead
		if cumulative >= halfTarget {
			halfLife = ep.EndTurn - ep.StartTurn
			if halfLife < 1 {
				halfLife = 1
			}
			break
		}
	}

	return &HalfLifeResult{
		HalfLife:       halfLife,
		DeadContextPct: deadPct,
		TotalTurns:     turns,
		TotalTokens:    totalTokens,
		Epochs:         epochs,
	}
}

// epochBound tracks the turn range of an epoch.
type epochBound struct {
	startTurn int
	endTurn   int
}

// buildEpochBounds creates epoch boundaries from compaction events.
func buildEpochBounds(entries []jsonl.Entry, compactions []CompactionEvent) []epochBound {
	if len(compactions) == 0 {
		turns := countAssistantTurns(entries)
		return []epochBound{{startTurn: 0, endTurn: turns}}
	}

	// Map compaction LineIndex to assistant turn numbers.
	var bounds []epochBound
	turnsSoFar := 0
	lastStart := 0

	for i, e := range entries {
		if e.Type == jsonl.TypeAssistant {
			turnsSoFar++
		}
		for _, c := range compactions {
			if c.LineIndex == i {
				bounds = append(bounds, epochBound{startTurn: lastStart, endTurn: turnsSoFar})
				lastStart = turnsSoFar
			}
		}
	}
	// Final epoch.
	bounds = append(bounds, epochBound{startTurn: lastStart, endTurn: turnsSoFar})

	return bounds
}

// epochIndexForEntry maps an entry index to its epoch index using compaction boundaries.
func epochIndexForEntry(idx int, compactions []CompactionEvent) int {
	if len(compactions) == 0 {
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

func countAssistantTurns(entries []jsonl.Entry) int {
	count := 0
	for _, e := range entries {
		if e.Type == jsonl.TypeAssistant {
			count++
		}
	}
	return count
}
