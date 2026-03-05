package commands

import (
	"fmt"
	"os"
	"path/filepath"
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
	conflictsOutput string
	conflictsDays   int
)

var conflictsCmd = &cobra.Command{
	Use:   "conflicts <project>",
	Short: "Detect structural decision conflicts across project sessions",
	Args:  cobra.ExactArgs(1),
	RunE:  runConflicts,
}

type conflictSeverity string

const (
	conflictInfo    conflictSeverity = "info"
	conflictWarning conflictSeverity = "warning"
	conflictAlert   conflictSeverity = "alert"
)

type conflictType string

const (
	conflictScopeViolation conflictType = "scope_violation"
	conflictConstraintDrft conflictType = "constraint_drift"
	conflictReversal       conflictType = "reversal_pattern"
)

type conflictItem struct {
	Severity        conflictSeverity `json:"severity"`
	Type            conflictType     `json:"type"`
	File            string           `json:"file"`
	DecisionSession string           `json:"decision_session"`
	DecisionUUID    string           `json:"decision_uuid"`
	DecisionGoal    string           `json:"decision_goal"`
	Session         string           `json:"session"`
	SessionID       string           `json:"session_id"`
	Epoch           int              `json:"epoch"`
	EntryIndex      int              `json:"entry_index"`
	Evidence        string           `json:"evidence"`
}

type conflictsOutputJSON struct {
	Project         string         `json:"project"`
	SessionsScanned int            `json:"sessions_scanned"`
	GeneratedAt     time.Time      `json:"generated_at"`
	Conflicts       []conflictItem `json:"conflicts"`
}

type conflictDecision struct {
	SessionID     string
	SessionLabel  string
	UUID          string
	Goal          string
	Files         []string
	Scopes        []string
	HasConstraint bool
	Seq           int
}

type conflictSession struct {
	info      session.Info
	entries   []jsonl.Entry
	compacts  []analyzer.CompactionEvent
	markers   *editor.MarkerFile
	touched   map[string]int
	deleted   map[string]int
	refScopes map[string]bool
}

func runConflicts(cmd *cobra.Command, args []string) error {
	projectFlag := args[0]
	claudeDir := resolveClaudeDir()

	d := &session.Discoverer{ClaudeDir: claudeDir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	sessions = resolveProjectSessions(sessions, projectFlag, claudeDir)
	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found for project: %s", projectFlag)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.Before(sessions[j].Modified)
	})

	if conflictsDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -conflictsDays)
		var recent []session.Info
		for _, s := range sessions {
			if s.Modified.IsZero() || s.Modified.After(cutoff) || s.Modified.Equal(cutoff) {
				recent = append(recent, s)
			}
		}
		if len(recent) > 0 {
			sessions = recent
		}
	}

	parsed := parseConflictSessions(sessions)
	decisions := collectConflictDecisions(parsed)
	conflicts := detectConflicts(parsed, decisions)

	sortConflicts(conflicts)
	if isJSON() {
		return printJSON(conflictsOutputJSON{
			Project:         projectFlag,
			SessionsScanned: len(parsed),
			GeneratedAt:     time.Now(),
			Conflicts:       conflicts,
		})
	}

	if conflictsOutput == "" {
		conflictsOutput = "conflicts.md"
	}
	report := renderConflictsMarkdown(projectFlag, parsed, conflicts)
	if err := os.WriteFile(conflictsOutput, []byte(report), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	fmt.Printf("Conflict report written: %s (%d findings)\n", conflictsOutput, len(conflicts))
	return nil
}

func parseConflictSessions(sessions []session.Info) []conflictSession {
	result := make([]conflictSession, 0, len(sessions))
	for _, si := range sessions {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			continue
		}
		stats := analyzer.Analyze(entries)
		markers, err := editor.LoadMarkers(si.FullPath)
		if err != nil {
			continue
		}
		touched, deleted := collectConflictFileOps(entries)
		result = append(result, conflictSession{
			info:      si,
			entries:   entries,
			compacts:  stats.Compactions,
			markers:   markers,
			touched:   touched,
			deleted:   deleted,
			refScopes: collectConflictReferenceScopes(markers),
		})
	}
	return result
}

func collectConflictDecisions(sessions []conflictSession) []conflictDecision {
	var out []conflictDecision
	seq := 0
	for _, s := range sessions {
		if s.markers == nil {
			continue
		}
		for _, cp := range s.markers.CommitPoints {
			files := normalizeConflictPaths(cp.Files)
			if len(files) == 0 {
				continue
			}
			scopes := buildConflictScopes(files)
			out = append(out, conflictDecision{
				SessionID:     s.info.SessionID,
				SessionLabel:  s.info.DisplayName(),
				UUID:          cp.UUID,
				Goal:          cp.Goal,
				Files:         files,
				Scopes:        scopes,
				HasConstraint: len(cp.Constraints) > 0,
				Seq:           seq,
			})
		}
		seq++
	}
	return out
}

func detectConflicts(sessions []conflictSession, decisions []conflictDecision) []conflictItem {
	var findings []conflictItem
	seen := map[string]bool{}

	sessionPos := make(map[string]int, len(sessions))
	for i, s := range sessions {
		sessionPos[s.info.SessionID] = i
	}

	for _, d := range decisions {
		decisionPos := sessionPos[d.SessionID]
		for i := decisionPos + 1; i < len(sessions); i++ {
			s := sessions[i]
			refDecisionScope := conflictSessionReferencesScopes(s.refScopes, d.Scopes)

			for file, idx := range s.touched {
				if !conflictPathMatchesScopes(file, d.Scopes) {
					continue
				}
				epoch := epochForEntry(idx, s.compacts)
				if !refDecisionScope {
					item := conflictItem{
						Severity:        conflictWarning,
						Type:            conflictScopeViolation,
						File:            file,
						DecisionSession: d.SessionLabel,
						DecisionUUID:    d.UUID,
						DecisionGoal:    d.Goal,
						Session:         s.info.DisplayName(),
						SessionID:       s.info.SessionID,
						Epoch:           epoch,
						EntryIndex:      idx,
						Evidence:        "modification in constrained scope without decision context",
					}
					key := conflictKey(item)
					if !seen[key] {
						findings = append(findings, item)
						seen[key] = true
					}
				}

				if d.HasConstraint {
					item := conflictItem{
						Severity:        conflictInfo,
						Type:            conflictConstraintDrft,
						File:            file,
						DecisionSession: d.SessionLabel,
						DecisionUUID:    d.UUID,
						DecisionGoal:    d.Goal,
						Session:         s.info.DisplayName(),
						SessionID:       s.info.SessionID,
						Epoch:           epoch,
						EntryIndex:      idx,
						Evidence:        "work in constrained area from prior decision",
					}
					key := conflictKey(item)
					if !seen[key] {
						findings = append(findings, item)
						seen[key] = true
					}
				}
			}

			for file, idx := range s.deleted {
				if !conflictFileInList(file, d.Files) {
					continue
				}
				epoch := epochForEntry(idx, s.compacts)
				item := conflictItem{
					Severity:        conflictAlert,
					Type:            conflictReversal,
					File:            file,
					DecisionSession: d.SessionLabel,
					DecisionUUID:    d.UUID,
					DecisionGoal:    d.Goal,
					Session:         s.info.DisplayName(),
					SessionID:       s.info.SessionID,
					Epoch:           epoch,
					EntryIndex:      idx,
					Evidence:        "prior decision artifact deleted in later session",
				}
				key := conflictKey(item)
				if !seen[key] {
					findings = append(findings, item)
					seen[key] = true
				}
			}
		}
	}

	return findings
}

func collectConflictFileOps(entries []jsonl.Entry) (map[string]int, map[string]int) {
	touched := map[string]int{}
	deleted := map[string]int{}

	for i, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			tool := strings.ToLower(strings.TrimSpace(b.Name))
			if !isConflictMutationTool(tool) {
				continue
			}
			path := normalizeConflictPath(analyzer.ExtractToolInputPath(b.Input))
			if path == "" {
				continue
			}
			if prev, ok := touched[path]; !ok || i < prev {
				touched[path] = i
			}
			if tool == "delete" {
				if prev, ok := deleted[path]; !ok || i < prev {
					deleted[path] = i
				}
			}
		}
	}

	return touched, deleted
}

func isConflictMutationTool(tool string) bool {
	switch tool {
	case "edit", "multiedit", "write", "delete", "notebookedit":
		return true
	default:
		return false
	}
}

func collectConflictReferenceScopes(m *editor.MarkerFile) map[string]bool {
	scopes := make(map[string]bool)
	if m == nil {
		return scopes
	}
	for _, cp := range m.CommitPoints {
		for _, s := range buildConflictScopes(normalizeConflictPaths(cp.Files)) {
			scopes[s] = true
		}
	}
	return scopes
}

func buildConflictScopes(files []string) []string {
	scopeSet := map[string]bool{}
	for _, f := range files {
		scope := filepath.Dir(f)
		if scope == "." || scope == "/" || scope == "" {
			scope = f
		}
		scopeSet[scope] = true
	}
	var scopes []string
	for s := range scopeSet {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)
	return scopes
}

func normalizeConflictPaths(paths []string) []string {
	set := map[string]bool{}
	for _, p := range paths {
		if n := normalizeConflictPath(p); n != "" {
			set[n] = true
		}
	}
	var out []string
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func normalizeConflictPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func conflictPathMatchesScopes(path string, scopes []string) bool {
	for _, scope := range scopes {
		if path == scope {
			return true
		}
		if strings.HasPrefix(path, scope+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func conflictFileInList(path string, files []string) bool {
	for _, f := range files {
		if path == f {
			return true
		}
	}
	return false
}

func conflictSessionReferencesScopes(refScopes map[string]bool, decisionScopes []string) bool {
	if len(refScopes) == 0 || len(decisionScopes) == 0 {
		return false
	}
	for rs := range refScopes {
		for _, ds := range decisionScopes {
			if rs == ds ||
				strings.HasPrefix(rs, ds+string(filepath.Separator)) ||
				strings.HasPrefix(ds, rs+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

func conflictKey(item conflictItem) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%d",
		item.Type, item.DecisionUUID, item.SessionID, item.File, item.Evidence, item.EntryIndex)
}

func sortConflicts(items []conflictItem) {
	rank := map[conflictSeverity]int{
		conflictAlert:   0,
		conflictWarning: 1,
		conflictInfo:    2,
	}
	sort.Slice(items, func(i, j int) bool {
		ri := rank[items[i].Severity]
		rj := rank[items[j].Severity]
		if ri != rj {
			return ri < rj
		}
		if items[i].Session != items[j].Session {
			return items[i].Session < items[j].Session
		}
		if items[i].Epoch != items[j].Epoch {
			return items[i].Epoch < items[j].Epoch
		}
		return items[i].File < items[j].File
	})
}

func renderConflictsMarkdown(project string, sessions []conflictSession, conflicts []conflictItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Conflict report: %s\n\n", project)
	fmt.Fprintf(&b, "Generated: %s\n", time.Now().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "Sessions scanned: %d\n", len(sessions))
	fmt.Fprintf(&b, "Findings: %d\n\n", len(conflicts))

	if len(conflicts) == 0 {
		b.WriteString("No conflicts detected.\n")
		return b.String()
	}

	groups := []conflictSeverity{conflictAlert, conflictWarning, conflictInfo}
	for _, sev := range groups {
		var subset []conflictItem
		for _, c := range conflicts {
			if c.Severity == sev {
				subset = append(subset, c)
			}
		}
		if len(subset) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", strings.ToUpper(string(sev)))
		for _, c := range subset {
			fmt.Fprintf(&b, "- **%s** `%s`\n", c.Type, c.File)
			fmt.Fprintf(&b, "  - Decision: `%s` (%s)\n", c.DecisionUUID, c.DecisionSession)
			if strings.TrimSpace(c.DecisionGoal) != "" {
				fmt.Fprintf(&b, "  - Goal: %s\n", c.DecisionGoal)
			}
			fmt.Fprintf(&b, "  - Evidence: session `%s`, epoch %d, entry %d — %s\n",
				c.Session, c.Epoch, c.EntryIndex, c.Evidence)
		}
		b.WriteByte('\n')
	}

	return b.String()
}

func init() {
	conflictsCmd.Flags().StringVar(&conflictsOutput, "output", "conflicts.md", "Output report path")
	conflictsCmd.Flags().IntVar(&conflictsDays, "days", 0, "Only scan sessions modified in last N days (0 = all)")
	rootCmd.AddCommand(conflictsCmd)
}
