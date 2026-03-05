package analyzer

import (
	"sort"
	"strings"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// ContinuityReport holds cross-session continuity analysis results.
type ContinuityReport struct {
	ProjectName      string
	SessionsScanned  int
	RepeatedFiles    []RepeatedFile
	RepeatedTexts    []RepeatedText
	RepeatTopics     []RepeatTopic
	Suggestions      []ContinuitySuggestion
	TotalFileReads   int
	UniqueFileReads  int
	TotalTextBlocks  int
	UniqueTextBlocks int
	ContinuityIndex  float64
	TotalFileTokens  int
	TotalTextTokens  int
	TotalTaxTokens   int
	TotalTaxCost     float64
	TotalFileCost    float64
	TotalTextCost    float64
}

// RepeatedFile tracks a file path read across multiple sessions.
type RepeatedFile struct {
	Path            string
	SessionCount    int
	ReadCount       int
	RedundantReads  int
	Sessions        []string
	EstimatedTokens int
	EstimatedCost   float64
}

// RepeatedText tracks a user text block repeated across sessions.
type RepeatedText struct {
	Text            string
	CharCount       int
	SessionCount    int
	ReadCount       int
	RedundantReads  int
	Sessions        []string
	EstimatedTokens int
	EstimatedCost   float64
}

// RepeatTopic tracks a repeated file cluster (co-occurring reads across sessions).
type RepeatTopic struct {
	Files           []string
	SessionCount    int
	EstimatedTokens int
	EstimatedCost   float64
}

// ContinuitySuggestion is a file suggestion for project context docs.
type ContinuitySuggestion struct {
	Path            string
	SessionCount    int
	EstimatedTokens int
	EstimatedCost   float64
	Reason          string
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

	// map[sessionLabel] -> unique files read in that session
	sessionFiles := make(map[string][]string)

	var model string

	for _, si := range sessions {
		sessionLabel := si.SessionSlug
		if sessionLabel == "" && len(si.SessionID) >= 8 {
			sessionLabel = si.SessionID[:8]
		}
		if sessionLabel == "" {
			sessionLabel = si.SessionID
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

			// Extract file reads from assistant tool_use blocks.
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
					report.TotalFileReads++
				}
			}

			// Extract user text blocks (>100 chars).
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
					report.TotalTextBlocks++
				}
			}
		}

		if len(sessionFileSeen) > 0 {
			files := make([]string, 0, len(sessionFileSeen))
			for path := range sessionFileSeen {
				files = append(files, path)
			}
			sort.Strings(files)
			sessionFiles[sessionLabel] = files
		}
	}

	report.UniqueFileReads = len(fileSessionMap)
	report.UniqueTextBlocks = len(textSessionMap)

	pricing := PricingForModel(model)

	// Build repeated files (2+ sessions).
	for path, sessionMap := range fileSessionMap {
		if len(sessionMap) < 2 {
			continue
		}
		rf := RepeatedFile{
			Path:           path,
			SessionCount:   len(sessionMap),
			ReadCount:      len(sessionMap),
			RedundantReads: len(sessionMap) - 1,
		}
		for sid, tokens := range sessionMap {
			rf.Sessions = append(rf.Sessions, sid)
			rf.EstimatedTokens += tokens
		}
		minTokens := rf.EstimatedTokens
		for _, tokens := range sessionMap {
			if tokens < minTokens {
				minTokens = tokens
			}
		}
		rf.EstimatedTokens -= minTokens
		if rf.EstimatedTokens < 0 {
			rf.EstimatedTokens = 0
		}
		rf.EstimatedCost = float64(rf.EstimatedTokens) / 1_000_000 * pricing.CacheReadPerMillion
		sort.Strings(rf.Sessions)
		report.RepeatedFiles = append(report.RepeatedFiles, rf)
	}
	sort.Slice(report.RepeatedFiles, func(i, j int) bool {
		if report.RepeatedFiles[i].EstimatedCost != report.RepeatedFiles[j].EstimatedCost {
			return report.RepeatedFiles[i].EstimatedCost > report.RepeatedFiles[j].EstimatedCost
		}
		if report.RepeatedFiles[i].EstimatedTokens != report.RepeatedFiles[j].EstimatedTokens {
			return report.RepeatedFiles[i].EstimatedTokens > report.RepeatedFiles[j].EstimatedTokens
		}
		return report.RepeatedFiles[i].SessionCount > report.RepeatedFiles[j].SessionCount
	})

	// Build repeated texts (2+ sessions).
	for _, ti := range textSessionMap {
		if len(ti.sessions) < 2 {
			continue
		}
		rt := RepeatedText{
			Text:           ti.original,
			CharCount:      ti.charCount,
			SessionCount:   len(ti.sessions),
			ReadCount:      len(ti.sessions),
			RedundantReads: len(ti.sessions) - 1,
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
		if rt.EstimatedTokens < 0 {
			rt.EstimatedTokens = 0
		}
		rt.EstimatedCost = float64(rt.EstimatedTokens) / 1_000_000 * pricing.InputPerMillion
		sort.Strings(rt.Sessions)
		report.RepeatedTexts = append(report.RepeatedTexts, rt)
	}
	sort.Slice(report.RepeatedTexts, func(i, j int) bool {
		if report.RepeatedTexts[i].EstimatedCost != report.RepeatedTexts[j].EstimatedCost {
			return report.RepeatedTexts[i].EstimatedCost > report.RepeatedTexts[j].EstimatedCost
		}
		if report.RepeatedTexts[i].EstimatedTokens != report.RepeatedTexts[j].EstimatedTokens {
			return report.RepeatedTexts[i].EstimatedTokens > report.RepeatedTexts[j].EstimatedTokens
		}
		return report.RepeatedTexts[i].SessionCount > report.RepeatedTexts[j].SessionCount
	})

	fileCostByPath := make(map[string]float64)
	fileTokensByPath := make(map[string]int)
	for _, rf := range report.RepeatedFiles {
		report.TotalFileTokens += rf.EstimatedTokens
		report.TotalFileCost += rf.EstimatedCost
		fileCostByPath[rf.Path] = rf.EstimatedCost
		fileTokensByPath[rf.Path] = rf.EstimatedTokens
	}
	for _, rt := range report.RepeatedTexts {
		report.TotalTextTokens += rt.EstimatedTokens
		report.TotalTextCost += rt.EstimatedCost
	}
	report.TotalTaxTokens = report.TotalFileTokens + report.TotalTextTokens
	report.TotalTaxCost = report.TotalFileCost + report.TotalTextCost

	report.ContinuityIndex = computeContinuityIndex(report)
	report.RepeatTopics = buildRepeatTopics(sessionFiles, fileTokensByPath, fileCostByPath)
	report.Suggestions = buildContinuitySuggestions(report.RepeatedFiles)

	return report
}

func computeContinuityIndex(r *ContinuityReport) float64 {
	if r == nil {
		return 0
	}

	var parts []float64
	if r.TotalFileReads > 0 {
		parts = append(parts, float64(r.UniqueFileReads)/float64(r.TotalFileReads))
	}
	if r.TotalTextBlocks > 0 {
		parts = append(parts, float64(r.UniqueTextBlocks)/float64(r.TotalTextBlocks))
	}
	if len(parts) == 0 {
		return 100
	}

	sum := 0.0
	for _, p := range parts {
		sum += p
	}
	return (sum / float64(len(parts))) * 100
}

func buildRepeatTopics(sessionFiles map[string][]string, tokenByPath map[string]int, costByPath map[string]float64) []RepeatTopic {
	type topicAccum struct {
		files [2]string
		count int
	}

	topicMap := make(map[string]*topicAccum)
	for _, files := range sessionFiles {
		if len(files) < 2 {
			continue
		}
		for i := 0; i < len(files); i++ {
			for j := i + 1; j < len(files); j++ {
				key := files[i] + "\x00" + files[j]
				acc := topicMap[key]
				if acc == nil {
					acc = &topicAccum{files: [2]string{files[i], files[j]}}
					topicMap[key] = acc
				}
				acc.count++
			}
		}
	}

	var topics []RepeatTopic
	for _, acc := range topicMap {
		if acc.count <= 2 {
			continue
		}
		topic := RepeatTopic{
			Files:           []string{acc.files[0], acc.files[1]},
			SessionCount:    acc.count,
			EstimatedTokens: tokenByPath[acc.files[0]] + tokenByPath[acc.files[1]],
			EstimatedCost:   costByPath[acc.files[0]] + costByPath[acc.files[1]],
		}
		topics = append(topics, topic)
	}

	sort.Slice(topics, func(i, j int) bool {
		if topics[i].SessionCount != topics[j].SessionCount {
			return topics[i].SessionCount > topics[j].SessionCount
		}
		if topics[i].EstimatedCost != topics[j].EstimatedCost {
			return topics[i].EstimatedCost > topics[j].EstimatedCost
		}
		return topics[i].EstimatedTokens > topics[j].EstimatedTokens
	})
	return topics
}

func buildContinuitySuggestions(repeated []RepeatedFile) []ContinuitySuggestion {
	var suggestions []ContinuitySuggestion
	for _, rf := range repeated {
		if rf.SessionCount <= 3 || rf.EstimatedTokens <= 1000 {
			continue
		}
		suggestions = append(suggestions, ContinuitySuggestion{
			Path:            rf.Path,
			SessionCount:    rf.SessionCount,
			EstimatedTokens: rf.EstimatedTokens,
			EstimatedCost:   rf.EstimatedCost,
			Reason:          "frequently re-read with high token cost",
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].EstimatedCost != suggestions[j].EstimatedCost {
			return suggestions[i].EstimatedCost > suggestions[j].EstimatedCost
		}
		return suggestions[i].SessionCount > suggestions[j].SessionCount
	})
	return suggestions
}

// normalizeForContinuity normalizes text for cross-session dedup.
// Same logic as editor.normalizeForDedup but avoids circular import.
func normalizeForContinuity(s string) string {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	return strings.ToLower(strings.Join(fields, " "))
}
