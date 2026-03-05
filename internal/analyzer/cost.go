package analyzer

import (
	"fmt"
	"strings"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// ModelPricing holds per-million token pricing for a Claude model.
type ModelPricing struct {
	Name                 string
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheWritePerMillion float64
	CacheReadPerMillion  float64
}

// CacheReadCostForTokens computes cache-read dollar cost for a token count.
//
// Claude usage fields expose a single cache_read_input_tokens counter; it does
// not split text vs. vision token classes. ContextSpectre therefore applies the
// same cache-read rate to all cache-read tokens, including image-derived tokens.
func CacheReadCostForTokens(model string, cacheReadTokens int) float64 {
	if cacheReadTokens <= 0 {
		return 0
	}
	pricing := PricingForModel(model)
	return float64(cacheReadTokens) / 1_000_000 * pricing.CacheReadPerMillion
}

// KnownPricing maps model IDs to their pricing. Prices are per million tokens.
var KnownPricing = map[string]ModelPricing{
	"claude-opus-4-6": {
		Name:                 "Opus 4.6",
		InputPerMillion:      15.0,
		OutputPerMillion:     75.0,
		CacheWritePerMillion: 3.75,
		CacheReadPerMillion:  0.75,
	},
	"claude-sonnet-4-6": {
		Name:                 "Sonnet 4.6",
		InputPerMillion:      3.0,
		OutputPerMillion:     15.0,
		CacheWritePerMillion: 0.75,
		CacheReadPerMillion:  0.15,
	},
	"claude-haiku-4-5-20251001": {
		Name:                 "Haiku 4.5",
		InputPerMillion:      0.80,
		OutputPerMillion:     4.0,
		CacheWritePerMillion: 0.20,
		CacheReadPerMillion:  0.04,
	},
}

// DefaultPricing is used when model cannot be determined.
var DefaultPricing = KnownPricing["claude-opus-4-6"]

// CostBreakdown holds itemized cost for a session or epoch.
type CostBreakdown struct {
	InputCost        float64
	OutputCost       float64
	CacheWriteCost   float64
	CacheReadCost    float64
	TotalCost        float64
	InputTokens      int
	OutputTokens     int
	CacheWriteTokens int
	CacheReadTokens  int
	TurnCount        int
	CostPerTurn      float64
	Model            string                    // primary model (highest cost)
	PerModel         map[string]*CostBreakdown // per-model breakdown (nil for sub-entries)
}

// EpochCost holds cost for a single compaction epoch.
type EpochCost struct {
	EpochIndex int
	TurnCount  int
	PeakTokens int
	Cost       CostBreakdown
}

// PricingForModel looks up pricing by model ID. Supports prefix matching
// (e.g. "claude-opus-4-6-20260301" matches "claude-opus-4-6").
// Falls back to DefaultPricing if no match.
func PricingForModel(model string) ModelPricing {
	if model == "" {
		return DefaultPricing
	}
	// Exact match first
	if p, ok := KnownPricing[model]; ok {
		return p
	}
	// Prefix match
	for prefix, p := range KnownPricing {
		if strings.HasPrefix(model, prefix) {
			return p
		}
	}
	return DefaultPricing
}

// CalculateCost computes total session cost from assistant message usage fields.
// Groups by model and applies correct per-model pricing.
func CalculateCost(entries []jsonl.Entry) *CostBreakdown {
	cb := &CostBreakdown{
		PerModel: make(map[string]*CostBreakdown),
	}

	for _, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil || e.Message.Usage == nil {
			continue
		}
		u := e.Message.Usage
		model := e.Message.Model
		if model == "" {
			model = "unknown"
		}

		// Get or create per-model breakdown
		pm, ok := cb.PerModel[model]
		if !ok {
			pm = &CostBreakdown{Model: model}
			cb.PerModel[model] = pm
		}

		pm.InputTokens += u.InputTokens
		pm.OutputTokens += u.OutputTokens
		pm.CacheWriteTokens += u.CacheCreationInputTokens
		pm.CacheReadTokens += u.CacheReadInputTokens
		pm.TurnCount++

		// Accumulate totals
		cb.InputTokens += u.InputTokens
		cb.OutputTokens += u.OutputTokens
		cb.CacheWriteTokens += u.CacheCreationInputTokens
		cb.CacheReadTokens += u.CacheReadInputTokens
		cb.TurnCount++
	}

	if cb.TurnCount == 0 {
		return cb
	}

	// Calculate costs per model with correct pricing
	for model, pm := range cb.PerModel {
		pricing := PricingForModel(model)
		pm.InputCost = float64(pm.InputTokens) / 1_000_000 * pricing.InputPerMillion
		pm.OutputCost = float64(pm.OutputTokens) / 1_000_000 * pricing.OutputPerMillion
		pm.CacheWriteCost = float64(pm.CacheWriteTokens) / 1_000_000 * pricing.CacheWritePerMillion
		pm.CacheReadCost = float64(pm.CacheReadTokens) / 1_000_000 * pricing.CacheReadPerMillion
		pm.TotalCost = pm.InputCost + pm.OutputCost + pm.CacheWriteCost + pm.CacheReadCost
		if pm.TurnCount > 0 {
			pm.CostPerTurn = pm.TotalCost / float64(pm.TurnCount)
		}

		// Aggregate into total
		cb.InputCost += pm.InputCost
		cb.OutputCost += pm.OutputCost
		cb.CacheWriteCost += pm.CacheWriteCost
		cb.CacheReadCost += pm.CacheReadCost
	}

	cb.TotalCost = cb.InputCost + cb.OutputCost + cb.CacheWriteCost + cb.CacheReadCost
	if cb.TurnCount > 0 {
		cb.CostPerTurn = cb.TotalCost / float64(cb.TurnCount)
	}

	// Primary model = highest cost contributor
	var primaryModel string
	var highestCost float64
	for model, pm := range cb.PerModel {
		if pm.TotalCost > highestCost {
			highestCost = pm.TotalCost
			primaryModel = model
		}
	}
	cb.Model = primaryModel

	return cb
}

// CalculateEpochCosts segments entries by compaction boundaries and computes
// cost per epoch. Epoch 0 is pre-first-compaction, epoch N is after Nth compaction.
func CalculateEpochCosts(entries []jsonl.Entry, compactions []CompactionEvent) []EpochCost {
	if len(entries) == 0 {
		return nil
	}

	// Build compaction line indices for boundary detection
	boundaries := make([]int, len(compactions))
	for i, c := range compactions {
		boundaries[i] = c.LineIndex
	}

	// Segment entries into epochs
	epochIdx := 0
	var epochs [][]jsonl.Entry
	var current []jsonl.Entry
	boundaryPos := 0

	for i, e := range entries {
		if boundaryPos < len(boundaries) && i >= boundaries[boundaryPos] {
			epochs = append(epochs, current)
			current = nil
			boundaryPos++
			epochIdx++
		}
		current = append(current, e)
	}
	epochs = append(epochs, current)

	// Calculate cost per epoch
	var result []EpochCost
	for i, epoch := range epochs {
		cost := CalculateCost(epoch)

		// Find peak context tokens in this epoch
		peak := 0
		for _, e := range epoch {
			if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Usage != nil {
				ctx := e.Message.Usage.TotalContextTokens()
				if ctx > peak {
					peak = ctx
				}
			}
		}

		result = append(result, EpochCost{
			EpochIndex: i,
			TurnCount:  cost.TurnCount,
			PeakTokens: peak,
			Cost:       *cost,
		})
	}

	return result
}

// FormatCost formats a dollar cost for display.
func FormatCost(cost float64) string {
	if cost == 0 {
		return "$0.00"
	}
	if cost < 0.01 {
		return "<$0.01"
	}
	if cost >= 1000 {
		whole := int(cost)
		frac := cost - float64(whole)
		return fmt.Sprintf("$%d,%03d.%02d", whole/1000, whole%1000, int(frac*100))
	}
	return fmt.Sprintf("$%.2f", cost)
}

// FormatCostPerTurn formats cost with per-turn breakdown.
func FormatCostPerTurn(total float64, turns int) string {
	if turns == 0 {
		return FormatCost(total)
	}
	return fmt.Sprintf("%s (%s/t)", FormatCost(total), FormatCost(total/float64(turns)))
}

// CostPercent returns the percentage of total that component represents.
func CostPercent(component, total float64) float64 {
	if total == 0 {
		return 0
	}
	return component / total * 100
}

// DecisionEconomics holds CPD, TTC, and CDR metrics for a session.
type DecisionEconomics struct {
	CPD             float64                  // Cost Per Decision: TotalCost / TotalDecisions
	TTC             int                      // Turns To Convergence: TotalTurns / TotalDecisions
	CDR             float64                  // Context Drift Rate: from ScopeDrift.OverallDrift (0-1)
	TotalDecisions  int                      // sum of epoch decision counts
	DecisionDensity float64                  // TotalDecisions / TotalTurns
	HasDecisions    bool                     // false when 0 decisions detected
	PerEpoch        []EpochDecisionEconomics // per-epoch breakdown
}

// EpochDecisionEconomics holds decision economics for a single epoch.
type EpochDecisionEconomics struct {
	EpochIndex int
	CPD        float64
	TTC        int
	CDR        float64
	Decisions  int
	Density    float64
}

// ComputeDecisionEconomics computes CPD/TTC/CDR from analyzed stats and scope drift.
func ComputeDecisionEconomics(stats *ContextStats, drift *ScopeDrift) *DecisionEconomics {
	de := &DecisionEconomics{}

	if stats == nil || stats.Archaeology == nil {
		return de
	}

	// Sum decisions across all epochs
	for _, ev := range stats.Archaeology.Events {
		de.TotalDecisions += ev.Before.DecisionCount
	}

	// Also count decisions in the active (post-last-compaction) epoch
	// Active epoch doesn't appear in Archaeology.Events, so we skip it
	// (decisions there haven't been compacted yet)

	if de.TotalDecisions == 0 {
		return de
	}
	de.HasDecisions = true

	// CPD: total cost / decisions
	if stats.Cost != nil && stats.Cost.TotalCost > 0 {
		de.CPD = stats.Cost.TotalCost / float64(de.TotalDecisions)
	}

	// TTC: total conversational turns / decisions
	if stats.ConversationalTurns > 0 {
		de.TTC = stats.ConversationalTurns / de.TotalDecisions
		de.DecisionDensity = float64(de.TotalDecisions) / float64(stats.ConversationalTurns)
	}

	// CDR: from scope drift
	if drift != nil {
		de.CDR = drift.OverallDrift
	}

	// Per-epoch breakdown
	for i, ev := range stats.Archaeology.Events {
		ede := EpochDecisionEconomics{
			EpochIndex: ev.CompactionIndex,
			Decisions:  ev.Before.DecisionCount,
		}

		if ev.Before.DecisionCount > 0 {
			// Per-epoch CPD
			if i < len(stats.EpochCosts) {
				ede.CPD = stats.EpochCosts[i].Cost.TotalCost / float64(ev.Before.DecisionCount)
			}

			// Per-epoch TTC
			if i < len(stats.EpochCosts) && stats.EpochCosts[i].TurnCount > 0 {
				ede.TTC = stats.EpochCosts[i].TurnCount / ev.Before.DecisionCount
				ede.Density = float64(ev.Before.DecisionCount) / float64(stats.EpochCosts[i].TurnCount)
			}
		}

		// Per-epoch CDR
		if drift != nil && i < len(drift.EpochScopes) {
			ede.CDR = drift.EpochScopes[i].DriftRatio
		}

		de.PerEpoch = append(de.PerEpoch, ede)
	}

	return de
}

// QuickCost calculates an estimated cost from accumulated token counts.
// Used by session browser where full parsing is too slow.
func QuickCost(inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens int, model string) float64 {
	pricing := PricingForModel(model)
	cost := float64(inputTokens) / 1_000_000 * pricing.InputPerMillion
	cost += float64(outputTokens) / 1_000_000 * pricing.OutputPerMillion
	cost += float64(cacheWriteTokens) / 1_000_000 * pricing.CacheWritePerMillion
	cost += float64(cacheReadTokens) / 1_000_000 * pricing.CacheReadPerMillion
	return cost
}
