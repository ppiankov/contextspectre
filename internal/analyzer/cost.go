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
	Model            string
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
func CalculateCost(entries []jsonl.Entry) *CostBreakdown {
	cb := &CostBreakdown{}
	var model string

	for _, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil || e.Message.Usage == nil {
			continue
		}
		u := e.Message.Usage
		cb.InputTokens += u.InputTokens
		cb.OutputTokens += u.OutputTokens
		cb.CacheWriteTokens += u.CacheCreationInputTokens
		cb.CacheReadTokens += u.CacheReadInputTokens
		cb.TurnCount++

		if model == "" && e.Message.Model != "" {
			model = e.Message.Model
		}
	}

	if cb.TurnCount == 0 {
		return cb
	}

	cb.Model = model
	pricing := PricingForModel(model)

	cb.InputCost = float64(cb.InputTokens) / 1_000_000 * pricing.InputPerMillion
	cb.OutputCost = float64(cb.OutputTokens) / 1_000_000 * pricing.OutputPerMillion
	cb.CacheWriteCost = float64(cb.CacheWriteTokens) / 1_000_000 * pricing.CacheWritePerMillion
	cb.CacheReadCost = float64(cb.CacheReadTokens) / 1_000_000 * pricing.CacheReadPerMillion
	cb.TotalCost = cb.InputCost + cb.OutputCost + cb.CacheWriteCost + cb.CacheReadCost

	if cb.TurnCount > 0 {
		cb.CostPerTurn = cb.TotalCost / float64(cb.TurnCount)
	}

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
