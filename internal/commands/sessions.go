package commands

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ppiankov/contextspectre/internal/project"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	sessionsActive  bool
	sessionsProject string
	sessionsCWD     bool
	sessionsAll     bool
	sessionsLimit   int
)

var sessionsCmd = &cobra.Command{
	Use:     "sessions",
	Aliases: []string{"list"},
	Short:   "List all conversation sessions",
	RunE:    runSessions,
}

func runSessions(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}

	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	// Apply filters
	sessions = filterSessions(sessions)
	total := len(sessions)

	// Apply limit (default 20 unless --all or explicit filters are set).
	if !sessionsAll && sessionsLimit > 0 && !sessionsActive && sessionsProject == "" && !sessionsCWD && total > sessionsLimit {
		sessions = sessions[:sessionsLimit]
	}

	if total == 0 {
		if isJSON() {
			return printJSON(SessionsOutput{Sessions: []SessionJSON{}, Total: 0})
		}
		fmt.Println("No sessions found.")
		return nil
	}

	if isJSON() {
		out := SessionsOutput{Total: len(sessions)}
		for _, s := range sessions {
			sj := SessionJSON{
				ID:            s.SessionID,
				Slug:          s.Slug,
				Project:       s.ProjectPath,
				Branch:        s.GitBranch,
				Messages:      s.MessageCount,
				FileSizeBytes: int64(s.FileSizeMB * 1024 * 1024),
				LastModified:  s.Modified,
				Active:        s.IsActive(),
			}
			if sj.Project == "" {
				sj.Project = s.ProjectName
			}
			if s.ContextStats != nil {
				sj.Tokens = s.ContextStats.ContextTokens
				sj.ContextPercent = s.ContextStats.ContextPct
				sj.Compactions = s.ContextStats.CompactionCount
				sj.Images = s.ContextStats.ImageCount
				sj.EstimatedCost = s.ContextStats.EstimatedCost
				sj.Model = s.ContextStats.Model
				sj.ClientType = s.ContextStats.ClientType
				if s.ContextStats.ContextTokens > 0 {
					sp := s.ContextStats.SignalPercent
					sj.SignalPercent = &sp
					es := s.ContextStats.EntropyScore
					sj.EntropyScore = &es
					sj.EntropyLevel = s.ContextStats.EntropyLevel
					sj.CleanupStatus = s.ContextStats.CleanupStatus
					cs := s.ContextStats.CleanupCadenceScore
					sj.CadenceScore = &cs
				}
			}
			// Zombie state cached from discovery
			sj.Zombie = s.Zombie
			sj.ZombieReason = s.ZombieReason
			out.Sessions = append(out.Sessions, sj)
		}
		return printJSON(out)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tSLUG\tID\tBRANCH\tMSGS\tSIZE\tCONTEXT\tMODIFIED")
	fmt.Fprintln(w, "───────\t────\t──\t──────\t────\t────\t───────\t────────")

	for _, s := range sessions {
		prefix := ""
		if s.IsActive() {
			prefix = "[ACTIVE] "
		}
		if s.Zombie {
			prefix = "[ZOMBIE] "
		}

		contextStr := "—"
		if s.ContextStats != nil && s.ContextStats.ContextTokens > 0 {
			contextStr = fmt.Sprintf("%.1f%%", s.ContextStats.ContextPct)
		}

		branch := s.GitBranch
		if branch == "" {
			branch = "—"
		}

		slug := s.Slug
		if slug == "" {
			slug = "—"
		}

		fmt.Fprintf(w, "%s%s\t%s\t%s\t%s\t%d\t%.1f MB\t%s\t%s\n",
			prefix,
			s.ProjectName,
			slug,
			s.ShortID(),
			branch,
			s.MessageCount,
			s.FileSizeMB,
			contextStr,
			timeAgo(s.Modified),
		)
	}
	_ = w.Flush()

	// Show truncation hint when output was capped.
	if len(sessions) < total {
		fmt.Printf("\nShowing %d of %d sessions. Use --all to see everything.\n", len(sessions), total)
	}

	return nil
}

// filterSessions applies --active, --project, and --cwd flags.
func filterSessions(sessions []session.Info) []session.Info {
	if !sessionsActive && sessionsProject == "" && !sessionsCWD {
		return sessions
	}

	// Apply --cwd filter: match sessions whose path contains the encoded CWD
	if sessionsCWD {
		cwd, err := os.Getwd()
		if err == nil {
			encoded := session.EncodePath(cwd)
			var filtered []session.Info
			for _, s := range sessions {
				if strings.Contains(s.FullPath, encoded) {
					filtered = append(filtered, s)
				}
			}
			sessions = filtered
		}
	}

	// Apply project filter (alias-aware)
	if sessionsProject != "" {
		sessions = resolveProjectSessions(sessions, sessionsProject, resolveClaudeDir())
	}

	// Apply active filter
	if sessionsActive {
		var filtered []session.Info
		for _, s := range sessions {
			if time.Since(s.Modified) <= 5*time.Minute {
				filtered = append(filtered, s)
			}
		}
		return filtered
	}

	return sessions
}

// resolveProjectSessions filters sessions by project flag.
// If projectFlag matches an alias exactly, filters by encoded paths.
// Otherwise falls back to substring match on ProjectName/ProjectPath.
func resolveProjectSessions(sessions []session.Info, projectFlag, claudeDir string) []session.Info {
	if projectFlag == "" {
		return sessions
	}

	// Try exact alias match
	cfg, err := project.Load(claudeDir)
	if err == nil {
		if paths := cfg.Resolve(projectFlag); paths != nil {
			var result []session.Info
			for _, s := range sessions {
				for _, p := range paths {
					if strings.Contains(s.FullPath, session.EncodePath(p)) {
						result = append(result, s)
						break
					}
				}
			}
			return result
		}
	}

	// Fallback: substring match
	proj := strings.ToLower(projectFlag)
	var result []session.Info
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.ProjectName), proj) ||
			strings.Contains(strings.ToLower(s.ProjectPath), proj) {
			result = append(result, s)
		}
	}
	return result
}

func resolveClaudeDir() string {
	if claudeDir != "" {
		return claudeDir
	}
	return session.DefaultClaudeDir()
}

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func init() {
	sessionsCmd.Flags().BoolVar(&sessionsActive, "active", false, "Show only active sessions (modified within last 5 minutes)")
	sessionsCmd.Flags().StringVar(&sessionsProject, "project", "", "Filter by project name (substring match)")
	sessionsCmd.Flags().BoolVar(&sessionsCWD, "cwd", false, "Show only sessions for the current working directory")
	sessionsCmd.Flags().BoolVar(&sessionsAll, "all", false, "Show all sessions (no limit)")
	sessionsCmd.Flags().IntVar(&sessionsLimit, "limit", 20, "Maximum sessions to display")
	rootCmd.AddCommand(sessionsCmd)
}
