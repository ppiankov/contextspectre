package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var (
	checkpointCWD    bool
	checkpointOutput string
)

var checkpointCmd = &cobra.Command{
	Use:   "checkpoint [session-id-or-path]",
	Short: "Generate a structured resume brief from the current session state",
	Long: `Extract decisions, findings, modified files, and current goal from the
active epoch and write a structured resume brief.

Designed to survive compaction — the brief exists on disk, not in session context.
Use with status-line hooks to auto-generate before compaction hits.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCheckpoint,
}

func init() {
	checkpointCmd.Flags().BoolVar(&checkpointCWD, "cwd", false, "Use most recent session for current directory")
	checkpointCmd.Flags().StringVar(&checkpointOutput, "output", "", "Write brief to file (default: stdout)")
	rootCmd.AddCommand(checkpointCmd)
}

// CheckpointOutput is the JSON output for the checkpoint command.
type CheckpointOutput struct {
	SessionID      string          `json:"session_id"`
	Slug           string          `json:"slug,omitempty"`
	Project        string          `json:"project"`
	ClientType     string          `json:"client_type"`
	Timestamp      string          `json:"timestamp"`
	ContextPercent float64         `json:"context_percent"`
	TurnsRemaining int             `json:"turns_remaining"`
	Epoch          CheckpointEpoch `json:"active_epoch"`
	Decisions      []string        `json:"decisions"`
	Findings       []string        `json:"findings"`
	Questions      []string        `json:"questions"`
	Files          []string        `json:"files"`
	CommitPoints   []CheckpointCP  `json:"commit_points,omitempty"`
	OutputPath     string          `json:"output_path,omitempty"`
}

// CheckpointEpoch holds active epoch metadata.
type CheckpointEpoch struct {
	Index      int     `json:"index"`
	TurnCount  int     `json:"turn_count"`
	PeakTokens int     `json:"peak_tokens"`
	Cost       float64 `json:"cost"`
	Topic      string  `json:"topic"`
}

// CheckpointCP is a commit point summary.
type CheckpointCP struct {
	Goal      string   `json:"goal"`
	Decisions []string `json:"decisions"`
	Files     []string `json:"files,omitempty"`
}

func runCheckpoint(_ *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, checkpointCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	stats := analyzer.Analyze(entries)

	// Build epochs to find the active one
	activeHint := ""
	if len(entries) > 0 {
		for i := len(entries) - 1; i >= 0; i-- {
			if entries[i].Type == jsonl.TypeUser && entries[i].Message != nil {
				activeHint = entries[i].ContentPreview(60)
				break
			}
		}
	}
	epochs := analyzer.BuildEpochs(stats.EpochCosts, stats.Archaeology, activeHint)

	// Extract active epoch entries (after last compaction)
	activeStart := 0
	if stats.LastCompactionLine > 0 {
		activeStart = stats.LastCompactionLine
	}
	activeEntries := entries[activeStart:]

	// Extract decisions, findings, user questions, files from active epoch
	epochSummary := extractCheckpointData(activeEntries)

	// Get commit points from markers
	markers, _ := editor.LoadMarkers(path)
	var commitPoints []CheckpointCP
	for _, cp := range markers.CommitPoints {
		commitPoints = append(commitPoints, CheckpointCP{
			Goal:      cp.Goal,
			Decisions: cp.Decisions,
			Files:     cp.Files,
		})
	}

	// Session identity
	base := filepath.Base(path)
	sessionID := strings.TrimSuffix(base, ".jsonl")
	project := extractProjectFromPath(path)

	// Extract display name (custom title > slug)
	var sessionSlug, sessionCustomTitle string
	for _, e := range entries {
		if sessionSlug == "" && e.Slug != "" {
			sessionSlug = e.Slug
		}
		if e.CustomTitle != "" {
			sessionCustomTitle = e.CustomTitle
		}
	}
	displayName := sessionCustomTitle
	if displayName == "" {
		displayName = sessionSlug
	}

	// Active epoch data
	var activeEpoch CheckpointEpoch
	if len(epochs) > 0 {
		last := epochs[len(epochs)-1]
		activeEpoch = CheckpointEpoch{
			Index:      last.Index,
			TurnCount:  last.TurnCount,
			PeakTokens: last.PeakTokens,
			Cost:       last.Cost,
			Topic:      last.Topic,
		}
	}

	output := CheckpointOutput{
		SessionID:      sessionID,
		Slug:           displayName,
		Project:        project,
		ClientType:     stats.ClientType,
		Timestamp:      time.Now().Format(time.RFC3339),
		ContextPercent: stats.UsagePercent,
		TurnsRemaining: stats.EstimatedTurnsLeft,
		Epoch:          activeEpoch,
		Decisions:      epochSummary.decisions,
		Findings:       epochSummary.findings,
		Questions:      epochSummary.questions,
		Files:          epochSummary.files,
		CommitPoints:   commitPoints,
	}

	// Generate markdown brief
	brief := renderCheckpointBrief(output)

	// Write to file or stdout
	if checkpointOutput != "" {
		if err := os.WriteFile(checkpointOutput, []byte(brief), 0o644); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		output.OutputPath = checkpointOutput
		if isJSON() {
			return printJSON(output)
		}
		fmt.Printf("Checkpoint written to %s (%d decisions, %d findings, %d files)\n",
			checkpointOutput, len(output.Decisions), len(output.Findings), len(output.Files))
		return nil
	}

	if isJSON() {
		return printJSON(output)
	}

	fmt.Print(brief)
	return nil
}

// checkpointData holds extracted data from the active epoch.
type checkpointData struct {
	decisions []string
	findings  []string
	questions []string
	files     []string
}

// extractCheckpointData scans entries for decisions, findings, user questions, and files.
func extractCheckpointData(entries []jsonl.Entry) checkpointData {
	data := checkpointData{}
	fileSet := make(map[string]bool)

	for _, e := range entries {
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
					if hint := analyzer.ExtractDecisionHint(b.Text); hint != "" && len(data.decisions) < 20 {
						data.decisions = append(data.decisions, hint)
					}
					if finding := extractFinding(b.Text); finding != "" && len(data.findings) < 20 {
						data.findings = append(data.findings, finding)
					}
				}
			}
		}

		if e.Type == jsonl.TypeUser {
			for _, b := range blocks {
				if b.Type == "text" && len(data.questions) < 15 {
					text := strings.TrimSpace(b.Text)
					if len(text) > 5 && len(text) < 500 {
						data.questions = append(data.questions, analyzer.TruncateHint(text, 120))
					}
				}
			}
		}
	}

	for path := range fileSet {
		data.files = append(data.files, path)
	}

	return data
}

// findingKeywords are indicators of discovered facts or observations.
var findingKeywords = []string{
	"found that", "discovered", "noticed", "turns out",
	"the issue is", "the problem is", "root cause",
	"confirmed", "verified", "tested and",
}

// extractFinding returns a truncated snippet if the text contains finding keywords.
func extractFinding(text string) string {
	lower := strings.ToLower(text)
	for _, kw := range findingKeywords {
		idx := strings.Index(lower, kw)
		if idx >= 0 {
			start := idx
			if start > 20 {
				start = idx - 20
			}
			snippet := text[start:]
			return analyzer.TruncateHint(snippet, 120)
		}
	}
	return ""
}

// renderCheckpointBrief generates a structured markdown resume brief.
func renderCheckpointBrief(out CheckpointOutput) string {
	var sb strings.Builder

	heading := out.SessionID
	if out.Slug != "" {
		heading = out.Slug
	}
	fmt.Fprintf(&sb, "# Checkpoint — %s\n\n", heading)
	fmt.Fprintf(&sb, "**Project:** %s\n", out.Project)
	fmt.Fprintf(&sb, "**Client:** %s\n", out.ClientType)
	fmt.Fprintf(&sb, "**Context:** %.0f%% | **Turns left:** ~%d\n", out.ContextPercent, out.TurnsRemaining)
	fmt.Fprintf(&sb, "**Epoch:** #%d (%d turns, $%.4f)\n", out.Epoch.Index, out.Epoch.TurnCount, out.Epoch.Cost)
	fmt.Fprintf(&sb, "**Saved:** %s\n", time.Now().Format("2006-01-02 15:04"))
	if out.Slug != "" {
		fmt.Fprintf(&sb, "**Resume:** `claude --resume %q`\n", out.Slug)
	}
	sb.WriteString("\n")

	if len(out.Decisions) > 0 {
		sb.WriteString("## Decisions\n")
		for _, d := range out.Decisions {
			fmt.Fprintf(&sb, "- %s\n", d)
		}
		sb.WriteString("\n")
	}

	if len(out.Findings) > 0 {
		sb.WriteString("## Findings\n")
		for _, f := range out.Findings {
			fmt.Fprintf(&sb, "- %s\n", f)
		}
		sb.WriteString("\n")
	}

	if len(out.Questions) > 0 {
		sb.WriteString("## User Requests\n")
		for _, q := range out.Questions {
			fmt.Fprintf(&sb, "- %s\n", q)
		}
		sb.WriteString("\n")
	}

	if len(out.Files) > 0 {
		sb.WriteString("## Files Touched\n")
		for _, f := range out.Files {
			fmt.Fprintf(&sb, "- %s\n", f)
		}
		sb.WriteString("\n")
	}

	if len(out.CommitPoints) > 0 {
		sb.WriteString("## Commit Points\n")
		for _, cp := range out.CommitPoints {
			fmt.Fprintf(&sb, "- **%s**\n", cp.Goal)
			for _, d := range cp.Decisions {
				fmt.Fprintf(&sb, "  - %s\n", d)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
