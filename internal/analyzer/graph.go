package analyzer

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// GraphNode represents a session in the project reasoning graph.
type GraphNode struct {
	SessionID   string    `json:"session_id"`
	Slug        string    `json:"slug"`
	ProjectName string    `json:"project_name,omitempty"`
	ProjectPath string    `json:"project_path,omitempty"`
	Created     time.Time `json:"created"`
	Modified    time.Time `json:"modified"`
	Epochs      int       `json:"epochs"`
	Cost        float64   `json:"cost"`
	ContextPct  float64   `json:"context_pct"`
}

// GraphEdge represents a structural relationship between two sessions.
type GraphEdge struct {
	Type   string  `json:"type"`
	From   string  `json:"from"`
	To     string  `json:"to"`
	Weight int     `json:"weight,omitempty"`
	Label  string  `json:"label,omitempty"`
	Cost   float64 `json:"cost,omitempty"`
}

// ProjectGraph is the complete reasoning graph for a set of sessions.
type ProjectGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	Stats GraphStats  `json:"stats"`
}

// GraphStats summarizes the graph.
type GraphStats struct {
	TotalSessions  int     `json:"total_sessions"`
	TotalEdges     int     `json:"total_edges"`
	TotalCost      float64 `json:"total_cost"`
	FilesShared    int     `json:"files_shared"`
	DecisionCount  int     `json:"decision_count"`
	ContinuityCost float64 `json:"continuity_cost"`
}

// GraphCommitPoint holds the subset of commit point data needed for graph edges.
// Avoids importing editor (which imports analyzer, causing a cycle).
type GraphCommitPoint struct {
	Goal  string
	Files []string
}

// GraphSessionInput is the pre-parsed data for a single session.
// Uses plain fields to avoid importing session (which imports analyzer).
type GraphSessionInput struct {
	SessionID       string
	Slug            string
	ProjectName     string
	ProjectPath     string
	Created         time.Time
	Modified        time.Time
	CompactionCount int
	EstimatedCost   float64
	ContextPct      float64
	Model           string
	Entries         []jsonl.Entry
	FilesTouched    map[string]int
	CommitPoints    []GraphCommitPoint
}

// BuildProjectGraph constructs the reasoning graph from pre-parsed session data.
func BuildProjectGraph(inputs []GraphSessionInput, minWeight int) *ProjectGraph {
	g := &ProjectGraph{
		Nodes: make([]GraphNode, 0, len(inputs)),
		Edges: make([]GraphEdge, 0),
	}

	var totalCost float64
	for _, inp := range inputs {
		totalCost += inp.EstimatedCost

		epochs := 1
		if inp.CompactionCount > 0 {
			epochs = inp.CompactionCount + 1
		}

		g.Nodes = append(g.Nodes, GraphNode{
			SessionID:   inp.SessionID,
			Slug:        inp.Slug,
			ProjectName: inp.ProjectName,
			ProjectPath: inp.ProjectPath,
			Created:     inp.Created,
			Modified:    inp.Modified,
			Epochs:      epochs,
			Cost:        inp.EstimatedCost,
			ContextPct:  inp.ContextPct,
		})
	}

	aliasEdges := buildAliasEdges(inputs)
	fileEdges, filesShared := buildFileTouchEdges(inputs, minWeight)
	decisionEdges, decisionCount := buildDecisionEdges(inputs)
	continuityEdges, continuityCost := buildContinuityEdges(inputs)
	temporalEdges := buildTemporalEdges(inputs)

	g.Edges = append(g.Edges, aliasEdges...)
	g.Edges = append(g.Edges, fileEdges...)
	g.Edges = append(g.Edges, decisionEdges...)
	g.Edges = append(g.Edges, continuityEdges...)
	g.Edges = append(g.Edges, temporalEdges...)

	g.Stats = GraphStats{
		TotalSessions:  len(inputs),
		TotalEdges:     len(g.Edges),
		TotalCost:      totalCost,
		FilesShared:    filesShared,
		DecisionCount:  decisionCount,
		ContinuityCost: continuityCost,
	}

	return g
}

// buildAliasEdges creates edges between sessions sharing the same project alias.
func buildAliasEdges(inputs []GraphSessionInput) []GraphEdge {
	var edges []GraphEdge
	for i := 0; i < len(inputs); i++ {
		nameI := inputs[i].ProjectName
		if nameI == "" {
			continue
		}
		for j := i + 1; j < len(inputs); j++ {
			if inputs[j].ProjectName == nameI {
				edges = append(edges, GraphEdge{
					Type:  "alias",
					From:  inputs[i].SessionID,
					To:    inputs[j].SessionID,
					Label: nameI,
				})
			}
		}
	}
	return edges
}

// buildFileTouchEdges creates edges for files touched across multiple sessions.
func buildFileTouchEdges(inputs []GraphSessionInput, minWeight int) ([]GraphEdge, int) {
	// file → list of (sessionIdx, count)
	type touchInfo struct {
		sessionIdx int
		count      int
	}
	fileSessions := make(map[string][]touchInfo)

	for idx, inp := range inputs {
		for path, count := range inp.FilesTouched {
			fileSessions[path] = append(fileSessions[path], touchInfo{idx, count})
		}
	}

	filesShared := 0
	var edges []GraphEdge

	for path, touches := range fileSessions {
		if len(touches) < 2 {
			continue
		}
		filesShared++
		for i := 0; i < len(touches); i++ {
			for j := i + 1; j < len(touches); j++ {
				weight := touches[i].count
				if touches[j].count < weight {
					weight = touches[j].count
				}
				if weight < minWeight {
					continue
				}
				edges = append(edges, GraphEdge{
					Type:   "file_touch",
					From:   inputs[touches[i].sessionIdx].SessionID,
					To:     inputs[touches[j].sessionIdx].SessionID,
					Weight: weight,
					Label:  path,
				})
			}
		}
	}

	return edges, filesShared
}

// buildDecisionEdges creates edges from commit point file overlap.
func buildDecisionEdges(inputs []GraphSessionInput) ([]GraphEdge, int) {
	var edges []GraphEdge
	decisionCount := 0

	for i, inp := range inputs {
		for _, cp := range inp.CommitPoints {
			decisionCount++
			if len(cp.Files) == 0 {
				continue
			}
			cpFiles := make(map[string]bool, len(cp.Files))
			for _, f := range cp.Files {
				cpFiles[f] = true
			}
			for j, other := range inputs {
				if i == j {
					continue
				}
				for f := range other.FilesTouched {
					if cpFiles[f] {
						edges = append(edges, GraphEdge{
							Type:  "decision",
							From:  inp.SessionID,
							To:    other.SessionID,
							Label: cp.Goal,
						})
						break
					}
				}
			}
		}
	}

	return edges, decisionCount
}

// buildContinuityEdges uses AnalyzeContinuity to find cross-session file re-reads.
func buildContinuityEdges(inputs []GraphSessionInput) ([]GraphEdge, float64) {
	if len(inputs) < 2 {
		return nil, 0
	}

	ciInputs := make([]ContinuitySessionInput, 0, len(inputs))
	for _, inp := range inputs {
		ciInputs = append(ciInputs, ContinuitySessionInput{
			SessionID:   inp.SessionID,
			SessionSlug: inp.Slug,
			Entries:     inp.Entries,
			Model:       inp.Model,
		})
	}

	report := AnalyzeContinuity(ciInputs)

	// Build session ID lookup by slug/shortID (same logic as continuity uses for session labels).
	slugToID := make(map[string]string, len(inputs))
	for _, inp := range inputs {
		label := inp.Slug
		if label == "" && len(inp.SessionID) >= 8 {
			label = inp.SessionID[:8]
		}
		if label == "" {
			label = inp.SessionID
		}
		slugToID[label] = inp.SessionID
	}

	// For each repeated file, create edges between all session pairs.
	seen := make(map[string]bool)
	var edges []GraphEdge
	totalCost := report.TotalTaxCost

	for _, rf := range report.RepeatedFiles {
		if len(rf.Sessions) < 2 {
			continue
		}
		for i := 0; i < len(rf.Sessions); i++ {
			for j := i + 1; j < len(rf.Sessions); j++ {
				fromID := slugToID[rf.Sessions[i]]
				toID := slugToID[rf.Sessions[j]]
				if fromID == "" || toID == "" {
					continue
				}
				key := fromID + ":" + toID
				if fromID > toID {
					key = toID + ":" + fromID
				}
				if seen[key] {
					continue
				}
				seen[key] = true
				edges = append(edges, GraphEdge{
					Type:  "continuity",
					From:  fromID,
					To:    toID,
					Cost:  rf.EstimatedCost,
					Label: rf.Path,
				})
			}
		}
	}

	return edges, totalCost
}

// buildTemporalEdges connects sessions within 24h of each other.
func buildTemporalEdges(inputs []GraphSessionInput) []GraphEdge {
	const threshold = 24 * time.Hour
	var edges []GraphEdge

	for i := 0; i < len(inputs); i++ {
		for j := i + 1; j < len(inputs); j++ {
			nameI := inputs[i].ProjectName
			nameJ := inputs[j].ProjectName
			if nameI == "" || nameI != nameJ {
				continue
			}
			// Check if sessions overlap or are close in time.
			gap := inputs[j].Created.Sub(inputs[i].Modified)
			if gap < 0 {
				gap = inputs[i].Created.Sub(inputs[j].Modified)
			}
			if gap < 0 {
				gap = 0 // overlapping sessions
			}
			if gap <= threshold {
				label := formatDuration(gap)
				edges = append(edges, GraphEdge{
					Type:  "temporal",
					From:  inputs[i].SessionID,
					To:    inputs[j].SessionID,
					Label: label,
				})
			}
		}
	}

	return edges
}

// CollectFileTouches scans entries for file read/write/edit operations and returns path → count.
func CollectFileTouches(entries []jsonl.Entry) map[string]int {
	touches := make(map[string]int)
	for _, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type != "tool_use" || !isFileTouchTool(b.Name) {
				continue
			}
			path := extractGraphToolPath(b.Input)
			if path != "" {
				touches[path]++
			}
		}
	}
	return touches
}

// isFileTouchTool returns true for tool names that read or write files.
func isFileTouchTool(name string) bool {
	switch name {
	case "Read", "View", "read_file", "ReadFile",
		"Write", "write_file", "WriteFile",
		"Edit", "edit_file", "EditFile",
		"Glob", "glob":
		return true
	}
	return false
}

// extractGraphToolPath extracts file_path or path from tool input JSON.
func extractGraphToolPath(input json.RawMessage) string {
	var fields struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
		Pattern  string `json:"pattern"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	if fields.FilePath != "" {
		return fields.FilePath
	}
	if fields.Path != "" {
		return fields.Path
	}
	return ""
}

// formatDuration formats a duration as a human-readable gap label.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m gap"
	}
	if d < time.Hour {
		m := int(d.Minutes())
		return fmt.Sprintf("%dm gap", m)
	}
	h := int(d.Hours())
	return fmt.Sprintf("%dh gap", h)
}

// NodeBySlug returns the slug or short ID for a session ID in the graph.
func (g *ProjectGraph) NodeBySlug(sessionID string) string {
	for _, n := range g.Nodes {
		if n.SessionID == sessionID {
			if n.Slug != "" {
				return n.Slug
			}
			if len(n.SessionID) >= 8 {
				return n.SessionID[:8]
			}
			return n.SessionID
		}
	}
	if len(sessionID) >= 8 {
		return sessionID[:8]
	}
	return sessionID
}

// EdgesFrom returns all edges originating from or connecting to the given session.
func (g *ProjectGraph) EdgesFrom(sessionID string) []GraphEdge {
	var result []GraphEdge
	for _, e := range g.Edges {
		if e.From == sessionID || e.To == sessionID {
			result = append(result, e)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Type < result[j].Type
	})
	return result
}
