package analyzer

import (
	"encoding/json"
	"strings"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// InputPurity measures how much tool result input could have been compressed
// before entering the context window.
type InputPurity struct {
	Score              float64        `json:"score"` // 0-100, 100 = all input pure
	TotalResultTokens  int            `json:"total_result_tokens"`
	CompressibleTokens int            `json:"compressible_tokens"`
	ByCategory         map[string]int `json:"by_category"` // category → compressible tokens
}

// compressionCategory defines a tool output category and its estimated compression ratio.
type compressionCategory struct {
	name    string
	ratio   float64
	matches []string // substrings to match in bash command
}

var bashCategories = []compressionCategory{
	{"git_output", 0.70, []string{"git status", "git diff", "git log", "git show", "git branch"}},
	{"file_listing", 0.60, []string{"ls ", "ls\n", "find ", "tree ", "tree\n"}},
	{"test_output", 0.65, []string{"go test", "pytest", "npm test", "jest", "cargo test", "make test"}},
	{"lint_output", 0.55, []string{"golangci-lint", "eslint", "ruff ", "pylint", "clippy"}},
	{"build_output", 0.50, []string{"go build", "make ", "npm run build", "cargo build", "gcc ", "g++ "}},
}

const largeResultThreshold = 2000 // tokens
const largeResultRatio = 0.30

// ComputeInputPurity analyzes tool results in a session and estimates
// what fraction could have been compressed before entering context.
func ComputeInputPurity(entries []jsonl.Entry) *InputPurity {
	// Pass 1: build map of tool_use ID → tool info.
	type toolInfo struct {
		name    string
		command string // bash command string, if applicable
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
			if b.Type != "tool_use" || b.ID == "" {
				continue
			}
			ti := toolInfo{name: b.Name}
			if isBashTool(b.Name) {
				ti.command = extractBashCommand(b.Input)
			}
			toolMap[b.ID] = ti
		}
	}

	// Pass 2: scan tool_result blocks, classify, and accumulate.
	ip := &InputPurity{
		ByCategory: make(map[string]int),
	}

	for _, e := range entries {
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
			resultTokens := toolResultContentSize(b) / 4
			if resultTokens == 0 {
				continue
			}
			ip.TotalResultTokens += resultTokens

			ti, ok := toolMap[b.ToolUseID]
			if !ok {
				// Unknown tool — only flag if large.
				if resultTokens > largeResultThreshold {
					compressible := int(float64(resultTokens) * largeResultRatio)
					ip.CompressibleTokens += compressible
					ip.ByCategory["large_result"] += compressible
				}
				continue
			}

			if isBashTool(ti.name) && ti.command != "" {
				cat, ratio := classifyBashCommand(ti.command)
				if cat != "" {
					compressible := int(float64(resultTokens) * ratio)
					ip.CompressibleTokens += compressible
					ip.ByCategory[cat] += compressible
					continue
				}
			}

			// Non-bash or unclassified bash — check if large.
			if resultTokens > largeResultThreshold {
				compressible := int(float64(resultTokens) * largeResultRatio)
				ip.CompressibleTokens += compressible
				ip.ByCategory["large_result"] += compressible
			}
		}
	}

	if ip.TotalResultTokens > 0 {
		ip.Score = (1 - float64(ip.CompressibleTokens)/float64(ip.TotalResultTokens)) * 100
		if ip.Score < 0 {
			ip.Score = 0
		}
	} else {
		ip.Score = 100
	}

	return ip
}

// classifyBashCommand matches a command string against known compressible categories.
func classifyBashCommand(command string) (string, float64) {
	lower := strings.ToLower(command)
	for _, cat := range bashCategories {
		for _, match := range cat.matches {
			if strings.Contains(lower, match) {
				return cat.name, cat.ratio
			}
		}
	}
	return "", 0
}

// extractBashCommand extracts the command string from a Bash tool_use input.
func extractBashCommand(input json.RawMessage) string {
	var fields struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	return fields.Command
}
