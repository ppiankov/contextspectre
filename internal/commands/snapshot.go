package commands

import (
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analytics"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/savings"
	"github.com/ppiankov/contextspectre/internal/session"
)

// recordAnalyticsSnapshot builds and records a session analytics snapshot.
// Silently returns on any error — analytics are best-effort.
func recordAnalyticsSnapshot(path string) {
	entries, err := jsonl.Parse(path)
	if err != nil {
		slog.Debug("Analytics snapshot skipped", "error", err)
		return
	}

	stats := analyzer.Analyze(entries)
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	// Health/signal grade.
	dupResult := analyzer.FindDuplicateReads(entries)
	retryResult := analyzer.FindFailedRetries(entries)
	tangentResult := analyzer.FindTangents(entries)
	rec := analyzer.Recommend(entries, stats, dupResult, retryResult, tangentResult, nil)
	health := analyzer.ComputeHealth(stats, rec)

	duration := sessionDuration(entries)

	// Extract project name from path: .../projects/<encoded-dir>/<session>.jsonl
	projectDir := filepath.Dir(path)
	projectName := session.ProjectNameFromDir(projectDir)

	// Get slug from ScanLight (fast).
	slug := ""
	if lightStats, err := jsonl.ScanLight(path); err == nil {
		slug = lightStats.Slug
	}

	snap := analytics.Snapshot{
		Timestamp:     time.Now(),
		SessionID:     sessionID,
		Project:       projectName,
		Slug:          slug,
		Client:        stats.ClientType,
		DurationHours: duration.Hours(),
		Turns:         stats.ConversationalTurns,
		Compactions:   stats.CompactionCount,
		ContextPct:    stats.UsagePercent,
	}

	if health != nil && health.TotalTokens > 0 {
		snap.SignalGrade = health.Grade
		snap.SignalPct = health.SignalPercent
		snap.NoiseTokens = health.NoiseTokens
		if health.BiggestOffender != "" {
			snap.TopNoise = health.BiggestOffender
		}
	}

	if stats.Cost != nil {
		snap.CostTotal = stats.Cost.TotalCost
		snap.CostPerTurn = stats.Cost.CostPerTurn
		if duration > 0 {
			snap.CostVelocityHr = stats.Cost.TotalCost / duration.Hours()
		}
		snap.ModelPrimary = stats.Cost.Model

		if stats.Cost.PerModel != nil {
			models := make(map[string]int)
			for model, bd := range stats.Cost.PerModel {
				models[model] = bd.TurnCount
			}
			snap.Models = models
		}
	}

	// Savings data for this session.
	dir := resolveClaudeDir()
	savingsEvents, _ := savings.Load(dir)
	if savingsEvents != nil {
		sessionSavings := savings.ForSession(savingsEvents, sessionID)
		if sessionSavings != nil {
			snap.CleanupTokensSaved = sessionSavings.TotalRemoved
			snap.CleanupCount = sessionSavings.TotalCleanups
		}
	}

	if err := analytics.Append(dir, snap); err != nil {
		slog.Debug("Analytics snapshot failed", "error", err)
	}
}
