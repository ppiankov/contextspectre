package commands

import (
	"fmt"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	searchProject string
	searchCWD     bool
	searchAll     bool
	caseSensitive bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search session content across projects",
	Args:  cobra.ExactArgs(1),
	RunE:  runSearch,
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	dir := resolveClaudeDir()

	if !searchCWD && searchProject == "" && !searchAll {
		return fmt.Errorf("specify --cwd, --project <name>, or --all")
	}

	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	// Filter sessions
	if searchCWD {
		sessions = filterDistillSessions(sessions, "", true)
	} else if searchProject != "" {
		sessions = resolveProjectSessions(sessions, searchProject, dir)
	}
	// --all: no filtering

	if len(sessions) == 0 {
		if isJSON() {
			return printJSON(SearchOutputJSON{Query: query, Hits: []SearchHitJSON{}})
		}
		fmt.Println("No sessions found.")
		return nil
	}

	ignoreCase := !caseSensitive

	type sessionResult struct {
		info session.Info
		hits []analyzer.SearchHit
	}

	var results []sessionResult
	totalHits := 0

	for _, si := range sessions {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			continue
		}

		hits := analyzer.Search(entries, query, ignoreCase)
		if len(hits) > 0 {
			results = append(results, sessionResult{info: si, hits: hits})
			totalHits += len(hits)
		}
	}

	if isJSON() {
		out := SearchOutputJSON{
			Query:    query,
			Total:    totalHits,
			Sessions: len(sessions),
			Matches:  len(results),
		}
		for _, r := range results {
			for _, h := range r.hits {
				out.Hits = append(out.Hits, SearchHitJSON{
					SessionID:  r.info.SessionID,
					Slug:       r.info.Slug,
					Project:    r.info.ProjectName,
					EntryIndex: h.EntryIndex,
					Timestamp:  h.Timestamp.Format(time.RFC3339),
					Role:       h.Role,
					Snippet:    h.Snippet,
				})
			}
		}
		if out.Hits == nil {
			out.Hits = []SearchHitJSON{}
		}
		return printJSON(out)
	}

	if totalHits == 0 {
		fmt.Printf("No matches for %q across %d sessions.\n", query, len(sessions))
		return nil
	}

	fmt.Printf("Found %d matches across %d sessions (searched %d)\n\n", totalHits, len(results), len(sessions))

	for _, r := range results {
		slug := r.info.DisplayName()
		fmt.Printf("Session: %s (%s) — %s\n", slug, r.info.ShortID(), r.info.ProjectName)
		for _, h := range r.hits {
			ts := ""
			if !h.Timestamp.IsZero() {
				ts = h.Timestamp.Format("2006-01-02 15:04")
			}
			snippet := singleLine(h.Snippet, 120)
			fmt.Printf("  #%-5d %s  [%-10s] %q\n", h.EntryIndex, ts, h.Role, snippet)
		}
		fmt.Println()
	}

	return nil
}

// singleLine collapses newlines and truncates to maxLen.
func singleLine(s string, maxLen int) string {
	// Replace newlines with spaces
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' || s[i] == '\t' {
			if len(result) > 0 && result[len(result)-1] != ' ' {
				result = append(result, ' ')
			}
		} else {
			result = append(result, s[i])
		}
	}
	out := string(result)
	if len(out) > maxLen {
		return out[:maxLen-3] + "..."
	}
	return out
}

func init() {
	searchCmd.Flags().StringVar(&searchProject, "project", "", "Filter by project name or alias")
	searchCmd.Flags().BoolVar(&searchCWD, "cwd", false, "Search sessions for current working directory")
	searchCmd.Flags().BoolVar(&searchAll, "all", false, "Search all sessions globally")
	searchCmd.Flags().BoolVar(&caseSensitive, "case-sensitive", false, "Case-sensitive search (default: case-insensitive)")
	rootCmd.AddCommand(searchCmd)
}
