package analyzer

import (
	"math"
	"testing"
)

func TestCalculateEntropy_WeightedFormula(t *testing.T) {
	in := EntropyInput{
		SignalRatio:     0.95,
		CurrentTokens:   82500, // 0.5 pressure vs 165k threshold
		DriftRatio:      0.10,
		OrphanTokens:    1000,
		TotalTokens:     10000, // 0.1 orphan ratio
		CompactionCount: 2,     // 0.2 compression loss
	}

	got := CalculateEntropy(in)
	want := 19.5 // (0.015 + 0.125 + 0.02 + 0.015 + 0.02) * 100

	if math.Abs(got.Score-want) > 0.0001 {
		t.Fatalf("score = %.4f, want %.4f", got.Score, want)
	}
	if got.Level != EntropyLow {
		t.Fatalf("level = %s, want %s", got.Level, EntropyLow)
	}

	if math.Abs(got.Breakdown.Noise-1.5) > 0.0001 {
		t.Fatalf("noise axis = %.4f, want 1.5", got.Breakdown.Noise)
	}
	if math.Abs(got.Breakdown.CompactionPressure-12.5) > 0.0001 {
		t.Fatalf("pressure axis = %.4f, want 12.5", got.Breakdown.CompactionPressure)
	}
}

func TestCalculateEntropy_CompactionCountCap(t *testing.T) {
	base := EntropyInput{
		SignalRatio:     0.80,
		CurrentTokens:   100000,
		DriftRatio:      0.10,
		OrphanTokens:    100,
		TotalTokens:     10000,
		CompactionCount: 10,
	}

	capped := CalculateEntropy(base)
	overflow := CalculateEntropy(EntropyInput{
		SignalRatio:     base.SignalRatio,
		CurrentTokens:   base.CurrentTokens,
		DriftRatio:      base.DriftRatio,
		OrphanTokens:    base.OrphanTokens,
		TotalTokens:     base.TotalTokens,
		CompactionCount: 99,
	})

	if math.Abs(capped.Score-overflow.Score) > 0.0001 {
		t.Fatalf("score should be capped: %.4f vs %.4f", capped.Score, overflow.Score)
	}
}

func TestCalculateEntropy_Levels(t *testing.T) {
	tests := []struct {
		score float64
		want  EntropyLevel
	}{
		{10, EntropyLow},
		{20, EntropyLow},
		{20.1, EntropyMedium},
		{50, EntropyMedium},
		{60, EntropyHigh},
		{75, EntropyHigh},
		{90, EntropyCritical},
	}

	for _, tt := range tests {
		got := entropyLevel(tt.score)
		if got != tt.want {
			t.Fatalf("entropyLevel(%.1f) = %s, want %s", tt.score, got, tt.want)
		}
	}
}

func TestSignalRatioForGrade(t *testing.T) {
	tests := []struct {
		grade string
		want  float64
	}{
		{"A", 0.95},
		{"B", 0.80},
		{"C", 0.65},
		{"D", 0.45},
		{"F", 0.20},
		{"", 0.20},
	}

	for _, tt := range tests {
		got := SignalRatioForGrade(tt.grade)
		if got != tt.want {
			t.Fatalf("SignalRatioForGrade(%q) = %.2f, want %.2f", tt.grade, got, tt.want)
		}
	}
}
