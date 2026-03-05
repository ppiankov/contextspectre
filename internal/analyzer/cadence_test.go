package analyzer

import "testing"

func TestAssessCleanupCadence_OverdueForImminentCompaction(t *testing.T) {
	stats := &ContextStats{
		CurrentContextTokens: 120000,
		EstimatedTurnsLeft:   6,
		TokenGrowthRate:      1400,
		Model:                "claude-sonnet-4",
		CompactionCount:      1,
		EpochCosts: []EpochCost{
			{TurnCount: 12},
			{TurnCount: 18},
		},
	}
	rec := &CleanupRecommendation{TotalTokens: 7100}

	assess := AssessCleanupCadence(stats, rec)
	if assess == nil {
		t.Fatal("expected assessment")
	}
	if assess.Status != CadenceOverdue {
		t.Fatalf("status = %s, want %s", assess.Status, CadenceOverdue)
	}
	if assess.Reason != "clean now to extend session" {
		t.Fatalf("reason = %q, want clean-now reason", assess.Reason)
	}
	if assess.ProjectedSaveTokens != 42600 {
		t.Fatalf("projected tokens = %d, want 42600", assess.ProjectedSaveTokens)
	}
}

func TestAssessCleanupCadence_DueForNoiseThreshold(t *testing.T) {
	stats := &ContextStats{
		CurrentContextTokens: 40000,
		EstimatedTurnsLeft:   20,
		TokenGrowthRate:      700,
		Model:                "claude-sonnet-4",
	}
	rec := &CleanupRecommendation{TotalTokens: 8000} // 20%

	assess := AssessCleanupCadence(stats, rec)
	if assess == nil {
		t.Fatal("expected assessment")
	}
	if assess.Status != CadenceDue {
		t.Fatalf("status = %s, want %s", assess.Status, CadenceDue)
	}
	if assess.Reason != "cleanup recommended" {
		t.Fatalf("reason = %q, want cleanup recommended", assess.Reason)
	}
}

func TestAssessCleanupCadence_ActionMapping(t *testing.T) {
	stats := &ContextStats{
		CurrentContextTokens: 50000,
		EstimatedTurnsLeft:   15,
		TokenGrowthRate:      900,
		Model:                "claude-sonnet-4",
	}
	rec := &CleanupRecommendation{
		TotalTokens: 9000,
		Items: []CleanupItem{
			{Category: "tangents", TokensSaved: 9000},
		},
	}

	assess := AssessCleanupCadence(stats, rec)
	if assess == nil {
		t.Fatal("expected assessment")
	}
	if assess.RecommendedAction != "split tangent range first" {
		t.Fatalf("action = %q, want split tangent range first", assess.RecommendedAction)
	}
}
