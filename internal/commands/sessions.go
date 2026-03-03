package commands

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
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
	fmt.Fprintln(w, "PROJECT\tBRANCH\tMSGS\tSIZE\tCONTEXT\tMODIFIED")
	fmt.Fprintln(w, "───────\t──────\t────\t────\t───────\t────────")

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

		fmt.Fprintf(w, "%s%s\t%s\t%d\t%.1f MB\t%s\t%s\n",
			active,
			s.ProjectName,
			branch,
			s.MessageCount,
			s.FileSizeMB,
			contextStr,
			timeAgo(s.Modified),
		)
	}
	return w.Flush()
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
	rootCmd.AddCommand(sessionsCmd)
}
