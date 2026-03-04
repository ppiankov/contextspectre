package commands

import (
	"fmt"
	"log/slog"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/project"
)

// tryExpertClean checks config and conditions, runs tiers 1-3 if warranted.
// Silently returns if expert mode is off, conditions not met, or nothing to clean.
func tryExpertClean(path string) {
	cfg, err := project.Load(resolveClaudeDir())
	if err != nil || !cfg.ExpertMode {
		return
	}

	stats, err := jsonl.ScanLight(path)
	if err != nil || stats.LastUsage == nil {
		return
	}

	currentTokens := stats.LastUsage.TotalContextTokens()
	if currentTokens == 0 {
		return
	}

	contextPct := float64(currentTokens) / float64(analyzer.ContextWindowSize) * 100

	// Turns remaining — epoch growth rate.
	assistantTurns := stats.EpochAssistantCount
	if assistantTurns == 0 {
		assistantTurns = stats.AssistantCount
	}
	turnsRemaining := 0
	if assistantTurns > 0 {
		avgPerTurn := currentTokens / assistantTurns
		if avgPerTurn > 0 {
			remaining := analyzer.CompactionThreshold - currentTokens
			if remaining > 0 {
				turnsRemaining = remaining / avgPerTurn
			}
		}
	}

	// Only auto-clean when conditions are met: context >80% or turns <5.
	if contextPct < 80 && turnsRemaining >= 5 {
		return
	}

	result, err := editor.CleanLive(path, editor.CleanLiveOpts{Tier3: true})
	if err != nil {
		slog.Debug("Expert clean skipped", "error", err)
		return
	}

	if result.TotalTokensSaved > 0 {
		fmt.Printf("[Expert] Auto-cleaned: %d prog, %d snap, %d stale, %d retry. Saved ~%d tokens.\n",
			result.ProgressRemoved, result.SnapshotsRemoved,
			result.StaleReadsRemoved, result.FailedRetries,
			result.TotalTokensSaved)
		recordCleanupSavings(path, result.TotalTokensSaved)
	}
}
