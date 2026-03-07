package commands

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	efficiencyCWD     bool
	efficiencyProject string
)

var efficiencyCmd = &cobra.Command{
	Use:   "repo-efficiency",
	Short: "Show reasoning efficiency: tokens per LOC, session-commit correlation",
	Long: `Correlate sessions with git commits to measure reasoning-to-code yield.

  contextspectre repo-efficiency --cwd
  contextspectre repo-efficiency --project myapp`,
	RunE: runEfficiency,
}

func runEfficiency(cmd *cobra.Command, args []string) error {
	if !efficiencyCWD && efficiencyProject == "" {
		return fmt.Errorf("specify --cwd or --project <name>")
	}

	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if efficiencyCWD {
		sessions = filterDistillSessions(sessions, "", true)
	} else if efficiencyProject != "" {
		sessions = resolveProjectSessions(sessions, efficiencyProject, dir)
	}

	if len(sessions) == 0 {
		if isJSON() {
			return printJSON(analyzer.EfficiencyResult{
				Sessions:        []analyzer.SessionEfficiency{},
				TopEfficient:    []analyzer.SessionEfficiency{},
				LowYield:        []analyzer.SessionEfficiency{},
				ReasoningSinks:  []analyzer.SessionEfficiency{},
				PackageHotspots: []analyzer.PackageHotspot{},
			})
		}
		fmt.Println("No sessions found.")
		return nil
	}

	// Build session inputs.
	var earliest time.Time
	var inputs []analyzer.SessionEffInput
	for _, si := range sessions {
		stats, err := jsonl.ScanLight(si.FullPath)
		if err != nil {
			continue
		}

		input := stats.TotalInputTokens + stats.TotalCacheWriteTokens + stats.TotalCacheReadTokens
		output := stats.TotalOutputTokens
		cost := analyzer.QuickCost(
			stats.TotalInputTokens, stats.TotalOutputTokens,
			stats.TotalCacheWriteTokens, stats.TotalCacheReadTokens,
			stats.Model,
		)

		created := si.Created
		modified := si.Modified
		// When sessions-index.json is missing, Created is zero.
		// Use JSONL timestamps as fallback.
		if created.IsZero() && !stats.FirstTimestamp.IsZero() {
			created = stats.FirstTimestamp
		}
		if created.IsZero() {
			created = modified
		}
		if !stats.LastTimestamp.IsZero() && stats.LastTimestamp.After(modified) {
			modified = stats.LastTimestamp
		}
		if earliest.IsZero() || created.Before(earliest) {
			earliest = created
		}

		inputs = append(inputs, analyzer.SessionEffInput{
			SessionID:   si.SessionID,
			Slug:        si.DisplayName(),
			Created:     created,
			Modified:    modified,
			TotalTokens: input + output,
			Cost:        cost,
		})
	}

	// Determine git directory from session project path or CWD.
	gitDir := ""
	if efficiencyCWD {
		gitDir = "."
	} else if len(sessions) > 0 && sessions[0].ProjectPath != "" {
		gitDir = sessions[0].ProjectPath
	}

	// Get git commits.
	var commits []analyzer.GitCommit
	if gitDir != "" && !earliest.IsZero() {
		commits, _ = parseGitLog(gitDir, earliest)
	}

	result := analyzer.ComputeEfficiency(inputs, commits)

	if isJSON() {
		return printJSON(result)
	}

	renderEfficiencyText(result, sessions[0].ProjectName)
	return nil
}

// parseGitLog runs git log --numstat and parses the output into GitCommit structs.
func parseGitLog(dir string, since time.Time) ([]analyzer.GitCommit, error) {
	sinceStr := since.Format("2006-01-02")
	cmd := exec.Command("git", "log", "--numstat", "--format=COMMIT %H %aI",
		"--since="+sinceStr, "--all")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var commits []analyzer.GitCommit
	var current *analyzer.GitCommit

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "COMMIT ") {
			if current != nil {
				commits = append(commits, *current)
			}
			parts := strings.SplitN(line, " ", 3)
			if len(parts) < 3 {
				current = nil
				continue
			}
			ts, err := time.Parse(time.RFC3339, parts[2])
			if err != nil {
				current = nil
				continue
			}
			current = &analyzer.GitCommit{
				Hash:      parts[1],
				Timestamp: ts,
			}
			continue
		}

		if current == nil || strings.TrimSpace(line) == "" {
			continue
		}

		// numstat line: "added\tdeleted\tpath"
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		added, err1 := strconv.Atoi(fields[0])
		deleted, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue // binary files show "-\t-\tpath"
		}
		current.Files = append(current.Files, analyzer.GitFileStat{
			Path:    fields[2],
			Added:   added,
			Deleted: deleted,
		})
	}
	if current != nil {
		commits = append(commits, *current)
	}

	return commits, nil
}

func renderEfficiencyText(r *analyzer.EfficiencyResult, projectName string) {
	fmt.Printf("Project: %s\n\n", projectName)
	fmt.Printf("Reasoning Efficiency\n")
	fmt.Printf("  Total tokens:     %s\n", formatTokenCount(r.TotalTokens))
	fmt.Printf("  Total LOC:        %d\n", r.TotalLOC)
	if r.TotalLOC > 0 {
		fmt.Printf("  Tokens per LOC:   %s\n", formatTokenCount(int(r.TokensPerLOC)))
	} else {
		fmt.Printf("  Tokens per LOC:   (no commits correlated)\n")
	}

	if len(r.TopEfficient) > 0 {
		fmt.Printf("\nTop efficient sessions\n")
		for _, s := range r.TopEfficient {
			fmt.Printf("  %-28s %5d LOC / %8s tokens  (%.2f LOC/Ktok)\n",
				s.Slug, s.LOCAdded, formatTokenCount(s.TotalTokens), s.Efficiency)
		}
	}

	if len(r.ReasoningSinks) > 0 {
		fmt.Printf("\nReasoning sinks (no commits)\n")
		for _, s := range r.ReasoningSinks {
			fmt.Printf("  %-28s %8s tokens  %s\n",
				s.Slug, formatTokenCount(s.TotalTokens), analyzer.FormatCost(s.Cost))
		}
	}

	if len(r.LowYield) > 0 {
		fmt.Printf("\nLow-yield sessions\n")
		for _, s := range r.LowYield {
			fmt.Printf("  %-28s %5d LOC / %8s tokens  (%.2f LOC/Ktok)\n",
				s.Slug, s.LOCAdded, formatTokenCount(s.TotalTokens), s.Efficiency)
		}
	}

	if len(r.PackageHotspots) > 0 {
		fmt.Printf("\nPackage hotspots (by reasoning cost)\n")
		limit := 10
		if len(r.PackageHotspots) < limit {
			limit = len(r.PackageHotspots)
		}
		for i := 0; i < limit; i++ {
			h := r.PackageHotspots[i]
			fmt.Printf("  %-32s %8s tokens  %4d LOC  %s\n",
				h.Package, formatTokenCount(h.TotalTokens), h.TotalLOC,
				analyzer.FormatCost(h.Cost))
		}
	}
}

func init() {
	efficiencyCmd.Flags().BoolVar(&efficiencyCWD, "cwd", false, "Use sessions for current working directory")
	efficiencyCmd.Flags().StringVar(&efficiencyProject, "project", "", "Filter by project name or alias")
	rootCmd.AddCommand(efficiencyCmd)
}
