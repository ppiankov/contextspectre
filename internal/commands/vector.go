package commands

import (
	"fmt"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	vectorProject string
	vectorCWD     bool
	vectorOutput  string
)

var vectorCmd = &cobra.Command{
	Use:   "vector",
	Short: "Extract a compact project north star from all sessions",
	Long: `Scan all sessions for a project and extract decisions, constraints,
and open questions into a compact snapshot.

Sources (by priority):
  1. Commit points from .markers.json sidecar files
  2. Compaction archaeology (decision hints, user questions)

Examples:
  contextspectre vector --cwd
  contextspectre vector --project myapp
  contextspectre vector --cwd --output north-star.md
  contextspectre vector --cwd --format json`,
	RunE: runVector,
}

func runVector(cmd *cobra.Command, args []string) error {
	if vectorProject == "" && !vectorCWD {
		return fmt.Errorf("--project or --cwd is required")
	}

	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}

	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	filtered := filterDistillSessions(sessions, vectorProject, vectorCWD)
	if len(filtered) == 0 {
		if isJSON() {
			return printJSON(VectorOutputJSON{
				Decisions:   []VectorItemJSON{},
				Constraints: []VectorItemJSON{},
				Questions:   []VectorItemJSON{},
				Files:       []string{},
			})
		}
		fmt.Println("No matching sessions found.")
		return nil
	}

	projectName := filtered[0].ProjectName
	if projectName == "" {
		projectName = "unknown"
	}

	var inputs []analyzer.TopicSessionInput
	for _, si := range filtered {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			continue
		}
		inputs = append(inputs, analyzer.TopicSessionInput{
			Entries: entries,
			Info: analyzer.SessionInfoLite{
				SessionID: si.SessionID,
				Slug:      si.Slug,
				Created:   si.Created,
				Modified:  si.Modified,
			},
		})
	}

	ts := analyzer.CollectTopics(inputs)
	ts.ProjectName = projectName

	var markerInputs []editor.VectorMarkerInput
	for _, si := range filtered {
		mf, err := editor.LoadMarkers(si.FullPath)
		if err != nil {
			continue
		}
		if len(mf.CommitPoints) == 0 {
			continue
		}
		markerInputs = append(markerInputs, editor.VectorMarkerInput{
			SessionLabel: si.DisplayName(),
			Markers:      mf,
		})
	}

	snap := editor.CollectVector(ts, markerInputs)

	if isJSON() {
		return printJSON(buildVectorOutputJSON(snap))
	}

	if vectorOutput == "" {
		vectorOutput = fmt.Sprintf("vector-%s.md", sanitizeFilename(projectName))
	}

	if err := editor.RenderVector(snap, vectorOutput); err != nil {
		return fmt.Errorf("render vector: %w", err)
	}

	totalItems := len(snap.Decisions) + len(snap.Constraints) + len(snap.Questions)
	fmt.Printf("Vector snapshot: %d items from %d sessions → %s\n",
		totalItems, snap.SessionsScanned, vectorOutput)
	fmt.Printf("  Decisions: %d  Constraints: %d  Questions: %d  Files: %d\n",
		len(snap.Decisions), len(snap.Constraints), len(snap.Questions), len(snap.Files))

	return nil
}

func buildVectorOutputJSON(snap *editor.VectorSnapshot) *VectorOutputJSON {
	out := &VectorOutputJSON{
		ProjectName:     snap.ProjectName,
		SnapshotDate:    snap.SnapshotDate.Format(time.RFC3339),
		SessionsScanned: snap.SessionsScanned,
	}

	for _, d := range snap.Decisions {
		out.Decisions = append(out.Decisions, VectorItemJSON{
			Text:       d.Text,
			Source:     d.Source,
			SourceType: string(d.SourceType),
			Epoch:      d.Epoch,
		})
	}
	for _, c := range snap.Constraints {
		out.Constraints = append(out.Constraints, VectorItemJSON{
			Text:       c.Text,
			Source:     c.Source,
			SourceType: string(c.SourceType),
			Epoch:      c.Epoch,
		})
	}
	for _, q := range snap.Questions {
		out.Questions = append(out.Questions, VectorItemJSON{
			Text:       q.Text,
			Source:     q.Source,
			SourceType: string(q.SourceType),
			Epoch:      q.Epoch,
		})
	}

	out.Files = snap.Files
	if out.Decisions == nil {
		out.Decisions = []VectorItemJSON{}
	}
	if out.Constraints == nil {
		out.Constraints = []VectorItemJSON{}
	}
	if out.Questions == nil {
		out.Questions = []VectorItemJSON{}
	}
	if out.Files == nil {
		out.Files = []string{}
	}

	return out
}

func init() {
	vectorCmd.Flags().StringVar(&vectorProject, "project", "", "Filter sessions by project name (substring match)")
	vectorCmd.Flags().BoolVar(&vectorCWD, "cwd", false, "Use sessions for the current working directory")
	vectorCmd.Flags().StringVar(&vectorOutput, "output", "", "Output file path (default: vector-<project>.md)")
	rootCmd.AddCommand(vectorCmd)
}
