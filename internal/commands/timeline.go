package commands

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var timelineCWD bool

var timelineCmd = &cobra.Command{
	Use:   "timeline [session-id-or-path]",
	Short: "Show a chronological reasoning timeline for a session",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTimeline,
}

type timelineMark struct {
	Type       string `json:"type"`
	Label      string `json:"label,omitempty"`
	UUID       string `json:"uuid"`
	EntryIndex int    `json:"entry_index"`
}

type timelineEpoch struct {
	Index        int            `json:"index"`
	TurnCount    int            `json:"turn_count"`
	PeakTokens   int            `json:"peak_tokens"`
	Cost         float64        `json:"cost"`
	Topic        string         `json:"topic"`
	ToolCounts   map[string]int `json:"tool_counts,omitempty"`
	Decisions    []string       `json:"decisions,omitempty"`
	Marks        []timelineMark `json:"marks,omitempty"`
	DriftRatio   float64        `json:"drift_ratio,omitempty"`
	OutScope     int            `json:"out_scope,omitempty"`
	OutScopeRepo []string       `json:"out_scope_repos,omitempty"`
	GhostFiles   []string       `json:"ghost_files,omitempty"`
	IsActive     bool           `json:"is_active,omitempty"`
}

type timelineOutput struct {
	SessionID string          `json:"session_id"`
	Epochs    []timelineEpoch `json:"epochs"`
}

func runTimeline(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, timelineCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	stats := analyzer.Analyze(entries)
	if len(stats.EpochCosts) == 0 {
		return fmt.Errorf("no epoch data available")
	}

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

	fmt.Printf("Session timeline: %s\n", sessionID)
	for _, ep := range epochs {
		activeMarker := ""
		if ep.IsActive {
			activeMarker = " (active)"
		}
		fmt.Printf("  Epoch %-2d — %-26s %4d turns  %8s%s\n",
			ep.Index,
			truncateTimelineTopic(ep.Topic, 26),
			ep.TurnCount,
			analyzer.FormatCost(ep.Cost),
			activeMarker)

		if len(ep.ToolCounts) > 0 {
			fmt.Printf("    Tools: %s\n", formatTimelineCounts(ep.ToolCounts))
		}
		if ep.OutScope > 0 {
			repoInfo := "out-of-scope activity"
			if len(ep.OutScopeRepo) > 0 {
				repoInfo = strings.Join(ep.OutScopeRepo, ", ")
			}
			fmt.Printf("    Drift: %.1f%% (%d entries) [%s]\n",
				ep.DriftRatio*100, ep.OutScope, repoInfo)
		}
		for _, g := range ep.GhostFiles {
			fmt.Printf("    Ghost: %s\n", g)
		}
		for _, d := range ep.Decisions {
			fmt.Printf("    Decision: %s\n", d)
		}
		for _, m := range ep.Marks {
			label := m.Label
			if label == "" {
				label = m.UUID
			}
			typeName := m.Type
			if len(typeName) > 0 {
				typeName = strings.ToUpper(typeName[:1]) + typeName[1:]
			}
			fmt.Printf("    %s: %s\n", typeName, label)
		}
	}
	return nil
}

func buildTimelineEpochs(entries []jsonl.Entry, stats *analyzer.ContextStats, drift *analyzer.ScopeDrift, markers *editor.MarkerFile) []timelineEpoch {
	epochs := make([]timelineEpoch, 0, len(stats.EpochCosts))
	driftByEpoch := map[int]analyzer.EpochScope{}
	if drift != nil {
		for _, d := range drift.EpochScopes {
			driftByEpoch[d.EpochIndex] = d
		}
	}

	ghostByEpoch := map[int][]string{}
	if stats.GhostReport != nil {
		for _, g := range stats.GhostReport.Files {
			ghostByEpoch[g.EpochModified] = append(ghostByEpoch[g.EpochModified], g.Path)
		}
	}

	marksByEpoch := collectTimelineMarks(entries, stats.Compactions, markers)

	for i, ec := range stats.EpochCosts {
		ep := timelineEpoch{
			Index:      ec.EpochIndex,
			TurnCount:  ec.TurnCount,
			PeakTokens: ec.PeakTokens,
			Cost:       ec.Cost.TotalCost,
			IsActive:   i == len(stats.EpochCosts)-1,
		}

		toolCounts := map[string]int{}
		files := []string{}
		if stats.Archaeology != nil && i < len(stats.Archaeology.Events) {
			arch := stats.Archaeology.Events[i]
			for k, v := range arch.Before.ToolCallCounts {
				toolCounts[k] = v
			}
			files = append(files, arch.Before.FilesReferenced...)
			ep.Decisions = append(ep.Decisions, arch.Before.DecisionHints...)
		} else {
			start, end := epochRange(i, stats.Compactions, len(entries))
			files, toolCounts = summarizeEpochActivity(entries, start, end)
		}
		ep.ToolCounts = toolCounts
		ep.Topic = inferTimelineTopic(files, toolCounts, i)

		if ds, ok := driftByEpoch[i]; ok {
			ep.DriftRatio = ds.DriftRatio
			ep.OutScope = ds.OutScope
			ep.OutScopeRepo = flattenTimelineRepos(ds.OutScopeByRepo)
		}
		if ghosts, ok := ghostByEpoch[i]; ok {
			sort.Strings(ghosts)
			ep.GhostFiles = ghosts
		}
		if marks, ok := marksByEpoch[i]; ok {
			sort.Slice(marks, func(i, j int) bool {
				return marks[i].EntryIndex < marks[j].EntryIndex
			})
			ep.Marks = marks
		}

		epochs = append(epochs, ep)
	}

	return epochs
}

func collectTimelineMarks(entries []jsonl.Entry, compactions []analyzer.CompactionEvent, markers *editor.MarkerFile) map[int][]timelineMark {
	if markers == nil {
		return nil
	}

	indexByUUID := make(map[string]int, len(entries))
	for i, e := range entries {
		if e.UUID != "" {
			indexByUUID[e.UUID] = i
		}
	}

	out := make(map[int][]timelineMark)
	add := func(uuid, typ, label string) {
		idx, ok := indexByUUID[uuid]
		if !ok {
			return
		}
		epoch := epochForEntry(idx, compactions)
		out[epoch] = append(out[epoch], timelineMark{
			Type:       typ,
			Label:      label,
			UUID:       uuid,
			EntryIndex: idx,
		})
	}

	for uuid, m := range markers.Markers {
		if m == editor.MarkerKeep {
			add(uuid, "keep", "")
		}
	}
	for uuid, b := range markers.Bookmarks {
		add(uuid, string(b.Type), b.Label)
	}
	for _, cp := range markers.CommitPoints {
		add(cp.UUID, "commit", cp.Goal)
	}

	return out
}

func epochRange(epoch int, compactions []analyzer.CompactionEvent, total int) (int, int) {
	start := 0
	if epoch > 0 && epoch-1 < len(compactions) {
		start = compactions[epoch-1].LineIndex
	}
	end := total
	if epoch < len(compactions) {
		end = compactions[epoch].LineIndex
	}
	if start < 0 {
		start = 0
	}
	if end > total {
		end = total
	}
	if start > end {
		start = end
	}
	return start, end
}

func summarizeEpochActivity(entries []jsonl.Entry, start, end int) ([]string, map[string]int) {
	fileSet := make(map[string]bool)
	toolCounts := make(map[string]int)

	for i := start; i < end; i++ {
		e := entries[i]
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
			toolCounts[b.Name]++
			if path := analyzer.ExtractToolInputPath(b.Input); path != "" {
				fileSet[path] = true
			}
		}
	}

	files := make([]string, 0, len(fileSet))
	for f := range fileSet {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, toolCounts
}

func inferTimelineTopic(files []string, tools map[string]int, epoch int) string {
	if len(files) > 0 {
		dirCounts := make(map[string]int)
		for _, f := range files {
			dir := filepath.Dir(f)
			if dir == "." || dir == "" {
				dir = f
			}
			dirCounts[dir]++
		}
		bestDir := ""
		bestCount := 0
		for dir, count := range dirCounts {
			if count > bestCount || (count == bestCount && dir < bestDir) {
				bestDir = dir
				bestCount = count
			}
		}
		if bestDir != "" {
			return bestDir
		}
	}

	if len(tools) > 0 {
		type toolRank struct {
			name  string
			count int
		}
		var ranked []toolRank
		for name, count := range tools {
			ranked = append(ranked, toolRank{name: name, count: count})
		}
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].count != ranked[j].count {
				return ranked[i].count > ranked[j].count
			}
			return ranked[i].name < ranked[j].name
		})
		switch ranked[0].name {
		case "Bash":
			return "debugging/testing"
		case "Read":
			return "code exploration"
		case "Edit", "Write", "MultiEdit":
			return "implementation"
		default:
			return "tool focus: " + ranked[0].name
		}
	}

	return fmt.Sprintf("epoch %d", epoch)
}

func flattenTimelineRepos(repos map[string]int) []string {
	type item struct {
		repo  string
		count int
	}
	var list []item
	for repo, count := range repos {
		list = append(list, item{repo: filepath.Base(repo), count: count})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].repo < list[j].repo
	})
	out := make([]string, 0, len(list))
	for _, it := range list {
		out = append(out, fmt.Sprintf("%s(%d)", it.repo, it.count))
	}
	return out
}

func formatTimelineCounts(counts map[string]int) string {
	type item struct {
		name  string
		count int
	}
	var list []item
	for name, count := range counts {
		list = append(list, item{name: name, count: count})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].name < list[j].name
	})
	parts := make([]string, 0, len(list))
	for _, it := range list {
		parts = append(parts, fmt.Sprintf("%s(%d)", it.name, it.count))
	}
	return strings.Join(parts, ", ")
}

func truncateTimelineTopic(topic string, limit int) string {
	if len([]rune(topic)) <= limit {
		return topic
	}
	r := []rune(topic)
	return string(r[:limit-3]) + "..."
}

func init() {
	timelineCmd.Flags().BoolVar(&timelineCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(timelineCmd)
}
