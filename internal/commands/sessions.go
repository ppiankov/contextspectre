package commands

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	sessionsActive  bool
	sessionsProject string
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List all conversation sessions",
	RunE:  runSessions,
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

	if len(sessions) == 0 {
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
				if s.ContextStats.ContextTokens > 0 {
					sp := s.ContextStats.SignalPercent
					sj.SignalPercent = &sp
				}
			}
			out.Sessions = append(out.Sessions, sj)
		}
		return printJSON(out)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tSLUG\tID\tBRANCH\tMSGS\tSIZE\tCONTEXT\tMODIFIED")
	fmt.Fprintln(w, "───────\t────\t──\t──────\t────\t────\t───────\t────────")

	for _, s := range sessions {
		active := ""
		if s.IsActive() {
			active = "[ACTIVE] "
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
			active,
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
	return w.Flush()
}

// filterSessions applies --active and --project flags.
func filterSessions(sessions []session.Info) []session.Info {
	if !sessionsActive && sessionsProject == "" {
		return sessions
	}
	var filtered []session.Info
	for _, s := range sessions {
		if sessionsActive && time.Since(s.Modified) > 5*time.Minute {
			continue
		}
		if sessionsProject != "" &&
			!strings.Contains(strings.ToLower(s.ProjectName), strings.ToLower(sessionsProject)) &&
			!strings.Contains(strings.ToLower(s.ProjectPath), strings.ToLower(sessionsProject)) {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
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
	rootCmd.AddCommand(sessionsCmd)
}
