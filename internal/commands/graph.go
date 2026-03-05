package commands

import (
	"fmt"
	"os"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	graphProject string
	graphCWD     bool
	graphMinWt   int
)

var graphCmd = &cobra.Command{
	Use:   "graph [session-id]",
	Short: "Show structural reasoning graph across sessions",
	Args:  cobra.MaximumNArgs(1),
	Long: `Build a project reasoning graph showing structural relationships between
sessions: shared files, decision references, continuity costs, project aliases,
and temporal proximity.

Edge types:
  alias       — sessions tied to the same project alias
  file_touch  — same file read/written across sessions (weighted)
  decision    — commit point files overlap with another session
  continuity  — cross-session file re-reads (with $ cost)
  temporal    — sessions within 24h of each other

Examples:
  contextspectre graph --project myapp
  contextspectre graph --cwd
  contextspectre graph --cwd --format text
  contextspectre graph 5d624f4a --format json`,
	RunE: runGraph,
}

func runGraph(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}

	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	projectFilter := graphProject
	useCWD := graphCWD

	if len(args) > 0 {
		targetPath := resolveSessionPath(args[0])
		for _, si := range sessions {
			if si.FullPath == targetPath || si.SessionID == args[0] || si.ShortID() == args[0] {
				if si.ProjectName != "" {
					projectFilter = si.ProjectName
				}
				break
			}
		}
	}

	if projectFilter == "" && !useCWD {
		return fmt.Errorf("--project, --cwd, or session-id is required")
	}

	filtered := filterDistillSessions(sessions, projectFilter, useCWD)
	if len(filtered) == 0 {
		if isJSON() {
			return printJSON(analyzer.ProjectGraph{
				Nodes: []analyzer.GraphNode{},
				Edges: []analyzer.GraphEdge{},
			})
		}
		fmt.Println("No matching sessions found.")
		return nil
	}

	inputs := buildGraphInputs(filtered)
	if len(inputs) == 0 {
		fmt.Println("No parseable sessions found.")
		return nil
	}

	graph := analyzer.BuildProjectGraph(inputs, graphMinWt)

	if isJSON() {
		return printJSON(graph)
	}

	printGraphText(graph)
	return nil
}

// buildGraphInputs parses JSONL and collects file touches + commit points for each session.
func buildGraphInputs(sessions []session.Info) []analyzer.GraphSessionInput {
	inputs := make([]analyzer.GraphSessionInput, 0, len(sessions))
	for _, si := range sessions {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: skip %s: %v\n", si.DisplayName(), err)
			continue
		}

		touches := analyzer.CollectFileTouches(entries)

		var commitPoints []analyzer.GraphCommitPoint
		markers, err := editor.LoadMarkers(si.FullPath)
		if err == nil && len(markers.CommitPoints) > 0 {
			for _, cp := range markers.CommitPoints {
				commitPoints = append(commitPoints, analyzer.GraphCommitPoint{
					Goal:  cp.Goal,
					Files: cp.Files,
				})
			}
		}

		cost := 0.0
		ctxPct := 0.0
		model := ""
		compactions := 0
		if si.ContextStats != nil {
			cost = si.ContextStats.EstimatedCost
			ctxPct = si.ContextStats.ContextPct
			model = si.ContextStats.Model
			compactions = si.ContextStats.CompactionCount
		}

		inputs = append(inputs, analyzer.GraphSessionInput{
			SessionID:       si.SessionID,
			Slug:            si.Slug,
			ProjectName:     si.ProjectName,
			ProjectPath:     si.ProjectPath,
			Created:         si.Created,
			Modified:        si.Modified,
			CompactionCount: compactions,
			EstimatedCost:   cost,
			ContextPct:      ctxPct,
			Model:           model,
			Entries:         entries,
			FilesTouched:    touches,
			CommitPoints:    commitPoints,
		})
	}
	return inputs
}

// printGraphText renders the graph as a human-readable adjacency list.
func printGraphText(g *analyzer.ProjectGraph) {
	fmt.Printf("Project reasoning graph — %d sessions, %d edges\n\n",
		g.Stats.TotalSessions, g.Stats.TotalEdges)

	if g.Stats.TotalEdges == 0 {
		fmt.Println("No structural relationships found.")
		return
	}

	// Group edges by source node.
	for _, node := range g.Nodes {
		edges := g.EdgesFrom(node.SessionID)
		if len(edges) == 0 {
			continue
		}

		name := node.Slug
		if name == "" {
			name = node.SessionID[:8]
		}
		fmt.Printf("%s (%d epochs, %s)\n", name, node.Epochs, analyzer.FormatCost(node.Cost))

		for _, e := range edges {
			other := e.To
			if other == node.SessionID {
				other = e.From
			}
			otherName := g.NodeBySlug(other)
			fmt.Printf("  %s %s\n", formatEdgeArrow(e), otherName)
		}
		fmt.Println()
	}

	// Summary.
	fmt.Printf("Files shared: %d\n", g.Stats.FilesShared)
	fmt.Printf("Decisions: %d\n", g.Stats.DecisionCount)
	if g.Stats.ContinuityCost > 0 {
		fmt.Printf("Continuity tax: %s\n", analyzer.FormatCost(g.Stats.ContinuityCost))
	}
	fmt.Printf("Total cost: %s\n", analyzer.FormatCost(g.Stats.TotalCost))
}

// formatEdgeArrow formats a single edge as a compact arrow string.
func formatEdgeArrow(e analyzer.GraphEdge) string {
	switch e.Type {
	case "alias":
		return fmt.Sprintf("─alias─> [%s]", e.Label)
	case "file_touch":
		path := e.Label
		if len(path) > 40 {
			path = "..." + path[len(path)-37:]
		}
		return fmt.Sprintf("─file(%d)─> [%s]", e.Weight, path)
	case "decision":
		label := e.Label
		if len(label) > 50 {
			label = label[:47] + "..."
		}
		return fmt.Sprintf("─decision─> [%s]", label)
	case "continuity":
		path := e.Label
		if len(path) > 40 {
			path = "..." + path[len(path)-37:]
		}
		return fmt.Sprintf("─continuity(%s)─> [%s]", analyzer.FormatCost(e.Cost), path)
	case "temporal":
		return fmt.Sprintf("─temporal─> [%s]", e.Label)
	default:
		return fmt.Sprintf("─%s─>", e.Type)
	}
}

func init() {
	graphCmd.Flags().StringVarP(&graphProject, "project", "p", "",
		"Filter sessions by project name")
	graphCmd.Flags().BoolVar(&graphCWD, "cwd", false,
		"Use sessions for the current working directory")
	graphCmd.Flags().IntVar(&graphMinWt, "min-weight", 2,
		"Minimum file_touch weight to include (reduces noise)")
	rootCmd.AddCommand(graphCmd)
}
