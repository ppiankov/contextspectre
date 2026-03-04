package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/savings"
	"github.com/spf13/cobra"
)

var (
	statusLinePath   string
	statusLineStdin  bool
	statusLineFormat string
)

var statusLineCmd = &cobra.Command{
	Use:   "status-line",
	Short: "Fast-path telemetry for status line integration",
	Long: `Outputs session telemetry optimized for Claude Code's status line hook.
Accepts the transcript path directly (--path) or via stdin JSON (--stdin).
Uses mtime-based caching for sub-millisecond repeat calls.`,
	RunE: runStatusLine,
}

func init() {
	statusLineCmd.Flags().StringVar(&statusLinePath, "path", "", "Direct path to transcript JSONL file")
	statusLineCmd.Flags().BoolVar(&statusLineStdin, "stdin", false, "Read JSON from stdin (Claude Code hook format)")
	statusLineCmd.Flags().StringVar(&statusLineFormat, "format", "tab", "Output format: tab, shell, human, json")
	rootCmd.AddCommand(statusLineCmd)
}

// statusLineData holds computed telemetry for a single session.
type statusLineData struct {
	ContextPercent float64 `json:"context_percent"`
	TurnsRemaining int     `json:"turns_remaining"`
	NoiseTokens    int     `json:"noise_tokens"`
	Grade          string  `json:"grade"`
	Cost           float64 `json:"cost"`
	SavedCost      float64 `json:"saved_cost"`
	Model          string  `json:"model"`
	VectorState    string  `json:"vector_state,omitempty"`
	VectorAction   string  `json:"vector_action,omitempty"`
}

// statusLineCache is the on-disk cache structure.
type statusLineCache struct {
	FileMtime int64 `json:"file_mtime"`
	statusLineData
}

func runStatusLine(_ *cobra.Command, _ []string) error {
	path, err := resolveStatusLinePath()
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	// Check cache.
	data, cached := loadStatusLineCache(sessionID, info.ModTime().UnixNano())
	if !cached {
		data, err = computeStatusLine(path, sessionID)
		if err != nil {
			return err
		}
		writeStatusLineCache(sessionID, info.ModTime().UnixNano(), data)
	}

	tryExpertClean(path)

	return formatStatusLine(data)
}

func resolveStatusLinePath() (string, error) {
	if statusLineStdin {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		var input struct {
			TranscriptPath string `json:"transcript_path"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return "", fmt.Errorf("parse stdin JSON: %w", err)
		}
		if input.TranscriptPath == "" {
			return "", fmt.Errorf("stdin JSON missing transcript_path field")
		}
		return input.TranscriptPath, nil
	}

	if statusLinePath == "" {
		return "", fmt.Errorf("--path or --stdin required")
	}
	return statusLinePath, nil
}

func computeStatusLine(path, sessionID string) (*statusLineData, error) {
	stats, err := jsonl.ScanLight(path)
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	currentTokens := 0
	if stats.LastUsage != nil {
		currentTokens = stats.LastUsage.TotalContextTokens()
	}

	contextPct := 0.0
	if currentTokens > 0 {
		contextPct = float64(currentTokens) / float64(analyzer.ContextWindowSize) * 100
	}

	grade := analyzer.GradeFromSignalPercent(stats.SignalPercent)

	noiseTokens := 0
	if currentTokens > 0 && stats.SignalPercent < 100 {
		noiseTokens = currentTokens * (100 - stats.SignalPercent) / 100
	}

	// Turns remaining — epoch growth rate (same as WO-078 logic).
	assistantTurns := stats.EpochAssistantCount
	if assistantTurns == 0 {
		assistantTurns = stats.AssistantCount
	}
	turnsRemaining := 0
	if assistantTurns > 0 && currentTokens > 0 {
		avgPerTurn := currentTokens / assistantTurns
		if avgPerTurn > 0 {
			remaining := analyzer.CompactionThreshold - currentTokens
			if remaining > 0 {
				turnsRemaining = remaining / avgPerTurn
			}
		}
	}

	cost := analyzer.QuickCost(stats.TotalInputTokens, stats.TotalOutputTokens,
		stats.TotalCacheWriteTokens, stats.TotalCacheReadTokens, stats.Model)

	// Savings for this session.
	savedCost := 0.0
	events, _ := savings.Load(resolveClaudeDir())
	if events != nil {
		if summary := savings.ForSession(events, sessionID); summary != nil {
			savedCost = summary.TotalSavedCost
		}
	}

	modelShort := ""
	if stats.Model != "" {
		modelShort = analyzer.PricingForModel(stats.Model).Name
	}

	// Fast-path vector gauge (context-only — no full parse)
	vectorState := ""
	vectorAction := ""
	if contextPct > 92 {
		vectorState = "emergency"
		vectorAction = "amputate"
	} else if contextPct > 75 {
		vectorState = "degrading"
		vectorAction = "clean"
	}

	return &statusLineData{
		ContextPercent: contextPct,
		TurnsRemaining: turnsRemaining,
		NoiseTokens:    noiseTokens,
		Grade:          grade,
		Cost:           cost,
		SavedCost:      savedCost,
		Model:          modelShort,
		VectorState:    vectorState,
		VectorAction:   vectorAction,
	}, nil
}

func formatStatusLine(d *statusLineData) error {
	switch statusLineFormat {
	case "json":
		return printJSON(d)
	case "shell":
		fmt.Printf("CTX=%.1f; TURNS=%d; NOISE=%d; GRADE=%s; COST=%.2f; SAVED=%.2f; MODEL=%s; VECTOR=%s; VACTION=%s\n",
			d.ContextPercent, d.TurnsRemaining, d.NoiseTokens,
			d.Grade, d.Cost, d.SavedCost, d.Model, d.VectorState, d.VectorAction)
	case "human":
		parts := []string{
			fmt.Sprintf("ctx:%.0f%%", d.ContextPercent),
			fmt.Sprintf("turns:%d", d.TurnsRemaining),
			fmt.Sprintf("noise:%s", formatTokens(d.NoiseTokens)),
			d.Grade,
			analyzer.FormatCost(d.Cost),
		}
		if d.SavedCost > 0 {
			parts = append(parts, fmt.Sprintf("saved:%s", analyzer.FormatCost(d.SavedCost)))
		}
		if d.VectorState != "" {
			parts = append(parts, fmt.Sprintf("%s:%s", d.VectorState, d.VectorAction))
		}
		fmt.Println(strings.Join(parts, " | "))
	default: // tab
		fmt.Printf("ctx=%.1f\tturns=%d\tnoise=%d\tgrade=%s\tcost=%.2f\tsaved=%.2f\tmodel=%s\tvector=%s\tvaction=%s\n",
			d.ContextPercent, d.TurnsRemaining, d.NoiseTokens,
			d.Grade, d.Cost, d.SavedCost, d.Model, d.VectorState, d.VectorAction)
	}
	return nil
}

// Cache helpers.

func statusLineCachePath(sessionID string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("contextspectre-sl-%s.json", sessionID))
}

func loadStatusLineCache(sessionID string, fileMtime int64) (*statusLineData, bool) {
	data, err := os.ReadFile(statusLineCachePath(sessionID))
	if err != nil {
		return nil, false
	}
	var cache statusLineCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	if cache.FileMtime != fileMtime {
		return nil, false
	}
	return &cache.statusLineData, true
}

func writeStatusLineCache(sessionID string, fileMtime int64, d *statusLineData) {
	cache := statusLineCache{
		FileMtime:      fileMtime,
		statusLineData: *d,
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	// Atomic write: temp + rename.
	tmp := statusLineCachePath(sessionID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, statusLineCachePath(sessionID))
}
