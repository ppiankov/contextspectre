// Package ips exposes the contextspectre IPS (Integrity/Performance Score)
// metrics API for use by external consumers (e.g. contextspectre-pro). All
// types are thin re-exports from the internal analyzer package — no logic
// lives here.
package ips

import "github.com/ppiankov/contextspectre/internal/analyzer"

// Type aliases.
type HealthScore = analyzer.HealthScore
type EntropyScore = analyzer.EntropyScore
type EntropyLevel = analyzer.EntropyLevel
type EntropyInput = analyzer.EntropyInput
type EntropyBreakdown = analyzer.EntropyBreakdown

// EntropyLevel constants.
const (
	EntropyLow      = analyzer.EntropyLow
	EntropyMedium   = analyzer.EntropyMedium
	EntropyHigh     = analyzer.EntropyHigh
	EntropyCritical = analyzer.EntropyCritical
)

// ComputeHealth derives a signal/noise health score from analysis data.
// Returns a HealthScore with grade (A-F) and per-axis breakdown.
var ComputeHealth = analyzer.ComputeHealth

// CalculateEntropy computes a deterministic session entropy score (0-100)
// from economic, reasoning, and structural decay axes.
var CalculateEntropy = analyzer.CalculateEntropy

// SignalRatioForGrade converts a letter grade to the normalized ratio used
// as input to CalculateEntropy.
var SignalRatioForGrade = analyzer.SignalRatioForGrade

// GradeFromSignalPercent converts an integer signal percentage to a letter
// grade. Exported for TUI and status line consumers.
var GradeFromSignalPercent = analyzer.GradeFromSignalPercent
