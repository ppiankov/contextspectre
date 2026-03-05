package commands

import (
	"fmt"
	"os"
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
	memoryProject string
	memoryCWD     bool
	memoryOutput  string
	memoryApply   bool
	memoryDryRun  bool
	memoryDays    int
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Project memory synthesis commands",
}

var memoryBuildCmd = &cobra.Command{
	Use:   "build [project]",
	Short: "Synthesize project memory from session history",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runMemoryBuild,
}

type memoryParsedSession struct {
	info    session.Info
	entries []jsonl.Entry
}

type memoryFileStat struct {
	Path     string `json:"path"`
	Sessions int    `json:"sessions"`
	Touches  int    `json:"touches"`
}

type memoryRecentItem struct {
	SessionID string   `json:"session_id"`
	Label     string   `json:"label"`
	Modified  string   `json:"modified"`
	Files     []string `json:"files,omitempty"`
	Actions   []string `json:"actions,omitempty"`
}

type memoryBuildOutput struct {
	Project          string             `json:"project"`
	SessionsAnalyzed int                `json:"sessions_analyzed"`
	Decisions        int                `json:"decisions"`
	Constraints      int                `json:"constraints"`
	OpenQuestions    int                `json:"open_questions"`
	TopFiles         []memoryFileStat   `json:"top_files"`
	RecentWork       []memoryRecentItem `json:"recent_work"`
	OutputPath       string             `json:"output_path"`
	Apply            bool               `json:"apply"`
	DryRun           bool               `json:"dry_run"`
	Content          string             `json:"content,omitempty"`
}

func runMemoryBuild(cmd *cobra.Command, args []string) error {
	projectFilter := memoryProject
	if len(args) > 0 {
		projectFilter = args[0]
	}
	if projectFilter == "" && !memoryCWD {
		return fmt.Errorf("provide a project or use --cwd")
	}

	if memoryApply {
		memoryDryRun = false
	}
	if !memoryApply && !memoryDryRun {
		return fmt.Errorf("refusing to write without --apply")
	}

	claudeDir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: claudeDir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if memoryCWD {
		sessions = filterDistillSessions(sessions, "", true)
	} else {
		sessions = resolveProjectSessions(sessions, projectFilter, claudeDir)
	}
	if len(sessions) == 0 {
		return fmt.Errorf("no matching sessions found")
	}

	parsed := parseMemorySessions(sessions)
	if len(parsed) == 0 {
		return fmt.Errorf("no parseable sessions found")
	}

	projectName := parsed[0].info.ProjectName
	if projectName == "" {
		projectName = projectFilter
	}
	if projectName == "" {
		projectName = "unknown"
	}

	snapshot := buildMemoryVectorSnapshot(projectName, parsed)
	topFiles := buildMemoryFileStats(parsed, 12)
	recent := buildMemoryRecentWork(parsed, memoryDays, time.Now())
	content := renderMemoryMarkdown(projectName, parsed, snapshot, topFiles, recent, memoryDays, time.Now())

	outputPath := memoryOutput
	if outputPath == "" {
		outputPath = "project-vector.md"
	}

	if isJSON() {
		out := memoryBuildOutput{
			Project:          projectName,
			SessionsAnalyzed: len(parsed),
			Decisions:        len(snapshot.Decisions),
			Constraints:      len(snapshot.Constraints),
			OpenQuestions:    len(snapshot.Questions),
			TopFiles:         topFiles,
			RecentWork:       recent,
			OutputPath:       outputPath,
			Apply:            memoryApply,
			DryRun:           memoryDryRun,
		}
		if memoryDryRun {
			out.Content = content
		}
		if memoryApply {
			if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
				return fmt.Errorf("write memory artifact: %w", err)
			}
		}
		return printJSON(out)
	}

	if memoryDryRun {
		fmt.Printf("Dry run — would write %s\n\n", outputPath)
		fmt.Println(content)
		return nil
	}

	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write memory artifact: %w", err)
	}
	fmt.Printf("Memory artifact written: %s (%d sessions)\n", outputPath, len(parsed))
	return nil
}

func parseMemorySessions(sessions []session.Info) []memoryParsedSession {
	result := make([]memoryParsedSession, 0, len(sessions))
	for _, si := range sessions {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			continue
		}
		result = append(result, memoryParsedSession{
			info:    si,
			entries: entries,
		})
	}
	return result
}

func buildMemoryVectorSnapshot(projectName string, parsed []memoryParsedSession) *editor.VectorSnapshot {
	inputs := make([]analyzer.TopicSessionInput, 0, len(parsed))
	markers := make([]editor.VectorMarkerInput, 0, len(parsed))

	for _, ps := range parsed {
		inputs = append(inputs, analyzer.TopicSessionInput{
			Entries: ps.entries,
			Info: analyzer.SessionInfoLite{
				SessionID: ps.info.SessionID,
				Slug:      ps.info.Slug,
				Created:   ps.info.Created,
				Modified:  ps.info.Modified,
			},
		})
		mf, err := editor.LoadMarkers(ps.info.FullPath)
		if err == nil && len(mf.CommitPoints) > 0 {
			markers = append(markers, editor.VectorMarkerInput{
				SessionLabel: ps.info.DisplayName(),
				Markers:      mf,
			})
		}
	}

	ts := analyzer.CollectTopics(inputs)
	ts.ProjectName = projectName
	return editor.CollectVector(ts, markers)
}

func buildMemoryFileStats(parsed []memoryParsedSession, limit int) []memoryFileStat {
	type acc struct {
		touches  int
		sessions map[string]bool
	}
	files := make(map[string]*acc)

	for _, ps := range parsed {
		for _, e := range ps.entries {
			if e.Message == nil || e.Type != jsonl.TypeAssistant {
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
				path := analyzer.ExtractToolInputPath(b.Input)
				if strings.TrimSpace(path) == "" {
					continue
				}
				state := files[path]
				if state == nil {
					state = &acc{sessions: make(map[string]bool)}
					files[path] = state
				}
				state.touches++
				state.sessions[ps.info.SessionID] = true
			}
		}
	}

	stats := make([]memoryFileStat, 0, len(files))
	for path, state := range files {
		stats = append(stats, memoryFileStat{
			Path:     path,
			Sessions: len(state.sessions),
			Touches:  state.touches,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Touches != stats[j].Touches {
			return stats[i].Touches > stats[j].Touches
		}
		if stats[i].Sessions != stats[j].Sessions {
			return stats[i].Sessions > stats[j].Sessions
		}
		return stats[i].Path < stats[j].Path
	})
	if limit > 0 && len(stats) > limit {
		stats = stats[:limit]
	}
	return stats
}

func buildMemoryRecentWork(parsed []memoryParsedSession, days int, now time.Time) []memoryRecentItem {
	if days <= 0 {
		days = 7
	}
	cutoff := now.AddDate(0, 0, -days)
	out := make([]memoryRecentItem, 0, len(parsed))

	for _, ps := range parsed {
		if ps.info.Modified.IsZero() || ps.info.Modified.Before(cutoff) {
			continue
		}
		files, actions := summarizeSessionWork(ps.entries)
		item := memoryRecentItem{
			SessionID: ps.info.ShortID(),
			Label:     ps.info.DisplayName(),
			Modified:  ps.info.Modified.Format("2006-01-02"),
			Files:     files,
			Actions:   actions,
		}
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Modified != out[j].Modified {
			return out[i].Modified > out[j].Modified
		}
		return out[i].SessionID < out[j].SessionID
	})
	return out
}

func summarizeSessionWork(entries []jsonl.Entry) ([]string, []string) {
	fileCounts := make(map[string]int)
	actionCounts := make(map[string]int)

	for _, e := range entries {
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
			actionCounts[b.Name]++
			path := analyzer.ExtractToolInputPath(b.Input)
			if strings.TrimSpace(path) != "" {
				fileCounts[path]++
			}
		}
	}

	files := topCountLabels(fileCounts, 3, false)
	actions := topCountLabels(actionCounts, 3, true)
	return files, actions
}

func topCountLabels(counts map[string]int, limit int, includeCount bool) []string {
	type item struct {
		key   string
		count int
	}
	list := make([]item, 0, len(counts))
	for k, c := range counts {
		list = append(list, item{key: k, count: c})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].key < list[j].key
	})
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	out := make([]string, 0, len(list))
	for _, it := range list {
		if includeCount {
			out = append(out, fmt.Sprintf("%s(%d)", it.key, it.count))
		} else {
			out = append(out, it.key)
		}
	}
	return out
}

func renderMemoryMarkdown(projectName string, parsed []memoryParsedSession, snap *editor.VectorSnapshot, topFiles []memoryFileStat, recent []memoryRecentItem, days int, now time.Time) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Project Memory: %s\n", projectName)
	fmt.Fprintf(&b, "Generated by contextspectre memory build | %s | %d sessions analyzed\n\n",
		now.Format("2006-01-02"),
		len(parsed))

	b.WriteString("## Architecture decisions\n")
	if len(snap.Decisions) == 0 {
		b.WriteString("- (none)\n\n")
	} else {
		for _, d := range snap.Decisions {
			b.WriteString("- ")
			b.WriteString(strings.TrimSpace(d.Text))
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	b.WriteString("## Active constraints\n")
	if len(snap.Constraints) == 0 {
		b.WriteString("- (none)\n\n")
	} else {
		for _, c := range snap.Constraints {
			b.WriteString("- ")
			b.WriteString(strings.TrimSpace(c.Text))
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	b.WriteString("## Key files\n")
	if len(topFiles) == 0 {
		b.WriteString("- (none)\n\n")
	} else {
		for _, f := range topFiles {
			fmt.Fprintf(&b, "- %s — touched in %d sessions, %d references\n",
				f.Path, f.Sessions, f.Touches)
		}
		b.WriteByte('\n')
	}

	fmt.Fprintf(&b, "## Recent work (last %d days)\n", days)
	if len(recent) == 0 {
		b.WriteString("- (none)\n\n")
	} else {
		for _, r := range recent {
			fmt.Fprintf(&b, "- %s (%s) — files: %s; actions: %s\n",
				r.Label,
				r.Modified,
				joinOrPlaceholder(r.Files),
				joinOrPlaceholder(r.Actions))
		}
		b.WriteByte('\n')
	}

	b.WriteString("## Open questions\n")
	if len(snap.Questions) == 0 {
		b.WriteString("- (none)\n")
	} else {
		for _, q := range snap.Questions {
			b.WriteString("- ")
			b.WriteString(strings.TrimSpace(q.Text))
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func joinOrPlaceholder(items []string) string {
	if len(items) == 0 {
		return "n/a"
	}
	return strings.Join(items, ", ")
}

func init() {
	memoryBuildCmd.Flags().StringVar(&memoryProject, "project", "", "Project alias/name filter")
	memoryBuildCmd.Flags().BoolVar(&memoryCWD, "cwd", false, "Use sessions for current working directory")
	memoryBuildCmd.Flags().StringVar(&memoryOutput, "output", "project-vector.md", "Output file path")
	memoryBuildCmd.Flags().BoolVar(&memoryApply, "apply", false, "Write artifact to disk")
	memoryBuildCmd.Flags().BoolVar(&memoryDryRun, "dry-run", true, "Preview artifact without writing")
	memoryBuildCmd.Flags().IntVar(&memoryDays, "days", 7, "Recent work window in days")
	memoryCmd.AddCommand(memoryBuildCmd)
	rootCmd.AddCommand(memoryCmd)
}
