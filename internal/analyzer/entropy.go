package analyzer

// EntropyLevel is the severity bucket for session entropy.
type EntropyLevel string

const (
	EntropyLow      EntropyLevel = "LOW"
	EntropyMedium   EntropyLevel = "MEDIUM"
	EntropyHigh     EntropyLevel = "HIGH"
	EntropyCritical EntropyLevel = "CRITICAL"
)

const maxCompactionCountEntropy = 10

// EntropyInput holds normalized values required for entropy computation.
type EntropyInput struct {
	SignalRatio     float64 // 0..1
	CurrentTokens   int
	DriftRatio      float64 // 0..1
	OrphanTokens    int
	TotalTokens     int
	CompactionCount int
}

// EntropyBreakdown holds per-axis weighted contribution points (0..100 scale).
type EntropyBreakdown struct {
	Noise              float64
	CompactionPressure float64
	Drift              float64
	Orphans            float64
	CompressionLoss    float64
}

// EntropyScore is the composite score and its per-axis decomposition.
type EntropyScore struct {
	Score     float64
	Level     EntropyLevel
	Breakdown EntropyBreakdown
}

// SignalRatioForGrade maps signal grade to a normalized ratio used by entropy.
func SignalRatioForGrade(grade string) float64 {
	switch grade {
	case "A":
		return 0.95
	case "B":
		return 0.80
	case "C":
		return 0.65
	case "D":
		return 0.45
	default: // F / unknown
		return 0.20
	}
}

// CalculateEntropy computes a deterministic session entropy score from
// economic, reasoning, and structural decay axes.
func CalculateEntropy(in EntropyInput) EntropyScore {
	signalRatio := clamp01(in.SignalRatio)
	driftRatio := clamp01(in.DriftRatio)

	compactionPressure := 0.0
	if in.CurrentTokens > 0 {
		compactionPressure = clamp01(float64(in.CurrentTokens) / float64(CompactionThreshold))
	}

	orphanRatio := 0.0
	if in.TotalTokens > 0 && in.OrphanTokens > 0 {
		orphanRatio = clamp01(float64(in.OrphanTokens) / float64(in.TotalTokens))
	}

	compactionCount := in.CompactionCount
	if compactionCount < 0 {
		compactionCount = 0
	}
	if compactionCount > maxCompactionCountEntropy {
		compactionCount = maxCompactionCountEntropy
	}
	compressionLoss := float64(compactionCount) / float64(maxCompactionCountEntropy)

	noiseAxis := (1 - signalRatio) * 0.30
	pressureAxis := compactionPressure * 0.25
	driftAxis := driftRatio * 0.20
	orphanAxis := orphanRatio * 0.15
	compressionAxis := compressionLoss * 0.10

	score := clamp01(noiseAxis+pressureAxis+driftAxis+orphanAxis+compressionAxis) * 100
	breakdown := EntropyBreakdown{
		Noise:              noiseAxis * 100,
		CompactionPressure: pressureAxis * 100,
		Drift:              driftAxis * 100,
		Orphans:            orphanAxis * 100,
		CompressionLoss:    compressionAxis * 100,
	}

	return EntropyScore{
		Score:     score,
		Level:     entropyLevel(score),
		Breakdown: breakdown,
	}
}

func entropyLevel(score float64) EntropyLevel {
	switch {
	case score <= 20:
		return EntropyLow
	case score <= 50:
		return EntropyMedium
	case score <= 75:
		return EntropyHigh
	default:
		return EntropyCritical
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
