// Package scan exposes the contextspectre diagnostic API for use by
// external consumers. All types and functions are thin re-exports from internal
// packages — no logic lives here.
package scan

import (
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// Type aliases — keeps the pkg/ surface stable even if internal type names change.
type ContextStats = analyzer.ContextStats
type CleanupRecommendation = analyzer.CleanupRecommendation
type CleanupItem = analyzer.CleanupItem
type DiagnosisResult = analyzer.DiagnosisResult
type LightStats = jsonl.LightStats

// Analyze computes a ContextStats snapshot from a pre-parsed entry slice.
// Callers that need to parse from disk should use AnalyzePath instead.
var Analyze = analyzer.Analyze

// Recommend builds a ranked cleanup recommendation from a ContextStats snapshot.
var Recommend = analyzer.Recommend

// Diagnose detects structural issues (orphaned tools, oversized images, etc.)
// in a pre-parsed entry slice.
var Diagnose = analyzer.Diagnose

// ScanLight performs a fast lightweight stats pass from a session file path,
// without loading full entry content.
var ScanLight = jsonl.ScanLight

// Parse loads and parses a session JSONL file into entries.
var Parse = jsonl.Parse
