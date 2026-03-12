package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
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
	statusLinePPID   int
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
	statusLineCmd.Flags().IntVar(&statusLinePPID, "ppid", 0, "Write ctx cache file for the provided parent PID")
	rootCmd.AddCommand(statusLineCmd)
}

// statusLineData holds computed telemetry for a single session.
type statusLineData struct {
	SessionID       string  `json:"session_id"`
	Grade           string  `json:"grade"`
	ContextPercent  float64 `json:"context_percent"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	Cost            float64 `json:"cost"`
	Model           string  `json:"model"`
	Project         string  `json:"project,omitempty"`
	TurnsRemaining  int     `json:"turns_remaining"`
	NoiseTokens     int     `json:"noise_tokens"`
	NoiseMultiplier float64 `json:"noise_multiplier"`
	SavedCost       float64 `json:"saved_cost"`
	VectorState     string  `json:"vector_state,omitempty"`
	VectorAction    string  `json:"vector_action,omitempty"`
	ChainHealthy    bool    `json:"chain_healthy"`
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
	if err := writeContextPercentCache(statusLinePPID, data.ContextPercent); err != nil {
		return err
	}

	tryExpertClean(path)
	tryAutoCheckpoint(path)

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
	noiseMultiplier := 0.0
	if currentTokens > 0 && stats.SignalPercent < 100 {
		noiseTokens = currentTokens * (100 - stats.SignalPercent) / 100
		if currentTokens > 0 {
			noiseMultiplier = float64(noiseTokens) / float64(currentTokens)
		}
	}

	// Turns remaining — epoch growth rate.
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

	shortID := sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	// Token flow from last turn
	inputTokens := 0
	outputTokens := 0
	if stats.LastUsage != nil {
		inputTokens = stats.LastUsage.InputTokens
		outputTokens = stats.LastUsage.OutputTokens
	}

	// Project name from session path
	project := extractProjectFromPath(path)

	return &statusLineData{
		SessionID:       shortID,
		Grade:           grade,
		ContextPercent:  contextPct,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		Cost:            cost,
		Model:           modelShort,
		Project:         project,
		TurnsRemaining:  turnsRemaining,
		NoiseTokens:     noiseTokens,
		NoiseMultiplier: noiseMultiplier,
		SavedCost:       savedCost,
		VectorState:     vectorState,
		VectorAction:    vectorAction,
		ChainHealthy:    stats.ChainHealthy,
	}, nil
}

// extractProjectFromPath gets the project directory name from a session path.
func extractProjectFromPath(path string) string {
	// Path format: ~/.claude/projects/-Users-foo-bar-project/session.jsonl
	dir := filepath.Dir(path)
	encoded := filepath.Base(dir)
	// Decode: replace leading dash, split by dash, take last segment
	parts := strings.Split(encoded, "-")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func formatStatusLine(d *statusLineData) error {
	switch statusLineFormat {
	case "json":
		return printJSON(d)
	case "shell":
		chainOK := 1
		if !d.ChainHealthy {
			chainOK = 0
		}
		fmt.Printf("GRADE=%s; CTX=%.1f; IN=%d; OUT=%d; COST=%.2f; MODEL=%s; PROJECT=%s; SID=%s; TURNS=%d; NOISE=%d; NM=%.2f; SAVED=%.2f; VECTOR=%s; VACTION=%s; CHAIN=%d\n",
			d.Grade, d.ContextPercent, d.InputTokens, d.OutputTokens,
			d.Cost, d.Model, d.Project, d.SessionID,
			d.TurnsRemaining, d.NoiseTokens, d.NoiseMultiplier, d.SavedCost,
			d.VectorState, d.VectorAction, chainOK)
	case "human":
		// Signal-first layout: [A] ctx:82% +11525/-2666 | $139.08 | Opus 4.6 | project
		tokenFlow := fmt.Sprintf("+%d/-%d", d.InputTokens, d.OutputTokens)
		parts := []string{
			fmt.Sprintf("[%s]", d.Grade),
			fmt.Sprintf("ctx:%.0f%% %s", d.ContextPercent, tokenFlow),
			analyzer.FormatCost(d.Cost),
		}
		if d.Model != "" {
			parts = append(parts, d.Model)
		}
		if d.Project != "" {
			parts = append(parts, d.Project)
		}
		if !d.ChainHealthy {
			parts = append(parts, "⚠")
		}
		fmt.Println(strings.Join(parts, " | "))
	default: // tab
		chainVal := "ok"
		if !d.ChainHealthy {
			chainVal = "broken"
		}
		fmt.Printf("grade=%s\tctx=%.1f\tin=%d\tout=%d\tcost=%.2f\tmodel=%s\tproject=%s\tsid=%s\tturns=%d\tnoise=%d\tnm=%.2f\tsaved=%.2f\tvector=%s\tvaction=%s\tchain=%s\n",
			d.Grade, d.ContextPercent, d.InputTokens, d.OutputTokens,
			d.Cost, d.Model, d.Project, d.SessionID,
			d.TurnsRemaining, d.NoiseTokens, d.NoiseMultiplier, d.SavedCost,
			d.VectorState, d.VectorAction, chainVal)
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

func contextPercentCachePath(ppid int) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("claude-ctx-%d", ppid))
}

func writeContextPercentCache(ppid int, contextPercent float64) error {
	if ppid <= 0 {
		return nil
	}
	value := int(math.Round(contextPercent))
	data := []byte(strconv.Itoa(value))
	path := contextPercentCachePath(ppid)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write ctx cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename ctx cache: %w", err)
	}
	return nil
}
