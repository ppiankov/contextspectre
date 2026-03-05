package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	exportTasksProject string
	exportTasksCWD     bool
	exportTasksOutPath string
	exportDecisionsCWD bool
	exportTimelineCWD  bool
)

var exportTasksCmd = &cobra.Command{
	Use:   "tasks [session-id-or-path]",
	Short: "Extract actionable work items from session history",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runExportTasks,
}

var exportDecisionsCmd = &cobra.Command{
	Use:   "decisions [session-id-or-path]",
	Short: "Export decisions from commit points and archaeology",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runExportDecisions,
}

var exportTimelineCmd = &cobra.Command{
	Use:   "timeline [session-id-or-path]",
	Short: "Export session timeline as portable artifact",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runExportTimeline,
}

type exportSource struct {
	SessionID  string `json:"session_id"`
	Session    string `json:"session"`
	Epoch      int    `json:"epoch"`
	EntryIndex int    `json:"entry_index"`
}

type exportItem struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Sources []exportSource `json:"sources"`
}

type exportTasksReport struct {
	Project   string       `json:"project,omitempty"`
	Sessions  int          `json:"sessions"`
	Total     int          `json:"total"`
	Todos     []exportItem `json:"todos"`
	Decisions []exportItem `json:"decisions"`
	Questions []exportItem `json:"questions"`
}

func runExportTasks(cmd *cobra.Command, args []string) error {
	paths, label, err := resolveExportTaskPaths(args)
	if err != nil {
		return err
	}

	items := collectExportItems(paths)
	grouped := groupExportItemsByType(items)
	out := exportTasksReport{
		Project:   label,
		Sessions:  len(paths),
		Total:     len(items),
		Todos:     grouped["todo"],
		Decisions: grouped["decision"],
		Questions: grouped["question"],
	}

	if isJSON() {
		return printJSON(out)
	}

	markdown := renderExportTasksMarkdown(label, len(paths), out.Todos, out.Decisions, out.Questions)
	if exportTasksOutPath != "" {
		return os.WriteFile(exportTasksOutPath, []byte(markdown), 0o644)
	}
	fmt.Print(markdown)
	return nil
}

func runExportDecisions(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, exportDecisionsCWD)
	if err != nil {
		return err
	}
	items := collectExportItems([]string{path})
	var decisions []exportItem
	for _, it := range items {
		if it.Type == "decision" {
			decisions = append(decisions, it)
		}
	}
	sortExportItems(decisions)

	if isJSON() {
		return printJSON(decisions)
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	var b strings.Builder
	fmt.Fprintf(&b, "## Extracted decisions: %s\n\n", sessionID)
	if len(decisions) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, d := range decisions {
			src := d.Sources[0]
			fmt.Fprintf(&b, "- %s (session %s, epoch %d)\n", d.Text, src.SessionID, src.Epoch)
		}
	}
	fmt.Print(b.String())
	return nil
}

func runExportTimeline(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, exportTimelineCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	stats := analyzer.Analyze(entries)
	drift := analyzer.AnalyzeScopeDrift(entries, stats.Compactions, "")
	markers, _ := editor.LoadMarkers(path)
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	epochs := buildTimelineEpochs(entries, stats, drift, markers)

	if isJSON() {
		return printJSON(timelineOutput{
			SessionID: sessionID,
			Epochs:    epochs,
		})
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Exported timeline: %s\n\n", sessionID)
	for _, ep := range epochs {
		fmt.Fprintf(&b, "- Epoch %d — %s, %d turns, %s\n",
			ep.Index, ep.Topic, ep.TurnCount, analyzer.FormatCost(ep.Cost))
	}
	fmt.Print(b.String())
	return nil
}

func resolveExportTaskPaths(args []string) ([]string, string, error) {
	if exportTasksProject != "" {
		d := &session.Discoverer{ClaudeDir: resolveClaudeDir()}
		sessions, err := d.ListAllSessions()
		if err != nil {
			return nil, "", err
		}
		filtered := resolveProjectSessions(sessions, exportTasksProject, resolveClaudeDir())
		if len(filtered) == 0 {
			return nil, "", fmt.Errorf("no sessions found for project: %s", exportTasksProject)
		}
		paths := make([]string, 0, len(filtered))
		for _, s := range filtered {
			paths = append(paths, s.FullPath)
		}
		return paths, exportTasksProject, nil
	}

	path, err := resolveSessionArg(args, exportTasksCWD)
	if err != nil {
		return nil, "", err
	}
	return []string{path}, strings.TrimSuffix(filepath.Base(path), ".jsonl"), nil
}

func collectExportItems(paths []string) []exportItem {
	index := make(map[string]*exportItem)
	for _, path := range paths {
		entries, err := jsonl.Parse(path)
		if err != nil {
			continue
		}
		stats := analyzer.Analyze(entries)
		sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		sessionLabel := sessionID
		for i, e := range entries {
			if e.Type != jsonl.TypeUser || e.Message == nil {
				continue
			}
			blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
			if err != nil {
				continue
			}
			for _, b := range blocks {
				if b.Type != "text" {
					continue
				}
				lines := splitExportTextUnits(b.Text)
				for _, line := range lines {
					class := classifyExportText(line)
					if class == "" {
						continue
					}
					addExportItem(index, class, line, exportSource{
						SessionID:  sessionID,
						Session:    sessionLabel,
						Epoch:      epochForEntry(i, stats.Compactions),
						EntryIndex: i,
					})
				}
			}
		}
		if stats.Archaeology != nil {
			for _, arch := range stats.Archaeology.Events {
				for _, d := range arch.Before.DecisionHints {
					addExportItem(index, "decision", d, exportSource{
						SessionID:  sessionID,
						Session:    sessionLabel,
						Epoch:      arch.CompactionIndex,
						EntryIndex: arch.LineIndex,
					})
				}
			}
		}
		markers, err := editor.LoadMarkers(path)
		if err == nil {
			entryByUUID := map[string]int{}
			for idx, e := range entries {
				if e.UUID != "" {
					entryByUUID[e.UUID] = idx
				}
			}
			for _, cp := range markers.CommitPoints {
				if strings.TrimSpace(cp.Goal) != "" {
					idx := entryByUUID[cp.UUID]
					addExportItem(index, "decision", cp.Goal, exportSource{
						SessionID:  sessionID,
						Session:    sessionLabel,
						Epoch:      epochForEntry(idx, stats.Compactions),
						EntryIndex: idx,
					})
				}
				for _, d := range cp.Decisions {
					idx := entryByUUID[cp.UUID]
					addExportItem(index, "decision", d, exportSource{
						SessionID:  sessionID,
						Session:    sessionLabel,
						Epoch:      epochForEntry(idx, stats.Compactions),
						EntryIndex: idx,
					})
				}
			}
		}
	}

	items := make([]exportItem, 0, len(index))
	for _, v := range index {
		sort.Slice(v.Sources, func(i, j int) bool {
			if v.Sources[i].SessionID != v.Sources[j].SessionID {
				return v.Sources[i].SessionID < v.Sources[j].SessionID
			}
			return v.Sources[i].EntryIndex < v.Sources[j].EntryIndex
		})
		items = append(items, *v)
	}
	sortExportItems(items)
	return items
}

func addExportItem(index map[string]*exportItem, typ, text string, src exportSource) {
	normalizedText := normalizeExportText(text)
	if normalizedText == "" {
		return
	}
	key := typ + "|" + normalizedText
	item, ok := index[key]
	if !ok {
		item = &exportItem{
			Type:    typ,
			Text:    strings.TrimSpace(text),
			Sources: []exportSource{},
		}
		index[key] = item
	}
	for _, s := range item.Sources {
		if s.SessionID == src.SessionID && s.EntryIndex == src.EntryIndex {
			return
		}
	}
	item.Sources = append(item.Sources, src)
}

func splitExportTextUnits(text string) []string {
	raw := strings.Split(text, "\n")
	var out []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

var (
	exportTodoRe = regexp.MustCompile(`(?i)\b(todo|fix later|follow up|need to|wo-[0-9]+|should we)\b`)
	exportTBDRe  = regexp.MustCompile(`(?i)\b(tbd|not sure|unclear|unknown)\b`)
)

func classifyExportText(line string) string {
	l := strings.TrimSpace(line)
	if l == "" {
		return ""
	}
	if exportTodoRe.MatchString(l) {
		return "todo"
	}
	if strings.Contains(l, "?") || exportTBDRe.MatchString(l) {
		return "question"
	}
	return ""
}

func normalizeExportText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.Join(strings.Fields(text), " ")
	return text
}

func sortExportItems(items []exportItem) {
	priority := map[string]int{"todo": 0, "decision": 1, "question": 2}
	sort.Slice(items, func(i, j int) bool {
		pi := priority[items[i].Type]
		pj := priority[items[j].Type]
		if pi != pj {
			return pi < pj
		}
		return items[i].Text < items[j].Text
	})
}

func groupExportItemsByType(items []exportItem) map[string][]exportItem {
	grouped := map[string][]exportItem{
		"todo":     {},
		"decision": {},
		"question": {},
	}
	for _, it := range items {
		grouped[it.Type] = append(grouped[it.Type], it)
	}
	return grouped
}

func renderExportTasksMarkdown(label string, sessions int, todos, decisions, questions []exportItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Extracted tasks: %s (%d sessions)\n\n", label, sessions)
	b.WriteString("### TODOs\n")
	writeExportItemList(&b, todos, true)
	b.WriteString("\n### Decisions\n")
	writeExportItemList(&b, decisions, false)
	b.WriteString("\n### Open questions\n")
	writeExportItemList(&b, questions, false)
	return b.String()
}

func writeExportItemList(b *strings.Builder, items []exportItem, checkbox bool) {
	if len(items) == 0 {
		b.WriteString("- (none)\n")
		return
	}
	for _, it := range items {
		src := it.Sources[0]
		prefix := "- "
		if checkbox {
			prefix = "- [ ] "
		}
		extra := ""
		if len(it.Sources) > 1 {
			extra = fmt.Sprintf(", +%d more source(s)", len(it.Sources)-1)
		}
		fmt.Fprintf(b, "%s%s (session %s, epoch %d, entry %d%s)\n",
			prefix, it.Text, src.SessionID, src.Epoch, src.EntryIndex, extra)
	}
}

func init() {
	exportTasksCmd.Flags().StringVar(&exportTasksProject, "project", "", "Aggregate tasks across project sessions")
	exportTasksCmd.Flags().BoolVar(&exportTasksCWD, "cwd", false, "Use most recent session for current directory")
	exportTasksCmd.Flags().StringVar(&exportTasksOutPath, "output", "", "Output file path (default: stdout)")
	exportDecisionsCmd.Flags().BoolVar(&exportDecisionsCWD, "cwd", false, "Use most recent session for current directory")
	exportTimelineCmd.Flags().BoolVar(&exportTimelineCWD, "cwd", false, "Use most recent session for current directory")
	exportCmd.AddCommand(exportTasksCmd)
	exportCmd.AddCommand(exportDecisionsCmd)
	exportCmd.AddCommand(exportTimelineCmd)
}
