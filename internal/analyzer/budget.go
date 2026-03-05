package analyzer

import "sort"

// BudgetRiskLevel is the combined session+budget risk level.
type BudgetRiskLevel string

const (
	BudgetRiskLow      BudgetRiskLevel = "low"
	BudgetRiskMedium   BudgetRiskLevel = "medium"
	BudgetRiskHigh     BudgetRiskLevel = "high"
	BudgetRiskCritical BudgetRiskLevel = "critical"
)

// BudgetAction is one ranked budget-protection action.
type BudgetAction struct {
	Action           string
	EstimatedSavings float64
}

// BudgetAssessment combines compaction pressure, noise, and weekly budget.
type BudgetAssessment struct {
	RiskLevel                 BudgetRiskLevel
	WeeklyLimit               float64
	WeeklySpent               float64
	WeeklyRemaining           float64
	WeeklyRemainingPercent    float64
	TurnsUntilCompaction      int
	NoiseTokens               int
	NoiseRatio                float64
	EstimatedCostToCompaction float64
	ExpectedTurnsGained       int
	ExpectedDelayMinutes      int
	RecommendedAction         string
	RecommendedSavings        float64
	OffloadHint               bool
	Actions                   []BudgetAction
}

// AssessBudgetRisk ranks budget-protection actions using context pressure,
// noise recoverability, and configured weekly budget constraints.
func AssessBudgetRisk(stats *ContextStats, rec *CleanupRecommendation, drift *ScopeDrift, weeklyLimit, weeklySpent float64) *BudgetAssessment {
	if stats == nil || weeklyLimit <= 0 {
		return nil
	}

	remaining := weeklyLimit - weeklySpent
	if remaining < 0 {
		remaining = 0
	}
	remainingPct := (remaining / weeklyLimit) * 100

	noiseTokens := 0
	expectedTurns := 0
	if rec != nil {
		noiseTokens = rec.TotalTokens
		expectedTurns = rec.TotalTurnsGained
	}
	noiseRatio := 0.0
	if stats.CurrentContextTokens > 0 && noiseTokens > 0 {
		noiseRatio = float64(noiseTokens) / float64(stats.CurrentContextTokens)
	}

	turnsLeft := stats.EstimatedTurnsLeft
	if turnsLeft < 0 {
		turnsLeft = 0
	}
	costPerTurn := 0.0
	if stats.Cost != nil && stats.Cost.CostPerTurn > 0 {
		costPerTurn = stats.Cost.CostPerTurn
	}
	estimatedCostToCompaction := costPerTurn * float64(turnsLeft)

	pricing := PricingForModel(stats.Model)
	cleanSavings := float64(noiseTokens*turnsLeft) / 1_000_000 * pricing.CacheReadPerMillion

	tangentSavings := 0.0
	if drift != nil {
		for _, ts := range drift.TangentSeqs {
			tangentSavings += ts.DollarCost
		}
	}

	restartSavings := estimatedCostToCompaction * 0.40
	offloadSavings := estimatedCostToCompaction * 0.25

	actions := []BudgetAction{
		{Action: "clean noise now", EstimatedSavings: cleanSavings},
		{Action: "split tangent range", EstimatedSavings: tangentSavings},
		{Action: "start new session with branch export", EstimatedSavings: restartSavings},
		{Action: "offload mechanical work to cheaper agents or CI", EstimatedSavings: offloadSavings},
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].EstimatedSavings > actions[j].EstimatedSavings
	})

	risk := budgetRiskLevel(turnsLeft, noiseRatio, remainingPct)
	offloadHint := remainingPct < 20

	recAction := actions[0].Action
	recSavings := actions[0].EstimatedSavings
	if recSavings <= 0 {
		recAction = "monitor"
	}

	return &BudgetAssessment{
		RiskLevel:                 risk,
		WeeklyLimit:               weeklyLimit,
		WeeklySpent:               weeklySpent,
		WeeklyRemaining:           remaining,
		WeeklyRemainingPercent:    remainingPct,
		TurnsUntilCompaction:      turnsLeft,
		NoiseTokens:               noiseTokens,
		NoiseRatio:                noiseRatio,
		EstimatedCostToCompaction: estimatedCostToCompaction,
		ExpectedTurnsGained:       expectedTurns,
		ExpectedDelayMinutes:      expectedTurns * 2,
		RecommendedAction:         recAction,
		RecommendedSavings:        recSavings,
		OffloadHint:               offloadHint,
		Actions:                   actions,
	}
}

func budgetRiskLevel(turnsLeft int, noiseRatio, remainingPct float64) BudgetRiskLevel {
	if remainingPct < 10 && turnsLeft > 0 && turnsLeft < 10 {
		return BudgetRiskCritical
	}

	elevated := 0
	if turnsLeft > 0 && turnsLeft < 10 {
		elevated++
	}
	if noiseRatio > 0.15 {
		elevated++
	}
	if remainingPct < 20 {
		elevated++
	}

	switch {
	case elevated >= 2:
		return BudgetRiskHigh
	case elevated == 1:
		return BudgetRiskMedium
	default:
		return BudgetRiskLow
	}
}
