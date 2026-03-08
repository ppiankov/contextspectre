package analyzer

import "math"

// SearchSpaceMetrics estimates the design search space traversed by a session.
// This is an experimental metric — approximate, not mathematically pure.
type SearchSpaceMetrics struct {
	EstimatedPathProbes      float64 `json:"estimated_path_probes"`
	ExplorationToCommitRatio float64 `json:"exploration_to_commit_ratio"`
	BranchFactor             float64 `json:"branch_factor"`
	ReexplorationMultiplier  float64 `json:"reexploration_multiplier"`
	BranchLabel              string  `json:"branch_label"`
	ReexplorationLabel       string  `json:"reexploration_label"`
	Decisions                int     `json:"decisions"`
}

// SearchSpaceInput provides the signals needed to estimate search space.
type SearchSpaceInput struct {
	Decisions        int
	CompactionCount  int
	SidechainGroups  int
	TangentEntries   int
	DistinctTools    int
	IntegrityBroken  bool
	GhostFiles       int
	NoiseTokens      int
	MaxContextTokens int
	StaleReads       int
}

// ComputeSearchSpace estimates the design paths explored by the session.
// Returns nil if the session has no detected decisions.
func ComputeSearchSpace(in SearchSpaceInput) *SearchSpaceMetrics {
	if in.Decisions == 0 {
		return nil
	}

	bf := estimateBranchFactor(in)
	rem := estimateReexploration(in)

	probes := float64(in.Decisions) * bf * rem
	ratio := bf * rem

	return &SearchSpaceMetrics{
		EstimatedPathProbes:      math.Round(probes*10) / 10,
		ExplorationToCommitRatio: math.Round(ratio*10) / 10,
		BranchFactor:             math.Round(bf*10) / 10,
		ReexplorationMultiplier:  math.Round(rem*10) / 10,
		BranchLabel:              branchLabel(bf),
		ReexplorationLabel:       reexplorationLabel(rem),
		Decisions:                in.Decisions,
	}
}

func estimateBranchFactor(in SearchSpaceInput) float64 {
	d := float64(max(1, in.Decisions))

	// Compaction-created branches: each compaction forces re-exploration of directions.
	splitsPerDecision := float64(in.CompactionCount) / d
	splitWeight := math.Min(1.5, splitsPerDecision*0.8)

	// Sidechains indicate speculative branching.
	sidechainGroupsPerDecision := float64(in.SidechainGroups) / d
	sidechainWeight := math.Min(1.5, sidechainGroupsPerDecision*0.4)

	// Tangents indicate cross-repo exploration.
	tangentsPerDecision := float64(in.TangentEntries) / d
	tangentWeight := math.Min(1.0, tangentsPerDecision*0.08)

	// Tool diversity indicates breadth of exploration approaches.
	toolDiversityWeight := 0.0
	if in.DistinctTools > 3 {
		toolDiversityWeight = math.Min(1.2, float64(in.DistinctTools-3)*0.12)
	}

	bf := 1.8 + splitWeight + sidechainWeight + tangentWeight + toolDiversityWeight
	return clamp(bf, 2.0, 8.0)
}

func estimateReexploration(in SearchSpaceInput) float64 {
	d := float64(max(1, in.Decisions))
	maxCtx := float64(max(1, in.MaxContextTokens))

	compactionWeight := math.Min(1.2, float64(in.CompactionCount)*0.07)

	integrityWeight := 0.0
	if in.IntegrityBroken {
		integrityWeight = 0.35
	}

	ghostWeight := math.Min(0.5, float64(in.GhostFiles)/d*0.08)
	noiseWeight := math.Min(0.6, float64(in.NoiseTokens)/maxCtx*0.5)
	staleReadWeight := math.Min(0.35, float64(in.StaleReads)/d*0.1)

	rem := 1.0 + compactionWeight + integrityWeight + ghostWeight + noiseWeight + staleReadWeight
	return clamp(rem, 1.0, 3.5)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func branchLabel(bf float64) string {
	switch {
	case bf <= 3.0:
		return "narrow"
	case bf <= 5.0:
		return "normal"
	default:
		return "broad"
	}
}

func reexplorationLabel(rem float64) string {
	switch {
	case rem <= 1.3:
		return "clean"
	case rem <= 2.0:
		return "normal"
	case rem <= 3.0:
		return "heavy"
	default:
		return "extreme"
	}
}
