package analyzer

import "testing"

func TestAssessBudgetRisk_Critical(t *testing.T) {
	stats := &ContextStats{
		CurrentContextTokens: 120000,
		EstimatedTurnsLeft:   6,
		Model:                "claude-sonnet-4-6",
		Cost: &CostBreakdown{
			CostPerTurn: 0.10,
		},
	}
	rec := &CleanupRecommendation{
		TotalTokens:      11000,
		TotalTurnsGained: 14,
	}

	out := AssessBudgetRisk(stats, rec, nil, 100, 93)
	if out == nil {
		t.Fatal("expected assessment")
	}
	if out.RiskLevel != BudgetRiskCritical {
		t.Fatalf("risk = %s, want %s", out.RiskLevel, BudgetRiskCritical)
	}
	if out.WeeklyRemaining != 7 {
		t.Fatalf("weekly remaining = %.2f, want 7", out.WeeklyRemaining)
	}
	if out.RecommendedAction == "" {
		t.Fatal("expected recommended action")
	}
	if len(out.Actions) == 0 {
		t.Fatal("expected ranked actions")
	}
}
