package commands

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// checkpointContextThreshold is the context percentage at which auto-checkpoint fires.
const checkpointContextThreshold = 70.0

// tryAutoCheckpoint fires a checkpoint when context pressure crosses 70%.
// Silently returns if conditions not met.
func tryAutoCheckpoint(path string) {
	stats, err := jsonl.ScanLight(path)
	if err != nil {
		return
	}
	tryAutoCheckpointWithStats(path, stats)
}

// tryAutoCheckpointWithStats is like tryAutoCheckpoint but accepts pre-computed ScanLight stats.
func tryAutoCheckpointWithStats(path string, stats *jsonl.LightStats) {
	if stats.LastUsage == nil {
		return
	}

	currentTokens := stats.LastUsage.TotalContextTokens()
	if currentTokens == 0 {
		return
	}

	contextPct := float64(currentTokens) / float64(analyzer.ContextWindowSize) * 100
	if contextPct < checkpointContextThreshold {
		return
	}

	// Throttle: check if we already checkpointed this epoch.
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	epochKey := fmt.Sprintf("%s-%d", sessionID, stats.CompactionCount)
	if alreadyCheckpointed(epochKey) {
		return
	}

	// Use CWD from pre-computed stats (avoids extra ScanLight call).
	projectDir := stats.CWD
	if projectDir == "" {
		slog.Debug("Auto-checkpoint: no project dir detected")
		return
	}

	outputPath := filepath.Join(projectDir, "docs", "context.txt")

	// Ensure docs/ directory exists.
	docsDir := filepath.Dir(outputPath)
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(docsDir, 0o755); err != nil {
			slog.Debug("Auto-checkpoint: cannot create docs/", "error", err)
			return
		}
	}

	// Run checkpoint.
	entries, err := jsonl.Parse(path)
	if err != nil {
		slog.Debug("Auto-checkpoint: parse failed", "error", err)
		return
	}

	fullStats := analyzer.Analyze(entries)

	activeHint := ""
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Type == jsonl.TypeUser && entries[i].Message != nil {
			activeHint = entries[i].ContentPreview(60)
			break
		}
	}
	epochs := analyzer.BuildEpochs(fullStats.EpochCosts, fullStats.Archaeology, activeHint)

	activeStart := 0
	if fullStats.LastCompactionLine > 0 {
		activeStart = fullStats.LastCompactionLine
	}
	activeEntries := entries[activeStart:]
	epochData := extractCheckpointData(activeEntries)

	var activeEpoch CheckpointEpoch
	if len(epochs) > 0 {
		last := epochs[len(epochs)-1]
		activeEpoch = CheckpointEpoch{
			Index:      last.Index,
			TurnCount:  last.TurnCount,
			PeakTokens: last.PeakTokens,
			Cost:       last.Cost,
			Topic:      last.Topic,
		}
	}

	output := CheckpointOutput{
		SessionID:      sessionID,
		Project:        extractProjectFromPath(path),
		ClientType:     fullStats.ClientType,
		ContextPercent: fullStats.UsagePercent,
		TurnsRemaining: fullStats.EstimatedTurnsLeft,
		Epoch:          activeEpoch,
		Decisions:      epochData.decisions,
		Findings:       epochData.findings,
		Questions:      epochData.questions,
		Files:          epochData.files,
	}

	brief := renderCheckpointBrief(output)

	if err := os.WriteFile(outputPath, []byte(brief), 0o644); err != nil {
		slog.Debug("Auto-checkpoint: write failed", "error", err)
		return
	}

	markCheckpointed(epochKey)
	fmt.Printf("[Checkpoint] ctx:%.0f%% — %d decisions, %d findings → %s\n",
		contextPct, len(output.Decisions), len(output.Findings), outputPath)
}

// Checkpoint throttle: simple file-based marker in /tmp.

func checkpointThrottlePath(epochKey string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("contextspectre-ckpt-%s", epochKey))
}

func alreadyCheckpointed(epochKey string) bool {
	_, err := os.Stat(checkpointThrottlePath(epochKey))
	return err == nil
}

func markCheckpointed(epochKey string) {
	_ = os.WriteFile(checkpointThrottlePath(epochKey), []byte(strconv.Itoa(os.Getpid())), 0o600)
}
