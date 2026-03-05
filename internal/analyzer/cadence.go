package analyzer

// CadenceStatus is the urgency level for cleanup timing.
type CadenceStatus string

const (
	CadenceClean   CadenceStatus = "clean"
	CadenceDue     CadenceStatus = "due"
	CadenceOverdue CadenceStatus = "overdue"
)

// CadenceAssessment is the computed cleanup timing recommendation.
type CadenceAssessment struct {
	Score                float64
	Status               CadenceStatus
	Reason               string
	RecommendedAction    string
	NoiseTokens          int
	NoiseRatio           float64
	TurnsUntilCompaction int
	TurnsSinceCleanup    int
	ProjectedSaveTokens  int
	ProjectedSaveCost    float64
	PerTurnSaveCost      float64
}

// AssessCleanupCadence computes deterministic cleanup urgency from noise ratio,
// compaction proximity, growth pressure, and turns since last cleanup boundary.
func AssessCleanupCadence(stats *ContextStats, rec *CleanupRecommendation) *CadenceAssessment {
	if stats == nil {
		return nil
	}

	noiseTokens := 0
	if rec != nil {
		noiseTokens = rec.TotalTokens
	}

	noiseRatio := 0.0
	if stats.CurrentContextTokens > 0 && noiseTokens > 0 {
		noiseRatio = float64(noiseTokens) / float64(stats.CurrentContextTokens)
	}

	turnsUntilCompaction := stats.EstimatedTurnsLeft
	if turnsUntilCompaction < 0 {
		turnsUntilCompaction = 0
	}
	turnsSinceCleanup := estimateTurnsSinceCleanup(stats)

	noiseScore := clamp01(noiseRatio/0.30) * 40
	compactionScore := compactionProximityScore(stats.EstimatedTurnsLeft) * 30
	growthScore := clamp01(stats.TokenGrowthRate/2000) * 20
	sinceCleanupScore := clamp01(float64(turnsSinceCleanup)/50) * 10
	score := noiseScore + compactionScore + growthScore + sinceCleanupScore
	if score > 100 {
		score = 100
	}

	status := cadenceStatusFromScore(score)
	reason := "session healthy"

	// Rule-based overrides from work order thresholds.
	switch {
	case noiseRatio > 0.30:
		status = CadenceOverdue
		reason = "cleanup overdue"
	case noiseRatio > 0.15:
		if status == CadenceClean {
			status = CadenceDue
		}
		reason = "cleanup recommended"
	}

	if stats.EstimatedTurnsLeft >= 0 && stats.EstimatedTurnsLeft < 10 && noiseTokens > 5000 {
		status = CadenceOverdue
		reason = "clean now to extend session"
	}
	if turnsSinceCleanup > 50 && noiseTokens > 10000 && status == CadenceClean {
		status = CadenceDue
		reason = "periodic cleanup due"
	}

	action := cadenceAction(status, stats, rec)
	pricing := PricingForModel(stats.Model)
	perTurnSaveCost := float64(noiseTokens) / 1_000_000 * pricing.CacheReadPerMillion
	projectedSaveTokens := noiseTokens * turnsUntilCompaction
	projectedSaveCost := float64(projectedSaveTokens) / 1_000_000 * pricing.CacheReadPerMillion

	return &CadenceAssessment{
		Score:                score,
		Status:               status,
		Reason:               reason,
		RecommendedAction:    action,
		NoiseTokens:          noiseTokens,
		NoiseRatio:           noiseRatio,
		TurnsUntilCompaction: turnsUntilCompaction,
		TurnsSinceCleanup:    turnsSinceCleanup,
		ProjectedSaveTokens:  projectedSaveTokens,
		ProjectedSaveCost:    projectedSaveCost,
		PerTurnSaveCost:      perTurnSaveCost,
	}
}

func cadenceStatusFromScore(score float64) CadenceStatus {
	switch {
	case score >= 70:
		return CadenceOverdue
	case score >= 40:
		return CadenceDue
	default:
		return CadenceClean
	}
}

func compactionProximityScore(turnsLeft int) float64 {
	if turnsLeft < 0 {
		return 0
	}
	if turnsLeft <= 10 {
		return 1
	}
	return clamp01((30 - float64(turnsLeft)) / 20)
}

func estimateTurnsSinceCleanup(stats *ContextStats) int {
	if stats == nil {
		return 0
	}
	if len(stats.EpochCosts) > 0 {
		last := stats.EpochCosts[len(stats.EpochCosts)-1]
		if last.TurnCount > 0 {
			return last.TurnCount
		}
	}
	if stats.ConversationalTurns > 0 {
		return stats.ConversationalTurns
	}
	return 0
}

func cadenceAction(status CadenceStatus, stats *ContextStats, rec *CleanupRecommendation) string {
	if status == CadenceClean {
		return "monitor"
	}

	nearDeadlock := stats != nil &&
		(stats.EstimatedTurnsLeft >= 0 && stats.EstimatedTurnsLeft < 5 ||
			stats.UsagePercent >= 85 || stats.CompactionCount >= 3)
	if nearDeadlock {
		return "export decisions then amputate"
	}

	primary := ""
	if rec != nil && len(rec.Items) > 0 {
		primary = rec.Items[0].Category
	}
	if primary == "tangents" {
		return "split tangent range first"
	}

	return "quick-clean --live"
}
