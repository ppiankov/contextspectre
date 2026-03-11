package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// PreserveResult holds the outcome of a preserve operation.
type PreserveResult struct {
	OutputPath string
	Decisions  int
	Findings   int
	Questions  int
	Files      int
}

// Preserve extracts decisions, findings, user questions, and file references
// from entries about to be deleted and writes them to a sidecar markdown file.
// toDelete is the set of entry indices that will be removed by the next operation.
func Preserve(sessionPath string, entries []jsonl.Entry, toDelete map[int]bool) (*PreserveResult, error) {
	if len(toDelete) == 0 {
		return &PreserveResult{}, nil
	}

	var decisions []string
	var findings []string
	var questions []string
	fileSet := make(map[string]bool)

	for idx := range toDelete {
		if idx < 0 || idx >= len(entries) {
			continue
		}
		e := entries[idx]
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
					path := analyzer.ExtractToolInputPath(b.Input)
					if path != "" {
						fileSet[path] = true
					}
				}
				if b.Type == "text" {
					if hint := analyzer.ExtractDecisionHint(b.Text); hint != "" && len(decisions) < 50 {
						decisions = append(decisions, hint)
					}
					if finding := extractPreserveFinding(b.Text); finding != "" && len(findings) < 50 {
						findings = append(findings, finding)
					}
				}
			}
		}

		if e.Type == jsonl.TypeUser {
			for _, b := range blocks {
				if b.Type == "text" && len(questions) < 30 {
					text := strings.TrimSpace(b.Text)
					if len(text) > 5 && len(text) < 500 {
						questions = append(questions, analyzer.TruncateHint(text, 120))
					}
				}
			}
		}
	}

	var files []string
	for path := range fileSet {
		files = append(files, path)
	}

	// Only write if there's something to preserve
	if len(decisions) == 0 && len(findings) == 0 && len(questions) == 0 {
		return &PreserveResult{}, nil
	}

	// Build output path: session.preserved.md (append if exists)
	base := strings.TrimSuffix(filepath.Base(sessionPath), ".jsonl")
	outputPath := filepath.Join(filepath.Dir(sessionPath), base+".preserved.md")

	content := renderPreserved(base, decisions, findings, questions, files, len(toDelete))

	// Append to existing file
	f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open preserve file: %w", err)
	}

	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write preserve file: %w", err)
	}

	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close preserve file: %w", err)
	}

	return &PreserveResult{
		OutputPath: outputPath,
		Decisions:  len(decisions),
		Findings:   len(findings),
		Questions:  len(questions),
		Files:      len(files),
	}, nil
}

// preserveFindingKeywords are indicators of discovered facts or observations.
var preserveFindingKeywords = []string{
	"found that", "discovered", "noticed", "turns out",
	"the issue is", "the problem is", "root cause",
	"confirmed", "verified", "tested and",
}

// extractPreserveFinding returns a truncated snippet if the text contains finding keywords.
func extractPreserveFinding(text string) string {
	lower := strings.ToLower(text)
	for _, kw := range preserveFindingKeywords {
		idx := strings.Index(lower, kw)
		if idx >= 0 {
			start := idx
			if start > 20 {
				start = idx - 20
			}
			snippet := text[start:]
			return analyzer.TruncateHint(snippet, 150)
		}
	}
	return ""
}

// renderPreserved generates a markdown section for preserved data.
func renderPreserved(sessionID string, decisions, findings, questions, files []string, entriesDeleted int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n---\n## Preserved — %s\n", time.Now().Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("Extracted from %d entries before cleanup.\n\n", entriesDeleted))

	if len(decisions) > 0 {
		sb.WriteString("### Decisions\n")
		for _, d := range decisions {
			sb.WriteString(fmt.Sprintf("- %s\n", d))
		}
		sb.WriteString("\n")
	}

	if len(findings) > 0 {
		sb.WriteString("### Findings\n")
		for _, f := range findings {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	if len(questions) > 0 {
		sb.WriteString("### User Requests\n")
		for _, q := range questions {
			sb.WriteString(fmt.Sprintf("- %s\n", q))
		}
		sb.WriteString("\n")
	}

	if len(files) > 0 {
		sb.WriteString("### Files Referenced\n")
		for _, f := range files {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
