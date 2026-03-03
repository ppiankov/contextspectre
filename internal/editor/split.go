package editor

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// SplitResult holds the result of a split-to-markdown operation.
type SplitResult struct {
	EntriesExtracted int
	TokenCost        int
	DollarCost       float64
	TargetRepo       string
	ReExplFiles      []string
	OutputPath       string
}

// SplitToMarkdown extracts entries[from:to+1] into a markdown file.
func SplitToMarkdown(entries []jsonl.Entry, from, to int, meta *analyzer.RangeMetadata, sessionID, outputPath string) (*SplitResult, error) {
	if from < 0 || to >= len(entries) || from > to {
		return nil, fmt.Errorf("index out of range (session has %d entries, indices 0-%d)", len(entries), len(entries)-1)
	}

	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "# Split: session %s\n\n", sessionID)
	if meta.TargetRepo != "" {
		fmt.Fprintf(&b, "Tangent to **%s** (entries %d-%d)\n\n", meta.TargetRepo, from, to)
	} else {
		fmt.Fprintf(&b, "Entries %d-%d\n\n", from, to)
	}

	// Summary table
	b.WriteString("| Metric | Value |\n")
	b.WriteString("|--------|-------|\n")
	b.WriteString(fmt.Sprintf("| Entries | %d |\n", to-from+1))
	b.WriteString(fmt.Sprintf("| Tokens | ~%s |\n", formatTokensCompact(meta.TokenCost)))
	b.WriteString(fmt.Sprintf("| Cost | %s |\n", analyzer.FormatCost(meta.DollarCost)))
	if len(meta.ReExplFiles) > 0 {
		b.WriteString(fmt.Sprintf("| Re-explanation files | %d |\n", len(meta.ReExplFiles)))
	}
	b.WriteString("\n---\n\n")

	// Entries as turns
	turnNum := 0
	for i := from; i <= to; i++ {
		e := entries[i]

		if !e.IsConversational() {
			continue
		}

		turnNum++
		ts := ""
		if !e.Timestamp.IsZero() {
			ts = e.Timestamp.Format("2006-01-02 15:04")
		}

		b.WriteString(fmt.Sprintf("## Turn %d (entry %d) %s %s\n\n", turnNum, i, e.Type, ts))

		if e.Message == nil {
			b.WriteString("*(empty)*\n\n")
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			b.WriteString("*(parse error)*\n\n")
			continue
		}

		for _, block := range blocks {
			b.WriteString(renderBlock(block))
		}
		b.WriteString("\n")
	}

	// Footer: re-explanation files
	if len(meta.ReExplFiles) > 0 {
		b.WriteString("---\n\n")
		b.WriteString("*Re-explanation files after return:*\n")
		for _, f := range meta.ReExplFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	if err := os.WriteFile(outputPath, []byte(b.String()), 0o644); err != nil {
		return nil, fmt.Errorf("write markdown: %w", err)
	}

	return &SplitResult{
		EntriesExtracted: to - from + 1,
		TokenCost:        meta.TokenCost,
		DollarCost:       meta.DollarCost,
		TargetRepo:       meta.TargetRepo,
		ReExplFiles:      meta.ReExplFiles,
		OutputPath:       outputPath,
	}, nil
}

func renderBlock(block jsonl.ContentBlock) string {
	switch block.Type {
	case "text":
		if block.Text == "" {
			return ""
		}
		return block.Text + "\n"

	case "tool_use":
		summary := extractToolSummary(block)
		return fmt.Sprintf("```tool_use: %s\n%s\n```\n\n", block.Name, summary)

	case "tool_result":
		content := splitResultText(block.Content)
		if len(content) > 500 {
			content = content[:497] + "..."
		}
		return fmt.Sprintf("```tool_result\n%s\n```\n\n", content)

	case "image":
		size := 0
		if block.Source != nil {
			size = len(block.Source.Data) * 3 / 4 / 1024
		}
		return fmt.Sprintf("[image: %d KB]\n\n", size)

	default:
		return ""
	}
}

func extractToolSummary(block jsonl.ContentBlock) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(block.Input, &fields); err != nil {
		return "(input)"
	}

	// Try common path fields
	for _, key := range []string{"file_path", "path", "command", "pattern", "query"} {
		if raw, ok := fields[key]; ok {
			var val string
			if json.Unmarshal(raw, &val) == nil && val != "" {
				return val
			}
		}
	}

	return "(input)"
}

func splitResultText(content json.RawMessage) string {
	if content == nil {
		return "(result)"
	}
	// Reuse existing extractToolResultText from truncate.go
	if text, ok := extractToolResultText(content); ok {
		return text
	}
	return "(result)"
}

func formatTokensCompact(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
