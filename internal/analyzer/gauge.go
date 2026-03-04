package analyzer

// VectorState represents the session health state.
type VectorState string

const (
	VectorHealthy   VectorState = "healthy"
	VectorDegrading VectorState = "degrading"
	VectorUnstable  VectorState = "unstable"
	VectorEmergency VectorState = "emergency"
)

// VectorAction is the recommended action for the current state.
type VectorAction string

const (
	ActionContinue VectorAction = "continue"
	ActionClean    VectorAction = "clean --all"
	ActionSplit    VectorAction = "split or distill"
	ActionAmputate VectorAction = "amputate"
)

// GaugeThresholds holds configurable thresholds for gauge scoring.
type GaugeThresholds struct {
	ContextWarn float64 // percent (default 75)
	CPDWarn     float64 // dollars (default 15)
	TTCWarn     int     // turns (default 90)
	CDRWarn     float64 // ratio 0-1 (default 0.35)
}

// DefaultGaugeThresholds are factory defaults from empirical observation.
var DefaultGaugeThresholds = GaugeThresholds{
	ContextWarn: 75.0,
	CPDWarn:     15.0,
	TTCWarn:     90,
	CDRWarn:     0.35,
}

// VectorGauge holds the computed session health vector.
type VectorGauge struct {
	State          VectorState
	Action         VectorAction
	ContextPct     float64
	CPD            float64
	TTC            int
	CDR            float64
	Score          int
	PostCompaction bool
}

// ComputeGauge evaluates session health from context stats and decision economics.
// Score-based: each metric over threshold adds 1. Override: context >92% = emergency.
// Greenwashing guards prevent artificially healthy readings.
func ComputeGauge(stats *ContextStats, decEcon *DecisionEconomics, t GaugeThresholds) *VectorGauge {
	g := &VectorGauge{}
	if stats == nil {
		return g
	}

	g.ContextPct = stats.UsagePercent

	if decEcon != nil && decEcon.HasDecisions {
		g.CPD = decEcon.CPD
		g.TTC = decEcon.TTC
		g.CDR = decEcon.CDR
	}

	// Post-compaction: compacted but no decisions detected yet
	if stats.CompactionCount > 0 && (decEcon == nil || !decEcon.HasDecisions) {
		g.PostCompaction = true
	}

	// Score accumulation
	score := 0
	if g.ContextPct > t.ContextWarn {
		score++
	}
	if decEcon != nil && decEcon.HasDecisions {
		if g.CPD > t.CPDWarn {
			score++
		}
		if g.TTC > t.TTCWarn {
			score++
		}
		if g.CDR > t.CDRWarn {
			score++
		}
	}

	// Override: context >92% always emergency
	if g.ContextPct > 92 {
		score = 4
	}

	// Greenwashing guard 1: low decision density forces unstable
	// (density < 0.05 = fewer than 1 decision per 20 turns)
	if decEcon != nil && decEcon.HasDecisions && decEcon.DecisionDensity < 0.05 && score < 2 {
		score = 2
	}

	// Greenwashing guard 2: high CDR forces unstable
	if decEcon != nil && g.CDR > t.CDRWarn && score < 2 {
		score = 2
	}

	// Greenwashing guard 3: post-compaction cannot return to healthy
	if g.PostCompaction && score < 1 {
		score = 1
	}

	g.Score = score

	switch {
	case score >= 3:
		g.State = VectorEmergency
		g.Action = ActionAmputate
	case score == 2:
		g.State = VectorUnstable
		g.Action = ActionSplit
	case score == 1:
		g.State = VectorDegrading
		g.Action = ActionClean
	default:
		g.State = VectorHealthy
		g.Action = ActionContinue
	}

	return g
}
