package editor

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// DistillOpts controls distillation output.
type DistillOpts struct {
	FullContent bool
	OutputPath  string
}

// DistillResult holds the output of a distill operation.
type DistillResult struct {
	TopicsIncluded  int
	SessionsSpanned int
	TotalTokens     int
	TotalCost       float64
	OutputPath      string
}

// DistillToMarkdown renders selected topics to a markdown file.
func DistillToMarkdown(ts *analyzer.TopicSet, selectedIndices []int, opts DistillOpts) (*DistillResult, error) {
	if len(selectedIndices) == 0 {
		for i := range ts.Topics {
			selectedIndices = append(selectedIndices, i)
		}
	}

	for _, idx := range selectedIndices {
		if idx < 0 || idx >= len(ts.Topics) {
			return nil, fmt.Errorf("topic index %d out of range (0-%d)", idx, len(ts.Topics)-1)
		}
	}

	result := &DistillResult{
		TopicsIncluded: len(selectedIndices),
		OutputPath:     opts.OutputPath,
	}

	sessionSet := make(map[string]bool)
	for _, idx := range selectedIndices {
		t := ts.Topics[idx]
		sessionSet[t.SessionID] = true
		result.TotalTokens += t.Branch.TokenCost
		result.TotalCost += t.CostDollars
	}
	result.SessionsSpanned = len(sessionSet)

	var b strings.Builder

	// Project header
	fmt.Fprintf(&b, "# Project Context — %s\n\n", ts.ProjectName)
	fmt.Fprintf(&b, "Distilled %s from %d sessions (%d topics selected)\n\n",
		time.Now().Format("2006-01-02 15:04"),
		len(ts.Sessions), len(selectedIndices))

	b.WriteString("| Metric | Value |\n")
	b.WriteString("|--------|-------|\n")
	fmt.Fprintf(&b, "| Topics | %d |\n", len(selectedIndices))
	fmt.Fprintf(&b, "| Sessions | %d |\n", result.SessionsSpanned)
	fmt.Fprintf(&b, "| Tokens | ~%s |\n", distillFormatTokens(result.TotalTokens))
	fmt.Fprintf(&b, "| Cost | %s |\n", analyzer.FormatCost(result.TotalCost))
	b.WriteString("\n")

	// Session history
	b.WriteString("## Session History\n\n")
	for _, si := range ts.Sessions {
		label := si.Slug
		if label == "" && len(si.SessionID) >= 8 {
			label = si.SessionID[:8]
		}
		fmt.Fprintf(&b, "- **%s** (%s – %s) %d topics, %s\n",
			label,
			si.Created.Format("Jan 2"),
			si.Modified.Format("Jan 2"),
			si.TopicCount,
			analyzer.FormatCost(si.Cost))
	}
	b.WriteString("\n---\n\n")

	// Topics
	for seqNum, idx := range selectedIndices {
		t := ts.Topics[idx]
		br := t.Branch

		sessionLabel := t.SessionSlug
		if sessionLabel == "" && len(t.SessionID) >= 8 {
			sessionLabel = t.SessionID[:8]
		}

		timeRange := ""
		if !br.TimeStart.IsZero() {
			timeRange = fmt.Sprintf("%s – %s",
				br.TimeStart.Format("2006-01-02 15:04"),
				br.TimeEnd.Format("2006-01-02 15:04"))
		}

		fmt.Fprintf(&b, "## Topic %d: %s\n\n", seqNum+1, br.Summary)
		fmt.Fprintf(&b, "- **Session:** %s\n", sessionLabel)
		if timeRange != "" {
			fmt.Fprintf(&b, "- **Time:** %s\n", timeRange)
		}
		fmt.Fprintf(&b, "- **Turns:** %d user, %d entries\n", br.UserTurns, br.EntryCount)
		fmt.Fprintf(&b, "- **Files:** %d\n", br.FileCount)
		fmt.Fprintf(&b, "- **Cost:** %s\n\n", analyzer.FormatCost(t.CostDollars))

		// Compaction summary
		if t.Compaction != nil {
			if len(t.Compaction.Before.UserQuestions) > 0 {
				b.WriteString("**Questions asked:**\n")
				for _, q := range t.Compaction.Before.UserQuestions {
					fmt.Fprintf(&b, "- %s\n", q)
				}
				b.WriteString("\n")
			}
			if len(t.Compaction.Before.DecisionHints) > 0 {
				b.WriteString("**Decisions made:**\n")
				for _, d := range t.Compaction.Before.DecisionHints {
					fmt.Fprintf(&b, "- %s\n", d)
				}
				b.WriteString("\n")
			}
			if t.Compaction.After.SummaryText != "" {
				summary := t.Compaction.After.SummaryText
				if len(summary) > 500 {
					summary = summary[:497] + "..."
				}
				fmt.Fprintf(&b, "**Compaction summary:** %s\n\n", summary)
			}
		}

		if opts.FullContent {
			renderFullTopicContent(&b, t.Entries)
		}

		b.WriteString("---\n\n")
	}

	if err := os.WriteFile(opts.OutputPath, []byte(b.String()), 0o644); err != nil {
		return nil, fmt.Errorf("write distill output: %w", err)
	}

	return result, nil
}

func renderFullTopicContent(b *strings.Builder, entries []jsonl.Entry) {
	turnNum := 0
	for _, e := range entries {
		if !e.IsConversational() {
			continue
		}
		turnNum++
		ts := ""
		if !e.Timestamp.IsZero() {
			ts = e.Timestamp.Format("15:04")
		}
		fmt.Fprintf(b, "### Turn %d (%s) %s\n\n", turnNum, e.Type, ts)

		if e.Message == nil {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		for _, block := range blocks {
			b.WriteString(renderBlock(block))
		}
		b.WriteString("\n")
	}
}

func distillFormatTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
