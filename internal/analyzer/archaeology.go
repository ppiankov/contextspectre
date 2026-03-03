package analyzer

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// CompactionReport holds archaeology data for all compaction events in a session.
type CompactionReport struct {
	Events []CompactionArchaeology
}

// CompactionArchaeology describes what was lost at a single compaction boundary.
type CompactionArchaeology struct {
	CompactionIndex int
	LineIndex       int
	Before          EpochSummary
	After           CompactionSummary
}

// EpochSummary holds structural metadata extracted from a pre-compaction epoch.
type EpochSummary struct {
	TurnCount       int
	TokensPeak      int
	FilesReferenced []string
	ToolCallCounts  map[string]int
	UserQuestions   []string
	DecisionHints   []string
}

// TotalToolCalls returns the sum of all tool call counts.
func (s EpochSummary) TotalToolCalls() int {
	total := 0
	for _, c := range s.ToolCallCounts {
		total += c
	}
	return total
}

// CompactionSummary holds post-compaction data.
type CompactionSummary struct {
	SummaryText      string
	SummaryCharCount int
	CompressionRatio float64
}

// AnalyzeCompactions performs archaeology on all compaction events.
// It segments entries by compaction boundaries, extracts structural metadata
// from each pre-compaction epoch, and captures the post-compaction summary.
func AnalyzeCompactions(entries []jsonl.Entry, compactions []CompactionEvent) *CompactionReport {
	if len(entries) == 0 || len(compactions) == 0 {
		return &CompactionReport{}
	}

	// Build boundary indices from compaction events (same as CalculateEpochCosts)
	boundaries := make([]int, len(compactions))
	for i, c := range compactions {
		boundaries[i] = c.LineIndex
	}

	// Segment entries into epochs
	var epochs [][]jsonl.Entry
	var current []jsonl.Entry
	boundaryPos := 0

	for i, e := range entries {
		if boundaryPos < len(boundaries) && i >= boundaries[boundaryPos] {
			epochs = append(epochs, current)
			current = nil
			boundaryPos++
		}
		current = append(current, e)
	}
	epochs = append(epochs, current)

	// Build archaeology for each compaction
	report := &CompactionReport{}
	for i, c := range compactions {
		arch := CompactionArchaeology{
			CompactionIndex: i,
			LineIndex:       c.LineIndex,
		}

		// Pre-compaction epoch
		if i < len(epochs) {
			arch.Before = extractEpochSummary(epochs[i])
		}

		// Post-compaction summary: entry at compaction LineIndex
		if c.LineIndex >= 0 && c.LineIndex < len(entries) {
			arch.After.SummaryText = extractSummaryText(entries[c.LineIndex])
			arch.After.SummaryCharCount = utf8.RuneCountInString(arch.After.SummaryText)
		}

		// Compression ratio
		if c.AfterTokens > 0 {
			arch.After.CompressionRatio = float64(c.BeforeTokens) / float64(c.AfterTokens)
		}

		report.Events = append(report.Events, arch)
	}

	return report
}

// extractEpochSummary scans an epoch's entries and builds structural metadata.
func extractEpochSummary(entries []jsonl.Entry) EpochSummary {
	summary := EpochSummary{
		ToolCallCounts: make(map[string]int),
	}

	fileSet := make(map[string]bool)

	for _, e := range entries {
		if e.IsConversational() {
			summary.TurnCount++
		}

		// Track peak tokens
		if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Usage != nil {
			ctx := e.Message.Usage.TotalContextTokens()
			if ctx > summary.TokensPeak {
				summary.TokensPeak = ctx
			}
		}

		if e.Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		if e.Type == jsonl.TypeAssistant {
			for _, b := range blocks {
				if b.Type == "tool_use" {
					summary.ToolCallCounts[b.Name]++
					path := extractToolInputPath(b.Input)
					if path != "" {
						fileSet[path] = true
					}
				}
				if b.Type == "text" && len(summary.DecisionHints) < 5 {
					if hint := extractDecisionHint(b.Text); hint != "" {
						summary.DecisionHints = append(summary.DecisionHints, hint)
					}
				}
			}
		}

		if e.Type == jsonl.TypeUser {
			for _, b := range blocks {
				if b.Type == "text" && len(summary.UserQuestions) < 10 {
					text := strings.TrimSpace(b.Text)
					if strings.HasSuffix(text, "?") {
						summary.UserQuestions = append(summary.UserQuestions, truncateHint(text, 120))
					}
				}
			}
		}
	}

	for path := range fileSet {
		summary.FilesReferenced = append(summary.FilesReferenced, path)
	}

	return summary
}

// extractSummaryText extracts concatenated text content from an entry.
func extractSummaryText(e jsonl.Entry) string {
	if e.Message == nil {
		return ""
	}
	blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
	if err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// extractToolInputPath extracts file_path, path, or pattern from tool_use input.
func extractToolInputPath(input json.RawMessage) string {
	var fields struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
		Pattern  string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	if fields.FilePath != "" {
		return fields.FilePath
	}
	if fields.Path != "" {
		return fields.Path
	}
	return fields.Pattern
}

// decisionKeywords are structural indicators of decisions in assistant text.
var decisionKeywords = []string{
	"decided", "chose", "going with", "instead of",
	"opted", "trade-off", "rather than", "because",
}

// extractDecisionHint returns a truncated hint if the text contains decision keywords.
func extractDecisionHint(text string) string {
	lower := strings.ToLower(text)
	for _, kw := range decisionKeywords {
		idx := strings.Index(lower, kw)
		if idx >= 0 {
			// Extract a window around the keyword
			start := idx
			if start > 20 {
				start = idx - 20
			}
			snippet := text[start:]
			return truncateHint(snippet, 120)
		}
	}
	return ""
}

// truncateHint truncates text to maxLen runes, adding "..." if needed.
func truncateHint(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
