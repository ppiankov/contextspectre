package analyzer

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// InjectionReport holds vector injection scan results.
type InjectionReport struct {
	Findings   []InjectionFinding `json:"findings"`
	RiskScore  float64            `json:"risk_score"`
	HighestSev string             `json:"highest_severity"`
}

// InjectionFinding records a single detected injection pattern.
type InjectionFinding struct {
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	EntryIndex int    `json:"entry_index"`
	Turn       int    `json:"turn"`
	ToolName   string `json:"tool_name"`
	Pattern    string `json:"pattern"`
	Context    string `json:"context"`
}

// Severity weights for risk scoring.
const (
	sevWeightCritical = 25
	sevWeightHigh     = 10
	sevWeightMedium   = 5
	sevWeightLow      = 1
	maxRiskScore      = 100
)

var severityOrder = map[string]int{
	"critical": 4,
	"high":     3,
	"medium":   2,
	"low":      1,
	"none":     0,
}

// Imperative phrases that suggest directive injection.
var imperativePhrases = []string{
	"you must", "you should", "always do", "never do",
	"ignore previous", "ignore above", "ignore all",
	"instead you should", "override", "disregard",
	"do not follow", "forget your", "new instructions",
	"from now on", "act as", "pretend to be",
	"your new role", "system prompt",
}

// System-like tag patterns.
var systemLikePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<system[^>]*>`),
	regexp.MustCompile(`(?i)<system-reminder[^>]*>`),
	regexp.MustCompile(`(?i)\[SYSTEM\]`),
	regexp.MustCompile(`(?i)\[INST\]`),
	regexp.MustCompile(`(?i)<\|im_start\|>system`),
}

// Role confusion patterns.
var roleConfusionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)as (?:the|a) (?:system|assistant|user|admin)`),
	regexp.MustCompile(`(?i)speaking as (?:system|assistant|user)`),
	regexp.MustCompile(`(?i)role:\s*(?:system|assistant|user)`),
}

// Embedded directive patterns.
var embeddedDirectivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)"instruction"\s*:`),
	regexp.MustCompile(`(?i)DIRECTIVE\s*:`),
	regexp.MustCompile(`(?i)(?:NOTE|IMPORTANT|WARNING)\s*:\s*(?:you must|you should|always|never|ignore|override|disregard)`),
}

// HTML comment with imperative content.
var htmlCommentPattern = regexp.MustCompile(`<!--\s*(.*?)\s*-->`)

// Zero-width unicode characters.
var zeroWidthChars = []rune{
	'\u200B', // zero-width space
	'\u200C', // zero-width non-joiner
	'\u200D', // zero-width joiner
	'\uFEFF', // byte order mark / zero-width no-break space
	'\u200E', // left-to-right mark
	'\u200F', // right-to-left mark
	'\u2060', // word joiner
	'\u2061', // function application
	'\u2062', // invisible times
	'\u2063', // invisible separator
	'\u2064', // invisible plus
}

// DetectInjection scans session entries for vector injection patterns.
func DetectInjection(entries []jsonl.Entry) *InjectionReport {
	report := &InjectionReport{
		HighestSev: "none",
	}

	// Pass 1: build tool_use ID → tool name map.
	type toolInfo struct {
		name string
	}
	toolMap := make(map[string]toolInfo)
	for _, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID != "" {
				toolMap[b.ID] = toolInfo{name: b.Name}
			}
		}
	}

	// Pass 2: scan tool_result blocks.
	turnCount := 0
	for i, e := range entries {
		if e.Type == jsonl.TypeAssistant {
			turnCount++
		}
		if e.Type != jsonl.TypeUser || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type != "tool_result" {
				continue
			}
			text := toolResultText(b)
			if text == "" {
				continue
			}

			toolName := ""
			if ti, ok := toolMap[b.ToolUseID]; ok {
				toolName = ti.name
			}

			// Run all scanners.
			findings := scanForInjection(text, i, turnCount, toolName)
			report.Findings = append(report.Findings, findings...)
		}
	}

	// Compute risk score and highest severity.
	score := 0.0
	for _, f := range report.Findings {
		switch f.Severity {
		case "critical":
			score += sevWeightCritical
		case "high":
			score += sevWeightHigh
		case "medium":
			score += sevWeightMedium
		case "low":
			score += sevWeightLow
		}
		if severityOrder[f.Severity] > severityOrder[report.HighestSev] {
			report.HighestSev = f.Severity
		}
	}
	if score > maxRiskScore {
		score = maxRiskScore
	}
	report.RiskScore = score

	return report
}

// scanForInjection runs all pattern scanners on text content.
func scanForInjection(text string, entryIdx, turn int, toolName string) []InjectionFinding {
	var findings []InjectionFinding

	findings = append(findings, scanZeroWidth(text, entryIdx, turn, toolName)...)
	findings = append(findings, scanSystemLike(text, entryIdx, turn, toolName)...)
	findings = append(findings, scanImperative(text, entryIdx, turn, toolName)...)
	findings = append(findings, scanHTMLComment(text, entryIdx, turn, toolName)...)
	findings = append(findings, scanEmbeddedDirective(text, entryIdx, turn, toolName)...)
	findings = append(findings, scanRoleConfusion(text, entryIdx, turn, toolName)...)

	return findings
}

// scanZeroWidth detects zero-width unicode characters.
func scanZeroWidth(text string, entryIdx, turn int, toolName string) []InjectionFinding {
	var findings []InjectionFinding
	for _, zwc := range zeroWidthChars {
		idx := strings.IndexRune(text, zwc)
		if idx >= 0 {
			findings = append(findings, InjectionFinding{
				Kind:       "zero_width",
				Severity:   "critical",
				EntryIndex: entryIdx,
				Turn:       turn,
				ToolName:   toolName,
				Pattern:    string(zwc),
				Context:    truncateContext(text, idx, 100),
			})
			break // One finding per block is enough
		}
	}
	return findings
}

// scanSystemLike detects system-like tags in tool results.
func scanSystemLike(text string, entryIdx, turn int, toolName string) []InjectionFinding {
	var findings []InjectionFinding
	for _, pat := range systemLikePatterns {
		loc := pat.FindStringIndex(text)
		if loc != nil {
			matched := text[loc[0]:loc[1]]
			findings = append(findings, InjectionFinding{
				Kind:       "system_like",
				Severity:   "high",
				EntryIndex: entryIdx,
				Turn:       turn,
				ToolName:   toolName,
				Pattern:    matched,
				Context:    truncateContext(text, loc[0], 100),
			})
			break
		}
	}
	return findings
}

// scanImperative detects directive phrases in tool output.
func scanImperative(text string, entryIdx, turn int, toolName string) []InjectionFinding {
	var findings []InjectionFinding
	lower := strings.ToLower(text)
	for _, phrase := range imperativePhrases {
		idx := strings.Index(lower, phrase)
		if idx >= 0 {
			findings = append(findings, InjectionFinding{
				Kind:       "imperative",
				Severity:   "high",
				EntryIndex: entryIdx,
				Turn:       turn,
				ToolName:   toolName,
				Pattern:    phrase,
				Context:    truncateContext(text, idx, 100),
			})
			break
		}
	}
	return findings
}

// scanHTMLComment detects HTML comments containing imperative content.
func scanHTMLComment(text string, entryIdx, turn int, toolName string) []InjectionFinding {
	var findings []InjectionFinding
	matches := htmlCommentPattern.FindAllStringSubmatchIndex(text, 10)
	for _, loc := range matches {
		if len(loc) < 4 {
			continue
		}
		commentBody := strings.ToLower(text[loc[2]:loc[3]])
		for _, phrase := range imperativePhrases {
			if strings.Contains(commentBody, phrase) {
				findings = append(findings, InjectionFinding{
					Kind:       "html_comment",
					Severity:   "medium",
					EntryIndex: entryIdx,
					Turn:       turn,
					ToolName:   toolName,
					Pattern:    text[loc[0]:loc[1]],
					Context:    truncateContext(text, loc[0], 100),
				})
				return findings // One per block
			}
		}
	}
	return findings
}

// scanEmbeddedDirective detects YAML/JSON-like directives.
func scanEmbeddedDirective(text string, entryIdx, turn int, toolName string) []InjectionFinding {
	var findings []InjectionFinding
	for _, pat := range embeddedDirectivePatterns {
		loc := pat.FindStringIndex(text)
		if loc != nil {
			matched := text[loc[0]:loc[1]]
			findings = append(findings, InjectionFinding{
				Kind:       "embedded_directive",
				Severity:   "medium",
				EntryIndex: entryIdx,
				Turn:       turn,
				ToolName:   toolName,
				Pattern:    matched,
				Context:    truncateContext(text, loc[0], 100),
			})
			break
		}
	}
	return findings
}

// scanRoleConfusion detects content claiming to be from system/assistant/user.
func scanRoleConfusion(text string, entryIdx, turn int, toolName string) []InjectionFinding {
	var findings []InjectionFinding
	for _, pat := range roleConfusionPatterns {
		loc := pat.FindStringIndex(text)
		if loc != nil {
			matched := text[loc[0]:loc[1]]
			findings = append(findings, InjectionFinding{
				Kind:       "role_confusion",
				Severity:   "high",
				EntryIndex: entryIdx,
				Turn:       turn,
				ToolName:   toolName,
				Pattern:    matched,
				Context:    truncateContext(text, loc[0], 100),
			})
			break
		}
	}
	return findings
}

// toolResultText extracts the text content from a tool_result block.
func toolResultText(b jsonl.ContentBlock) string {
	if b.Content == nil {
		return ""
	}
	// Try as string
	var s string
	if json.Unmarshal(b.Content, &s) == nil {
		return s
	}
	// Try as array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(b.Content, &blocks) == nil {
		var sb strings.Builder
		for _, bl := range blocks {
			if bl.Type == "text" {
				sb.WriteString(bl.Text)
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}
	return string(b.Content)
}

// truncateContext returns text around a position, truncated to maxLen.
func truncateContext(text string, pos, maxLen int) string {
	start := pos - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}
	result := text[start:end]
	// Clean up for display
	result = strings.ReplaceAll(result, "\n", " ")
	result = strings.TrimSpace(result)
	if start > 0 {
		result = "..." + result
	}
	if end < len(text) {
		result = result + "..."
	}
	return result
}
