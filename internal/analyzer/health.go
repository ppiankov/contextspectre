package analyzer

// HealthScore represents the signal/noise ratio for a session's context.
type HealthScore struct {
	SignalTokens    int     // CurrentContextTokens - NoiseTokens
	NoiseTokens     int     // sum of all CleanupItem.TokensSaved
	TotalTokens     int     // CurrentContextTokens
	SignalPercent   float64 // SignalTokens / TotalTokens * 100
	NoisePercent    float64 // NoiseTokens / TotalTokens * 100
	Grade           string  // "A" (>90%), "B" (>75%), "C" (>60%), "D" (>40%), "F" (<=40%)
	BiggestOffender string  // CleanupItem[0].Category
	OffenderTokens  int     // CleanupItem[0].TokensSaved
}

// ComputeHealth derives a health score from existing analysis data.
func ComputeHealth(stats *ContextStats, rec *CleanupRecommendation) *HealthScore {
	h := &HealthScore{}

	h.TotalTokens = stats.CurrentContextTokens
	if h.TotalTokens <= 0 {
		h.SignalPercent = 100
		h.Grade = "A"
		return h
	}

	if rec != nil {
		h.NoiseTokens = rec.TotalTokens
		if len(rec.Items) > 0 {
			h.BiggestOffender = rec.Items[0].Category
			h.OffenderTokens = rec.Items[0].TokensSaved
		}
	}

	h.SignalTokens = h.TotalTokens - h.NoiseTokens
	if h.SignalTokens < 0 {
		h.SignalTokens = 0
	}

	h.SignalPercent = float64(h.SignalTokens) / float64(h.TotalTokens) * 100
	h.NoisePercent = float64(h.NoiseTokens) / float64(h.TotalTokens) * 100
	h.Grade = gradeFromPercent(h.SignalPercent)

	return h
}

// gradeFromPercent maps signal percent to a letter grade.
func gradeFromPercent(pct float64) string {
	switch {
	case pct > 90:
		return "A"
	case pct > 75:
		return "B"
	case pct > 60:
		return "C"
	case pct > 40:
		return "D"
	default:
		return "F"
	}
}

// GradeFromSignalPercent exports grading for use by TUI session browser.
func GradeFromSignalPercent(pct int) string {
	return gradeFromPercent(float64(pct))
}
