package editor

import (
	"fmt"
	"os"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// ExportResult holds the result of a branch export operation.
type ExportResult struct {
	BranchesExported int
	EntriesExtracted int
	TokenCost        int
	DollarCost       float64
	OutputPath       string
}

// ExportBranches writes selected branches to a markdown file.
// selectedIndices are branch indices (0-based) into the branches slice.
// If selectedIndices is empty, all branches are exported.
func ExportBranches(entries []jsonl.Entry, branches []analyzer.Branch, selectedIndices []int, sessionID, outputPath string) (*ExportResult, error) {
	if len(branches) == 0 {
		return nil, fmt.Errorf("no branches to export")
	}

	// Default: export all
	if len(selectedIndices) == 0 {
		selectedIndices = make([]int, len(branches))
		for i := range branches {
			selectedIndices[i] = i
		}
	}

	// Validate indices
	for _, idx := range selectedIndices {
		if idx < 0 || idx >= len(branches) {
			return nil, fmt.Errorf("branch index %d out of range (0-%d)", idx, len(branches)-1)
		}
	}

	cwd := analyzer.DetectSessionCWD(entries)
	result := &ExportResult{
		BranchesExported: len(selectedIndices),
		OutputPath:       outputPath,
	}

	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "# Branch Export — %s\n\n", sessionID)

	// Summary table
	totalEntries := 0
	totalTokens := 0
	totalCost := 0.0
	for _, idx := range selectedIndices {
		br := branches[idx]
		totalEntries += br.EntryCount
		totalTokens += br.TokenCost
		meta := analyzer.ComputeRangeMetadata(entries, br.StartIdx, br.EndIdx, cwd)
		totalCost += meta.DollarCost
	}
	result.EntriesExtracted = totalEntries
	result.TokenCost = totalTokens
	result.DollarCost = totalCost

	b.WriteString("| Metric | Value |\n")
	b.WriteString("|--------|-------|\n")
	fmt.Fprintf(&b, "| Branches | %d |\n", len(selectedIndices))
	fmt.Fprintf(&b, "| Entries | %d |\n", totalEntries)
	fmt.Fprintf(&b, "| Tokens | ~%s |\n", formatTokensCompact(totalTokens))
	fmt.Fprintf(&b, "| Cost | %s |\n", analyzer.FormatCost(totalCost))
	b.WriteString("\n---\n\n")

	// Render each branch
	var allReExplFiles []string
	for _, idx := range selectedIndices {
		br := branches[idx]

		// Branch header
		timeRange := "—"
		if !br.TimeStart.IsZero() {
			timeRange = fmt.Sprintf("%s – %s",
				br.TimeStart.Format("2006-01-02 15:04"),
				br.TimeEnd.Format("2006-01-02 15:04"))
		}

		fmt.Fprintf(&b, "## Branch #%d: %s\n\n", br.Index, br.Summary)
		fmt.Fprintf(&b, "- **Time:** %s\n", timeRange)
		fmt.Fprintf(&b, "- **Entries:** %d (%d user turns)\n", br.EntryCount, br.UserTurns)
		fmt.Fprintf(&b, "- **Tokens:** ~%s\n", formatTokensCompact(br.TokenCost))
		fmt.Fprintf(&b, "- **Files:** %d\n\n", br.FileCount)

		// Conversational entries
		turnNum := 0
		for i := br.StartIdx; i <= br.EndIdx && i < len(entries); i++ {
			e := entries[i]
			if !e.IsConversational() {
				continue
			}

			turnNum++
			ts := ""
			if !e.Timestamp.IsZero() {
				ts = e.Timestamp.Format("15:04")
			}
			fmt.Fprintf(&b, "### Turn %d (%s) %s\n\n", turnNum, e.Type, ts)

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

		// Collect re-explanation files
		meta := analyzer.ComputeRangeMetadata(entries, br.StartIdx, br.EndIdx, cwd)
		allReExplFiles = append(allReExplFiles, meta.ReExplFiles...)

		b.WriteString("---\n\n")
	}

	// Footer: re-explanation files
	if len(allReExplFiles) > 0 {
		// Deduplicate
		seen := make(map[string]bool)
		var unique []string
		for _, f := range allReExplFiles {
			if !seen[f] {
				seen[f] = true
				unique = append(unique, f)
			}
		}
		b.WriteString("*Re-explanation files after return:*\n")
		for _, f := range unique {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}

	if err := os.WriteFile(outputPath, []byte(b.String()), 0o644); err != nil {
		return nil, fmt.Errorf("write markdown: %w", err)
	}

	return result, nil
}

// ExportBranchesPreview returns token and cost totals without writing a file.
func ExportBranchesPreview(entries []jsonl.Entry, branches []analyzer.Branch, selectedIndices []int) (int, float64) {
	if len(selectedIndices) == 0 {
		selectedIndices = make([]int, len(branches))
		for i := range branches {
			selectedIndices[i] = i
		}
	}

	cwd := analyzer.DetectSessionCWD(entries)
	totalTokens := 0
	totalCost := 0.0
	for _, idx := range selectedIndices {
		if idx < 0 || idx >= len(branches) {
			continue
		}
		br := branches[idx]
		totalTokens += br.TokenCost
		meta := analyzer.ComputeRangeMetadata(entries, br.StartIdx, br.EndIdx, cwd)
		totalCost += meta.DollarCost
	}
	return totalTokens, totalCost
}
