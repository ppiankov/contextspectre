package commands

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	distillProject string
	distillCWD     bool
	distillAuto    bool
	distillDryRun  bool
	distillOutput  string
	distillFull    bool
)

var distillCmd = &cobra.Command{
	Use:   "distill",
	Short: "Distill topics from sessions into a portable context file",
	Long: `Discover all sessions for a project, segment them into topics (branches),
and generate a markdown file you can load into a new session.

Sessions are discovered by --project (substring match) or --cwd (current directory).
Topics are sorted chronologically across all matching sessions.

By default enters interactive mode: shows a numbered topic list and prompts
for selection (e.g. "0,2,4-7" or "all"). Use --auto to select all topics.

Examples:
  contextspectre distill --cwd --dry-run
  contextspectre distill --cwd --auto --output context.md
  contextspectre distill --project myapp --auto --full
  contextspectre distill --cwd --format json`,
	RunE: runDistill,
}

func runDistill(cmd *cobra.Command, args []string) error {
	if distillProject == "" && !distillCWD {
		return fmt.Errorf("--project or --cwd is required")
	}

	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}

	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	// Filter sessions by project or CWD
	filtered := filterDistillSessions(sessions, distillProject, distillCWD)
	if len(filtered) == 0 {
		if isJSON() {
			return printJSON(DistillTopicListJSON{Topics: []DistillTopicJSON{}, Total: 0})
		}
		fmt.Println("No matching sessions found.")
		return nil
	}

	// Determine project name from first matching session
	projectName := filtered[0].ProjectName
	if projectName == "" {
		projectName = "unknown"
	}

	// Parse each session and collect topics
	var inputs []analyzer.TopicSessionInput
	for _, si := range filtered {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			continue
		}
		inputs = append(inputs, analyzer.TopicSessionInput{
			Entries: entries,
			Info: analyzer.SessionInfoLite{
				SessionID: si.SessionID,
				Slug:      si.Slug,
				Created:   si.Created,
				Modified:  si.Modified,
			},
		})
	}

	ts := analyzer.CollectTopics(inputs)
	ts.ProjectName = projectName

	if len(ts.Topics) == 0 {
		if isJSON() {
			return printJSON(DistillTopicListJSON{Topics: []DistillTopicJSON{}, Total: 0})
		}
		fmt.Println("No topics found across matching sessions.")
		return nil
	}

	// Dry run: show topic list
	if distillDryRun {
		if isJSON() {
			return printJSON(buildDistillTopicListJSON(ts))
		}
		printDistillTopicList(ts)
		return nil
	}

	// Determine selected indices
	var selectedIndices []int
	if distillAuto {
		for i := range ts.Topics {
			selectedIndices = append(selectedIndices, i)
		}
	} else if isJSON() {
		// JSON mode without --auto selects all (no interactive prompt)
		for i := range ts.Topics {
			selectedIndices = append(selectedIndices, i)
		}
	} else {
		// Interactive selection
		printDistillTopicList(ts)
		fmt.Print("\nSelect topics (e.g. 0,2,4-7 or \"all\"): ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" || strings.EqualFold(input, "all") {
			for i := range ts.Topics {
				selectedIndices = append(selectedIndices, i)
			}
		} else {
			selectedIndices, err = parseNumberRanges(input, len(ts.Topics))
			if err != nil {
				return err
			}
		}
	}

	// Default output path
	if distillOutput == "" {
		distillOutput = fmt.Sprintf("distilled-%s-%s.md",
			sanitizeFilename(projectName),
			time.Now().Format("2006-01-02"))
	}

	result, err := editor.DistillToMarkdown(ts, selectedIndices, editor.DistillOpts{
		FullContent: distillFull,
		OutputPath:  distillOutput,
	})
	if err != nil {
		return fmt.Errorf("distill: %w", err)
	}

	if isJSON() {
		return printJSON(buildDistillOutputJSON(ts, selectedIndices, result))
	}

	fmt.Printf("Distilled %d topics from %d sessions to %s\n",
		result.TopicsIncluded, result.SessionsSpanned, result.OutputPath)
	fmt.Printf("  Tokens: ~%s  Cost: %s\n",
		formatTokens(result.TotalTokens),
		analyzer.FormatCost(result.TotalCost))

	return nil
}

// filterDistillSessions filters sessions by project name or CWD.
func filterDistillSessions(sessions []session.Info, projectFilter string, useCWD bool) []session.Info {
	if useCWD {
		cwd, err := os.Getwd()
		if err != nil {
			return nil
		}
		encodedDir := session.EncodePath(cwd)
		var result []session.Info
		for _, s := range sessions {
			if strings.Contains(s.FullPath, encodedDir) {
				result = append(result, s)
			}
		}
		return result
	}

	return resolveProjectSessions(sessions, projectFilter, resolveClaudeDir())
}

// printDistillTopicList prints a numbered topic list to stdout.
func printDistillTopicList(ts *analyzer.TopicSet) {
	fmt.Printf("Project: %s  (%d sessions, %d topics)\n\n",
		ts.ProjectName, len(ts.Sessions), len(ts.Topics))

	for i, t := range ts.Topics {
		sessionLabel := t.SessionSlug
		if sessionLabel == "" && len(t.SessionID) >= 8 {
			sessionLabel = t.SessionID[:8]
		}

		timeStr := ""
		if !t.Branch.TimeStart.IsZero() {
			timeStr = t.Branch.TimeStart.Format("Jan 2 15:04")
		}

		fmt.Printf("  %3d  %-12s  %-14s  %3d turns  %7s  %s\n",
			i, sessionLabel, timeStr,
			t.Branch.UserTurns,
			analyzer.FormatCost(t.CostDollars),
			t.Branch.Summary)
	}

	fmt.Printf("\nTotal: ~%s tokens, %s\n",
		formatTokens(ts.TotalTokens),
		analyzer.FormatCost(ts.TotalCost))
}

// parseNumberRanges parses "0,2,4-7" into [0,2,4,5,6,7].
func parseNumberRanges(input string, max int) ([]int, error) {
	seen := make(map[int]bool)
	var result []int

	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid range %q: %w", part, err)
			}
			hi, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid range %q: %w", part, err)
			}
			if lo < 0 || hi >= max || lo > hi {
				return nil, fmt.Errorf("range %d-%d out of bounds (0-%d)", lo, hi, max-1)
			}
			for i := lo; i <= hi; i++ {
				if !seen[i] {
					seen[i] = true
					result = append(result, i)
				}
			}
		} else {
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid number %q: %w", part, err)
			}
			if n < 0 || n >= max {
				return nil, fmt.Errorf("index %d out of range (0-%d)", n, max-1)
			}
			if !seen[n] {
				seen[n] = true
				result = append(result, n)
			}
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid indices in %q", input)
	}
	return result, nil
}

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// sanitizeFilename converts a name to a safe filename component.
func sanitizeFilename(name string) string {
	s := nonAlphaNum.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "project"
	}
	return s
}

func init() {
	distillCmd.Flags().StringVar(&distillProject, "project", "", "Filter sessions by project name (substring match)")
	distillCmd.Flags().BoolVar(&distillCWD, "cwd", false, "Use sessions for the current working directory")
	distillCmd.Flags().BoolVar(&distillAuto, "auto", false, "Select all topics (skip interactive prompt)")
	distillCmd.Flags().BoolVar(&distillDryRun, "dry-run", false, "Show topic list without generating output")
	distillCmd.Flags().StringVar(&distillOutput, "output", "", "Output file path (default: distilled-<project>-<date>.md)")
	distillCmd.Flags().BoolVar(&distillFull, "full", false, "Include full conversation content (default: summary only)")
	rootCmd.AddCommand(distillCmd)
}
