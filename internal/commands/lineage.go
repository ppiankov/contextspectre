package commands

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	lineageProject  string
	lineageCWD      bool
	lineageAll      bool
	lineageDecision string
	lineageDays     int
)

var lineageCmd = &cobra.Command{
	Use:   "lineage <file-or-decision>",
	Short: "Trace history of a file or decision across sessions",
	Long: `Trace which sessions touched a file, what decisions were made,
what epoch each touch occurred in, and the cost of each touch.

File mode (default):
  contextspectre lineage internal/analyzer/analyzer.go --cwd
  contextspectre lineage analyzer.go --project myapp

Decision mode:
  contextspectre lineage --decision "add authentication" --all

Use --format json for machine-readable output.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLineage,
}

func runLineage(cmd *cobra.Command, args []string) error {
	if lineageDecision == "" && len(args) == 0 {
		return fmt.Errorf("specify a file path or use --decision <label>")
	}
	if lineageDecision != "" && len(args) > 0 {
		return fmt.Errorf("cannot combine file argument with --decision flag")
	}

	isDecisionMode := lineageDecision != ""
	query := lineageDecision
	if !isDecisionMode {
		query = args[0]
	}

	if !lineageCWD && lineageProject == "" && !lineageAll {
		return fmt.Errorf("specify --cwd, --project <name>, or --all")
	}

	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if lineageCWD {
		sessions = filterDistillSessions(sessions, "", true)
	} else if lineageProject != "" {
		sessions = resolveProjectSessions(sessions, lineageProject, dir)
	}

	if lineageDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -lineageDays)
		var recent []session.Info
		for _, s := range sessions {
			if s.Modified.After(cutoff) || s.Modified.Equal(cutoff) {
				recent = append(recent, s)
			}
		}
		sessions = recent
	}

	if len(sessions) == 0 {
		if isJSON() {
			return printJSON(emptyLineageOutput(query, isDecisionMode))
		}
		fmt.Println("No sessions found.")
		return nil
	}

	// Sort chronologically (oldest first).
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.Before(sessions[j].Modified)
	})

	if isDecisionMode {
		return runDecisionLineage(query, sessions)
	}
	return runFileLineage(query, sessions)
}

func runFileLineage(query string, sessions []session.Info) error {
	var allTouches []LineageTouchJSON
	var allDecisions []LineageDecisionJSON
	var totalCost float64
	matchedSessions := 0

	for _, si := range sessions {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			continue
		}
		stats := analyzer.Analyze(entries)
		markers, _ := editor.LoadMarkers(si.FullPath)

		lineage := analyzer.ExtractFileLineage(entries, stats.Compactions, query, stats.Model)
		if len(lineage.Touches) == 0 {
			continue
		}
		matchedSessions++

		for _, t := range lineage.Touches {
			allTouches = append(allTouches, LineageTouchJSON{
				SessionID:  si.SessionID,
				Slug:       si.DisplayName(),
				Project:    si.ProjectName,
				EntryIndex: t.EntryIndex,
				Timestamp:  t.Timestamp.Format(time.RFC3339),
				Action:     string(t.Action),
				ToolName:   t.ToolName,
				Epoch:      t.Epoch,
				TurnCost:   t.TurnCost,
			})
		}
		totalCost += lineage.TotalCost

		// Scan commit points for file references.
		if markers != nil {
			indexByUUID := buildLineageUUIDIndex(entries)
			for _, cp := range markers.CommitPoints {
				if lineageCommitPointReferencesFile(cp, query) {
					idx := indexByUUID[cp.UUID]
					epoch := epochForEntry(idx, stats.Compactions)
					allDecisions = append(allDecisions, LineageDecisionJSON{
						SessionID:  si.SessionID,
						Slug:       si.DisplayName(),
						Project:    si.ProjectName,
						UUID:       cp.UUID,
						Goal:       cp.Goal,
						Decisions:  cp.Decisions,
						Epoch:      epoch,
						EntryIndex: idx,
						Timestamp:  cp.Timestamp.Format(time.RFC3339),
					})
				}
			}
		}

		// Scan archaeology DecisionHints for file references.
		if stats.Archaeology != nil {
			for epochIdx, ev := range stats.Archaeology.Events {
				if len(ev.Before.DecisionHints) == 0 {
					continue
				}
				for _, f := range ev.Before.FilesReferenced {
					if analyzer.MatchesPathSuffix(f, query) {
						allDecisions = append(allDecisions, LineageDecisionJSON{
							SessionID: si.SessionID,
							Slug:      si.DisplayName(),
							Project:   si.ProjectName,
							UUID:      fmt.Sprintf("arch-epoch-%d", epochIdx),
							Goal:      strings.Join(ev.Before.DecisionHints, "; "),
							Epoch:     epochIdx,
						})
						break
					}
				}
			}
		}
	}

	if isJSON() {
		return printJSON(LineageOutputJSON{
			Query:           query,
			Mode:            "file",
			SessionsScanned: len(sessions),
			SessionsMatched: matchedSessions,
			TotalTouches:    len(allTouches),
			TotalCost:       totalCost,
			Touches:         ensureLineageTouches(allTouches),
			Decisions:       ensureLineageDecisions(allDecisions),
		})
	}

	return renderFileLineageText(query, len(sessions), allTouches, allDecisions, totalCost, matchedSessions)
}

func runDecisionLineage(query string, sessions []session.Info) error {
	var allDecisions []LineageDecisionJSON
	matchedSessions := 0

	for _, si := range sessions {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			continue
		}
		stats := analyzer.Analyze(entries)
		markers, _ := editor.LoadMarkers(si.FullPath)
		found := false

		if markers != nil {
			indexByUUID := buildLineageUUIDIndex(entries)
			for _, cp := range markers.CommitPoints {
				if analyzer.MatchesDecisionLabel(cp.Goal, cp.Decisions, query) {
					idx := indexByUUID[cp.UUID]
					epoch := epochForEntry(idx, stats.Compactions)
					allDecisions = append(allDecisions, LineageDecisionJSON{
						SessionID:  si.SessionID,
						Slug:       si.DisplayName(),
						Project:    si.ProjectName,
						UUID:       cp.UUID,
						Goal:       cp.Goal,
						Decisions:  cp.Decisions,
						Epoch:      epoch,
						EntryIndex: idx,
						Timestamp:  cp.Timestamp.Format(time.RFC3339),
					})
					found = true
				}
			}
		}

		if stats.Archaeology != nil {
			for epochIdx, ev := range stats.Archaeology.Events {
				if analyzer.MatchesDecisionHint(ev.Before.DecisionHints, query) {
					allDecisions = append(allDecisions, LineageDecisionJSON{
						SessionID: si.SessionID,
						Slug:      si.DisplayName(),
						Project:   si.ProjectName,
						UUID:      fmt.Sprintf("arch-epoch-%d", epochIdx),
						Goal:      strings.Join(ev.Before.DecisionHints, "; "),
						Epoch:     epochIdx,
					})
					found = true
				}
			}
		}

		if found {
			matchedSessions++
		}
	}

	if isJSON() {
		return printJSON(LineageOutputJSON{
			Query:           query,
			Mode:            "decision",
			SessionsScanned: len(sessions),
			SessionsMatched: matchedSessions,
			Touches:         []LineageTouchJSON{},
			Decisions:       ensureLineageDecisions(allDecisions),
		})
	}

	return renderDecisionLineageText(query, len(sessions), allDecisions, matchedSessions)
}

// --- helpers ---

func buildLineageUUIDIndex(entries []jsonl.Entry) map[string]int {
	m := make(map[string]int, len(entries))
	for i, e := range entries {
		if e.UUID != "" {
			m[e.UUID] = i
		}
	}
	return m
}

func lineageCommitPointReferencesFile(cp editor.CommitPoint, query string) bool {
	for _, f := range cp.Files {
		if analyzer.MatchesPathSuffix(f, query) {
			return true
		}
	}
	return false
}

func ensureLineageTouches(t []LineageTouchJSON) []LineageTouchJSON {
	if t == nil {
		return []LineageTouchJSON{}
	}
	return t
}

func ensureLineageDecisions(d []LineageDecisionJSON) []LineageDecisionJSON {
	if d == nil {
		return []LineageDecisionJSON{}
	}
	return d
}

func emptyLineageOutput(query string, isDecision bool) LineageOutputJSON {
	mode := "file"
	if isDecision {
		mode = "decision"
	}
	return LineageOutputJSON{
		Query:     query,
		Mode:      mode,
		Touches:   []LineageTouchJSON{},
		Decisions: []LineageDecisionJSON{},
	}
}

// --- text rendering ---

func renderFileLineageText(query string, scanned int, touches []LineageTouchJSON,
	decisions []LineageDecisionJSON, totalCost float64, matched int) error {

	if len(touches) == 0 {
		fmt.Printf("No touches found for %q across %d sessions.\n", query, scanned)
		return nil
	}

	fmt.Printf("Lineage: %s\n", query)
	fmt.Printf("  %d touches across %d sessions (searched %d), total cost: %s\n\n",
		len(touches), matched, scanned, analyzer.FormatCost(totalCost))

	// Group by session (preserving chronological order).
	type sessionGroup struct {
		slug    string
		project string
		touches []LineageTouchJSON
	}
	var groups []sessionGroup
	seen := map[string]int{}

	for _, t := range touches {
		if idx, ok := seen[t.SessionID]; ok {
			groups[idx].touches = append(groups[idx].touches, t)
		} else {
			seen[t.SessionID] = len(groups)
			groups = append(groups, sessionGroup{
				slug:    t.Slug,
				project: t.Project,
				touches: []LineageTouchJSON{t},
			})
		}
	}

	for _, g := range groups {
		fmt.Printf("Session: %s (%s)\n", g.slug, g.project)
		for _, t := range g.touches {
			ts := ""
			if t.Timestamp != "" {
				if parsed, err := time.Parse(time.RFC3339, t.Timestamp); err == nil {
					ts = parsed.Format("2006-01-02 15:04")
				}
			}
			fmt.Printf("  #%-5d  %-16s  epoch %-2d  %-6s  %-8s  %s\n",
				t.EntryIndex, ts, t.Epoch, t.Action, t.ToolName,
				analyzer.FormatCost(t.TurnCost))
		}
		fmt.Println()
	}

	if len(decisions) > 0 {
		fmt.Println("Related decisions:")
		for _, d := range decisions {
			uuid := d.UUID
			if len(uuid) > 8 {
				uuid = uuid[:8]
			}
			fmt.Printf("  [%-12s] %s — %s (epoch %d)\n", d.Slug, uuid, singleLine(d.Goal, 80), d.Epoch)
		}
		fmt.Println()
	}

	return nil
}

func renderDecisionLineageText(query string, scanned int,
	decisions []LineageDecisionJSON, matched int) error {

	if len(decisions) == 0 {
		fmt.Printf("No decisions matching %q across %d sessions.\n", query, scanned)
		return nil
	}

	fmt.Printf("Decision lineage: %q\n", query)
	fmt.Printf("  %d matches across %d sessions (searched %d)\n\n",
		len(decisions), matched, scanned)

	for _, d := range decisions {
		fmt.Printf("  [%-12s]  epoch %-2d  %s\n", d.Slug, d.Epoch, singleLine(d.Goal, 80))
		for _, dec := range d.Decisions {
			fmt.Printf("    - %s\n", singleLine(dec, 100))
		}
	}
	fmt.Println()

	return nil
}

func init() {
	lineageCmd.Flags().StringVar(&lineageProject, "project", "", "Filter by project name or alias")
	lineageCmd.Flags().BoolVar(&lineageCWD, "cwd", false, "Search sessions for current working directory")
	lineageCmd.Flags().BoolVar(&lineageAll, "all", false, "Search all sessions globally")
	lineageCmd.Flags().StringVar(&lineageDecision, "decision", "", "Find sessions where a decision label appears")
	lineageCmd.Flags().IntVar(&lineageDays, "days", 0, "Only scan sessions modified in last N days")
	rootCmd.AddCommand(lineageCmd)
}
