package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	pruneKeep    int
	pruneMinAge  int
	pruneProject string
	pruneApply   bool
	pruneArchive bool
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Reduce sessions per project to a manageable count",
	Long: `Groups sessions by project, keeps the N most recent per project,
and inspects candidates for worthy content before deletion.

Each candidate is assigned a verdict:
  empty       — 2 or fewer messages
  throwaway   — no compactions, no commit points, cost < $0.50
  has-content — has commit points, bookmarks, or compactions
  substantive — compactions >= 2, or commit points >= 3, or cost > $5

Dry run by default. Use --apply to delete, --apply --archive to archive.`,
	RunE: runPrune,
}

func init() {
	pruneCmd.Flags().IntVar(&pruneKeep, "keep", 10, "Number of most recent sessions to keep per project")
	pruneCmd.Flags().IntVar(&pruneMinAge, "min-age", 0, "Only prune sessions older than N days (0 = no age filter)")
	pruneCmd.Flags().StringVar(&pruneProject, "project", "", "Filter to a single project name or alias")
	pruneCmd.Flags().BoolVar(&pruneApply, "apply", false, "Actually delete/archive (default: dry run)")
	pruneCmd.Flags().BoolVar(&pruneArchive, "archive", false, "Move to ~/.claude/archive/ instead of deleting (requires --apply)")
	rootCmd.AddCommand(pruneCmd)
}

// pruneVerdict classifies a session's worth.
type pruneVerdict string

const (
	verdictEmpty       pruneVerdict = "empty"
	verdictThrowaway   pruneVerdict = "throwaway"
	verdictHasContent  pruneVerdict = "has-content"
	verdictSubstantive pruneVerdict = "substantive"
)

type pruneCandidate struct {
	Info         session.Info
	AgeDays      int
	Cost         float64
	Compactions  int
	CommitPoints int
	Bookmarks    int
	ClientType   string
	Verdict      pruneVerdict
}

// PruneProjectJSON is one project group in JSON output.
type PruneProjectJSON struct {
	Project    string               `json:"project"`
	Total      int                  `json:"total"`
	Keeping    int                  `json:"keeping"`
	Pruning    int                  `json:"pruning"`
	Candidates []PruneCandidateJSON `json:"candidates"`
}

// PruneCandidateJSON is a single prune candidate in JSON output.
type PruneCandidateJSON struct {
	SessionID    string  `json:"session_id"`
	ShortID      string  `json:"short_id"`
	Slug         string  `json:"slug,omitempty"`
	AgeDays      int     `json:"age_days"`
	Messages     int     `json:"messages"`
	SizeMB       float64 `json:"size_mb"`
	Cost         float64 `json:"cost"`
	Compactions  int     `json:"compactions"`
	CommitPoints int     `json:"commit_points"`
	Bookmarks    int     `json:"bookmarks"`
	ClientType   string  `json:"client_type"`
	Verdict      string  `json:"verdict"`
}

// PruneOutputJSON is the top-level JSON output.
type PruneOutputJSON struct {
	TotalScanned    int                `json:"total_scanned"`
	TotalCandidates int                `json:"total_candidates"`
	TotalBytes      int64              `json:"total_bytes"`
	Projects        []PruneProjectJSON `json:"projects"`
}

func runPrune(_ *cobra.Command, _ []string) error {
	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}

	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if pruneProject != "" {
		sessions = resolveProjectSessions(sessions, pruneProject, dir)
	}

	// Group by project directory.
	groups := groupByProject(sessions)

	// Sort project names for stable output.
	var projectNames []string
	for name := range groups {
		projectNames = append(projectNames, name)
	}
	sort.Strings(projectNames)

	now := time.Now()
	var allCandidates []pruneCandidate
	var jsonProjects []PruneProjectJSON

	for _, projName := range projectNames {
		projSessions := groups[projName]

		// Sessions are already sorted by modified desc from ListAllSessions.
		// Determine which to keep (most recent N).
		keepCount := pruneKeep
		if keepCount > len(projSessions) {
			keepCount = len(projSessions)
		}

		var candidates []pruneCandidate
		for i := keepCount; i < len(projSessions); i++ {
			s := projSessions[i]
			ageDays := int(now.Sub(s.Modified).Hours() / 24)

			if pruneMinAge > 0 && ageDays < pruneMinAge {
				continue
			}

			c := inspectSession(s, ageDays)
			candidates = append(candidates, c)
		}

		if len(candidates) == 0 {
			continue
		}

		allCandidates = append(allCandidates, candidates...)

		jp := PruneProjectJSON{
			Project: projName,
			Total:   len(projSessions),
			Keeping: keepCount,
			Pruning: len(candidates),
		}
		for _, c := range candidates {
			jp.Candidates = append(jp.Candidates, PruneCandidateJSON{
				SessionID:    c.Info.SessionID,
				ShortID:      c.Info.ShortID(),
				Slug:         c.Info.Slug,
				AgeDays:      c.AgeDays,
				Messages:     c.Info.MessageCount,
				SizeMB:       c.Info.FileSizeMB,
				Cost:         c.Cost,
				Compactions:  c.Compactions,
				CommitPoints: c.CommitPoints,
				Bookmarks:    c.Bookmarks,
				ClientType:   c.ClientType,
				Verdict:      string(c.Verdict),
			})
		}
		jsonProjects = append(jsonProjects, jp)
	}

	var totalBytes int64
	for _, c := range allCandidates {
		totalBytes += int64(c.Info.FileSizeMB * 1024 * 1024)
	}

	if isJSON() {
		if jsonProjects == nil {
			jsonProjects = []PruneProjectJSON{}
		}
		return printJSON(PruneOutputJSON{
			TotalScanned:    len(sessions),
			TotalCandidates: len(allCandidates),
			TotalBytes:      totalBytes,
			Projects:        jsonProjects,
		})
	}

	if len(allCandidates) == 0 {
		fmt.Printf("Nothing to prune (%d sessions scanned, keeping %d per project).\n",
			len(sessions), pruneKeep)
		return nil
	}

	// Text output grouped by project.
	contentWarnings := 0
	for _, jp := range jsonProjects {
		fmt.Printf("Project: %s (%d sessions, keeping %d, pruning %d)\n\n",
			jp.Project, jp.Total, jp.Keeping, jp.Pruning)

		fmt.Printf("  %-10s %5s %5s %8s %5s %4s %4s %-12s %s\n",
			"ID", "Age", "Msgs", "Cost", "Comp", "CPs", "Bmks", "Verdict", "Slug")
		fmt.Printf("  %-10s %5s %5s %8s %5s %4s %4s %-12s %s\n",
			"----------", "-----", "-----", "--------", "-----", "----", "----", "------------", "----")

		for _, c := range jp.Candidates {
			slug := c.Slug
			if slug == "" {
				slug = "—"
			}
			if len(slug) > 30 {
				slug = slug[:30]
			}

			verdictStr := string(c.Verdict)

			fmt.Printf("  %-10s %4dd %5d %8s %5d %4d %4d %-12s %s\n",
				c.ShortID,
				c.AgeDays,
				c.Messages,
				analyzer.FormatCost(c.Cost),
				c.Compactions,
				c.CommitPoints,
				c.Bookmarks,
				verdictStr,
				slug)

			if c.Verdict == string(verdictHasContent) || c.Verdict == string(verdictSubstantive) {
				contentWarnings++
			}
		}
		fmt.Println()
	}

	fmt.Printf("Total: %d candidates (%.1f MB)\n", len(allCandidates), float64(totalBytes)/1024/1024)

	if contentWarnings > 0 {
		fmt.Printf("\n⚠ %d session(s) marked has-content or substantive — review before pruning\n", contentWarnings)
	}

	if !pruneApply {
		fmt.Println("\nDry run — use --apply to delete, or --apply --archive to archive.")
		return nil
	}

	// Apply deletion/archival.
	archiveDir := ""
	action := "Deleted"
	if pruneArchive {
		archiveDir = filepath.Join(dir, "archive")
		action = "Archived"
	}

	removed := 0
	for _, c := range allCandidates {
		if pruneArchive {
			if err := archiveSession(c.Info.FullPath, archiveDir); err != nil {
				fmt.Fprintf(os.Stderr, "  Error archiving %s: %v\n", c.Info.ShortID(), err)
				continue
			}
		} else {
			if err := os.Remove(c.Info.FullPath); err != nil {
				fmt.Fprintf(os.Stderr, "  Error removing %s: %v\n", c.Info.ShortID(), err)
				continue
			}
			// Also remove sidecar markers file if it exists.
			markersPath := c.Info.FullPath + ".markers.json"
			_ = os.Remove(markersPath)
		}
		removed++
	}

	fmt.Printf("\n%s %d sessions (%.1f MB freed).\n", action, removed, float64(totalBytes)/1024/1024)
	return nil
}

// inspectSession performs a lightweight inspection of a session to determine its worth.
func inspectSession(s session.Info, ageDays int) pruneCandidate {
	c := pruneCandidate{
		Info:    s,
		AgeDays: ageDays,
	}

	// Get stats from QuickStats if available.
	if s.ContextStats != nil {
		c.Compactions = s.ContextStats.CompactionCount
		c.Cost = s.ContextStats.EstimatedCost
		c.ClientType = s.ContextStats.ClientType
	}

	// If no client type from QuickStats, do a lightweight scan.
	if c.ClientType == "" {
		stats, err := jsonl.ScanLight(s.FullPath)
		if err == nil {
			c.Compactions = stats.CompactionCount
			c.ClientType = clientTypeFromLight(stats)
			if c.Cost == 0 && stats.LastUsage != nil {
				u := stats.LastUsage
				c.Cost = analyzer.QuickCost(u.InputTokens, u.OutputTokens,
					u.CacheCreationInputTokens, u.CacheReadInputTokens, stats.Model)
			}
		}
	}

	// Check markers (commit points, bookmarks) — reads small sidecar file.
	markers, err := editor.LoadMarkers(s.FullPath)
	if err == nil {
		c.CommitPoints = len(markers.CommitPoints)
		c.Bookmarks = len(markers.Bookmarks)
	}

	// Assign verdict.
	c.Verdict = classifyVerdict(c)
	return c
}

func clientTypeFromLight(stats *jsonl.LightStats) string {
	snapshotCount := stats.TypeCounts[jsonl.TypeFileHistorySnapshot]
	if snapshotCount > 0 {
		return "cli"
	}
	if stats.StartsWithQueueOp {
		return "desktop"
	}
	if stats.LineCount > 100 {
		return "cli"
	}
	return "unknown"
}

func classifyVerdict(c pruneCandidate) pruneVerdict {
	if c.Info.MessageCount <= 2 {
		return verdictEmpty
	}
	if c.Compactions >= 2 || c.CommitPoints >= 3 || c.Cost > 5.0 {
		return verdictSubstantive
	}
	if c.Compactions > 0 || c.CommitPoints > 0 || c.Bookmarks > 0 {
		return verdictHasContent
	}
	if c.Cost < 0.50 {
		return verdictThrowaway
	}
	return verdictThrowaway
}

// groupByProject groups sessions by their project directory name.
func groupByProject(sessions []session.Info) map[string][]session.Info {
	groups := make(map[string][]session.Info)
	for _, s := range sessions {
		proj := s.ProjectName
		if proj == "" {
			proj = "unknown"
		}
		groups[proj] = append(groups[proj], s)
	}
	return groups
}
