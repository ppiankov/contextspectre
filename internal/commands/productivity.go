package commands

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/savings"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	productivityCWD     bool
	productivityProject string
)

var productivityCmd = &cobra.Command{
	Use:   "productivity",
	Short: "Show operator productivity metrics correlated with session economics",
	Long: `Correlate session cost and token data with git commits and cleanup savings
to measure actual throughput.

  contextspectre productivity --cwd
  contextspectre productivity --project myapp`,
	RunE: runProductivity,
}

// ProductivityOutput holds the JSON output for the productivity command.
type ProductivityOutput struct {
	Project        string        `json:"project"`
	Sessions       int           `json:"sessions"`
	TotalCost      float64       `json:"total_cost"`
	TotalCommits   int           `json:"total_commits"`
	TotalHours     float64       `json:"total_hours"`
	CostPerCommit  float64       `json:"cost_per_commit"`
	CommitsPerHour float64       `json:"commits_per_hour"`
	WasteRatio     float64       `json:"waste_ratio"`
	TotalCleaned   int           `json:"total_tokens_cleaned"`
	TotalTokens    int           `json:"total_tokens"`
	PerSession     []SessionProd `json:"per_session"`
}

// SessionProd holds per-session productivity data.
type SessionProd struct {
	SessionID     string  `json:"session_id"`
	Slug          string  `json:"slug,omitempty"`
	Cost          float64 `json:"cost"`
	Commits       int     `json:"commits"`
	Hours         float64 `json:"hours"`
	CostPerCommit float64 `json:"cost_per_commit"`
	TokensCleaned int     `json:"tokens_cleaned"`
}

func runProductivity(cmd *cobra.Command, args []string) error {
	if !productivityCWD && productivityProject == "" {
		return fmt.Errorf("specify --cwd or --project <name>")
	}

	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if productivityCWD {
		sessions = filterDistillSessions(sessions, "", true)
	} else if productivityProject != "" {
		sessions = resolveProjectSessions(sessions, productivityProject, dir)
	}

	if len(sessions) == 0 {
		if isJSON() {
			return printJSON(ProductivityOutput{PerSession: []SessionProd{}})
		}
		fmt.Println("No sessions found.")
		return nil
	}

	// Load savings events.
	savingsEvents, _ := savings.Load(dir)
	savingsMap := make(map[string]int) // session_id → tokens cleaned
	for _, ev := range savingsEvents {
		savingsMap[ev.SessionID] += ev.TokensRemoved
	}

	// Get git directory.
	gitDir := ""
	if productivityCWD {
		gitDir = "."
	} else if sessions[0].ProjectPath != "" {
		gitDir = sessions[0].ProjectPath
	}

	// Build per-session data.
	var earliest time.Time
	type sessionData struct {
		si       session.Info
		stats    *jsonl.LightStats
		cost     float64
		tokens   int
		created  time.Time
		modified time.Time
	}
	var sData []sessionData

	for _, si := range sessions {
		stats, err := jsonl.ScanLight(si.FullPath)
		if err != nil {
			continue
		}

		cost := analyzer.QuickCost(
			stats.TotalInputTokens, stats.TotalOutputTokens,
			stats.TotalCacheWriteTokens, stats.TotalCacheReadTokens,
			stats.Model,
		)
		tokens := stats.TotalInputTokens + stats.TotalOutputTokens +
			stats.TotalCacheWriteTokens + stats.TotalCacheReadTokens

		created := si.Created
		if created.IsZero() && !stats.FirstTimestamp.IsZero() {
			created = stats.FirstTimestamp
		}
		if created.IsZero() {
			created = si.Modified
		}
		modified := si.Modified
		if !stats.LastTimestamp.IsZero() && stats.LastTimestamp.After(modified) {
			modified = stats.LastTimestamp
		}

		if earliest.IsZero() || created.Before(earliest) {
			earliest = created
		}

		sData = append(sData, sessionData{
			si:       si,
			stats:    stats,
			cost:     cost,
			tokens:   tokens,
			created:  created,
			modified: modified,
		})
	}

	// Get git commits.
	var commits []gitCommitBasic
	if gitDir != "" && !earliest.IsZero() {
		commits, _ = parseGitLogBasic(gitDir, earliest)
	}

	// Correlate and compute.
	out := ProductivityOutput{
		Project: sessions[0].ProjectName,
	}
	var totalCost float64
	var totalCommits int
	var totalHours float64
	var totalCleaned int
	var totalTokens int

	for _, sd := range sData {
		out.Sessions++

		sessionCommits := 0
		for _, c := range commits {
			if !c.Timestamp.Before(sd.created) && !c.Timestamp.After(sd.modified.Add(30*time.Minute)) {
				sessionCommits++
			}
		}

		hours := sd.modified.Sub(sd.created).Hours()
		if hours < 0.01 {
			hours = 0.01
		}

		cleaned := savingsMap[sd.si.SessionID]

		cpc := 0.0
		if sessionCommits > 0 {
			cpc = sd.cost / float64(sessionCommits)
		}

		totalCost += sd.cost
		totalCommits += sessionCommits
		totalHours += hours
		totalCleaned += cleaned
		totalTokens += sd.tokens

		out.PerSession = append(out.PerSession, SessionProd{
			SessionID:     sd.si.SessionID,
			Slug:          sd.si.DisplayName(),
			Cost:          sd.cost,
			Commits:       sessionCommits,
			Hours:         hours,
			CostPerCommit: cpc,
			TokensCleaned: cleaned,
		})
	}

	out.TotalCost = totalCost
	out.TotalCommits = totalCommits
	out.TotalHours = totalHours
	out.TotalCleaned = totalCleaned
	out.TotalTokens = totalTokens

	if totalCommits > 0 {
		out.CostPerCommit = totalCost / float64(totalCommits)
	}
	if totalHours > 0 {
		out.CommitsPerHour = float64(totalCommits) / totalHours
	}
	if totalTokens > 0 {
		out.WasteRatio = float64(totalCleaned) / float64(totalTokens)
	}

	if isJSON() {
		return printJSON(out)
	}

	renderProductivityText(&out)
	return nil
}

type gitCommitBasic struct {
	Hash      string
	Timestamp time.Time
}

func parseGitLogBasic(dir string, since time.Time) ([]gitCommitBasic, error) {
	sinceStr := since.Format("2006-01-02")
	cmd := exec.Command("git", "log", "--format=%H %aI", "--since="+sinceStr, "--all")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []gitCommitBasic
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		ts, err := time.Parse(time.RFC3339, parts[1])
		if err != nil {
			continue
		}
		commits = append(commits, gitCommitBasic{Hash: parts[0], Timestamp: ts})
	}
	return commits, nil
}

func renderProductivityText(out *ProductivityOutput) {
	fmt.Printf("Project: %s\n\n", out.Project)
	fmt.Printf("Productivity Metrics\n")
	fmt.Printf("  Sessions:         %d\n", out.Sessions)
	fmt.Printf("  Total cost:       %s\n", analyzer.FormatCost(out.TotalCost))
	fmt.Printf("  Total commits:    %d\n", out.TotalCommits)
	fmt.Printf("  Total hours:      %.1f\n", out.TotalHours)
	if out.TotalCommits > 0 {
		fmt.Printf("  Cost per commit:  %s\n", analyzer.FormatCost(out.CostPerCommit))
		fmt.Printf("  Commits per hour: %.1f\n", out.CommitsPerHour)
	}
	fmt.Printf("  Waste ratio:      %.1f%%\n", out.WasteRatio*100)
	if out.TotalCleaned > 0 {
		fmt.Printf("  Tokens cleaned:   %s\n", formatTokenCount(out.TotalCleaned))
	}

	if len(out.PerSession) > 0 {
		fmt.Printf("\nPer-session breakdown\n")
		for _, s := range out.PerSession {
			cpc := "—"
			if s.Commits > 0 {
				cpc = analyzer.FormatCost(s.CostPerCommit)
			}
			fmt.Printf("  %-28s  %s  %3d commits  %.1fh  %s/commit\n",
				s.Slug, analyzer.FormatCost(s.Cost), s.Commits, s.Hours, cpc)
		}
	}
}

func init() {
	productivityCmd.Flags().BoolVar(&productivityCWD, "cwd", false, "Use sessions for current working directory")
	productivityCmd.Flags().StringVar(&productivityProject, "project", "", "Filter by project name or alias")
	rootCmd.AddCommand(productivityCmd)
}
