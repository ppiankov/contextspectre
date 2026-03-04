package analyzer

import (
	"sort"
	"strings"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// ContinuityReport holds cross-session continuity analysis results.
type ContinuityReport struct {
	ProjectName     string
	SessionsScanned int
	RepeatedFiles   []RepeatedFile
	RepeatedTexts   []RepeatedText
	TotalFileTokens int
	TotalTextTokens int
	TotalTaxTokens  int
	TotalTaxCost    float64
	TotalFileCost   float64
	TotalTextCost   float64
}

// RepeatedFile tracks a file path read across multiple sessions.
type RepeatedFile struct {
	Path            string
	SessionCount    int
	Sessions        []string
	EstimatedTokens int
}

// RepeatedText tracks a user text block repeated across sessions.
type RepeatedText struct {
	Text            string
	CharCount       int
	SessionCount    int
	Sessions        []string
	EstimatedTokens int
}

// ContinuitySessionInput is the input for a single session.
type ContinuitySessionInput struct {
	SessionID   string
	SessionSlug string
	Entries     []jsonl.Entry
	Model       string
}

// AnalyzeContinuity scans entries from multiple sessions and finds
// cross-session repetitions of file reads and user text blocks.
func AnalyzeContinuity(sessions []ContinuitySessionInput) *ContinuityReport {
	report := &ContinuityReport{
		SessionsScanned: len(sessions),
	}

	// map[filePath] -> map[sessionLabel] -> estimated tokens
	fileSessionMap := make(map[string]map[string]int)

	// map[normalizedText] -> textInfo
	type textInfo struct {
		original  string
		charCount int
		sessions  map[string]int // sessionLabel -> estimated tokens
	}
	textSessionMap := make(map[string]*textInfo)

	var model string

	for _, si := range sessions {
		sessionLabel := si.SessionSlug
		if sessionLabel == "" && len(si.SessionID) >= 8 {
			sessionLabel = si.SessionID[:8]
		}

		if model == "" && si.Model != "" {
			model = si.Model
		}

		sessionFileSeen := make(map[string]bool)
		sessionTextSeen := make(map[string]bool)

		for i, e := range si.Entries {
			if e.Message == nil {
				continue
			}

			blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
			if err != nil {
				continue
			}

			// Extract file reads from assistant tool_use blocks
			if e.Type == jsonl.TypeAssistant {
				for _, b := range blocks {
					if b.Type != "tool_use" || !isFileReadTool(b.Name) {
						continue
					}
					path := ExtractToolInputPath(b.Input)
					if path == "" {
						continue
					}
					if sessionFileSeen[path] {
						continue
					}
					sessionFileSeen[path] = true

					if fileSessionMap[path] == nil {
						fileSessionMap[path] = make(map[string]int)
					}

					resultSize := 0
					resultIdx := findToolResult(si.Entries, i, b.ID)
					if resultIdx >= 0 {
						resultSize = si.Entries[resultIdx].RawSize
					}
					fileSessionMap[path][sessionLabel] += resultSize / 4
				}
			}

			// Extract user text blocks (>100 chars)
			if e.Type == jsonl.TypeUser {
				for _, b := range blocks {
					if b.Type != "text" {
						continue
					}
					text := strings.TrimSpace(b.Text)
					if len(text) < 100 {
						continue
					}
					normalized := normalizeForContinuity(text)
					if sessionTextSeen[normalized] {
						continue
					}
					sessionTextSeen[normalized] = true

					if textSessionMap[normalized] == nil {
						textSessionMap[normalized] = &textInfo{
							original:  TruncateHint(text, 120),
							charCount: len(text),
							sessions:  make(map[string]int),
						}
					}
					textSessionMap[normalized].sessions[sessionLabel] += e.RawSize / 4
				}
			}
		}
	}

	// Build repeated files (2+ sessions)
	for path, sessionMap := range fileSessionMap {
		if len(sessionMap) < 2 {
			continue
		}
		rf := RepeatedFile{
			Path:         path,
			SessionCount: len(sessionMap),
		}
		for sid, tokens := range sessionMap {
			rf.Sessions = append(rf.Sessions, sid)
			rf.EstimatedTokens += tokens
		}
		// Subtract the cheapest session's cost (first read was necessary)
		minTokens := rf.EstimatedTokens
		for _, tokens := range sessionMap {
			if tokens < minTokens {
				minTokens = tokens
			}
		}
		rf.EstimatedTokens -= minTokens
		sort.Strings(rf.Sessions)
		report.RepeatedFiles = append(report.RepeatedFiles, rf)
	}
	sort.Slice(report.RepeatedFiles, func(i, j int) bool {
		return report.RepeatedFiles[i].SessionCount > report.RepeatedFiles[j].SessionCount
	})

	// Build repeated texts (2+ sessions)
	for _, ti := range textSessionMap {
		if len(ti.sessions) < 2 {
			continue
		}
		rt := RepeatedText{
			Text:         ti.original,
			CharCount:    ti.charCount,
			SessionCount: len(ti.sessions),
		}
		for sid, tokens := range ti.sessions {
			rt.Sessions = append(rt.Sessions, sid)
			rt.EstimatedTokens += tokens
		}
		minTokens := rt.EstimatedTokens
		for _, tokens := range ti.sessions {
			if tokens < minTokens {
				minTokens = tokens
			}
		}
		rt.EstimatedTokens -= minTokens
		sort.Strings(rt.Sessions)
		report.RepeatedTexts = append(report.RepeatedTexts, rt)
	}
	sort.Slice(report.RepeatedTexts, func(i, j int) bool {
		return report.RepeatedTexts[i].SessionCount > report.RepeatedTexts[j].SessionCount
	})

	// Compute totals
	for _, rf := range report.RepeatedFiles {
		report.TotalFileTokens += rf.EstimatedTokens
	}
	for _, rt := range report.RepeatedTexts {
		report.TotalTextTokens += rt.EstimatedTokens
	}
	report.TotalTaxTokens = report.TotalFileTokens + report.TotalTextTokens

	pricing := PricingForModel(model)
	report.TotalFileCost = float64(report.TotalFileTokens) / 1_000_000 * pricing.InputPerMillion
	report.TotalTextCost = float64(report.TotalTextTokens) / 1_000_000 * pricing.InputPerMillion
	report.TotalTaxCost = report.TotalFileCost + report.TotalTextCost

	return report
}

// normalizeForContinuity normalizes text for cross-session dedup.
// Same logic as editor.normalizeForDedup but avoids circular import.
func normalizeForContinuity(s string) string {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	return strings.ToLower(strings.Join(fields, " "))
}
