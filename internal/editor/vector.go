package editor

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
)

// VectorSourceType identifies where a vector item was extracted from.
type VectorSourceType string

const (
	VectorSourceCommitPoint VectorSourceType = "commit_point"
	VectorSourceArchaeology VectorSourceType = "archaeology"
)

// VectorItem is a single extracted decision, constraint, or question.
type VectorItem struct {
	Text       string
	Source     string
	SourceType VectorSourceType
	Epoch      int
}

// VectorSnapshot holds the aggregated project vector.
type VectorSnapshot struct {
	ProjectName     string
	SnapshotDate    time.Time
	Decisions       []VectorItem
	Constraints     []VectorItem
	Questions       []VectorItem
	Files           []string
	SessionsScanned int
}

// VectorMarkerInput pairs a session label with its loaded markers.
type VectorMarkerInput struct {
	SessionLabel string
	Markers      *MarkerFile
}

// CollectVector extracts decisions, constraints, and questions from topics and commit points.
func CollectVector(ts *analyzer.TopicSet, markers []VectorMarkerInput) *VectorSnapshot {
	snap := &VectorSnapshot{
		ProjectName:     ts.ProjectName,
		SnapshotDate:    time.Now(),
		SessionsScanned: len(ts.Sessions),
	}

	fileSet := make(map[string]bool)

	// Phase 1: CommitPoint data (highest priority — added first, wins dedup)
	for _, mi := range markers {
		if mi.Markers == nil {
			continue
		}
		for _, cp := range mi.Markers.CommitPoints {
			for _, d := range cp.Decisions {
				snap.Decisions = append(snap.Decisions, VectorItem{
					Text:       d,
					Source:     mi.SessionLabel,
					SourceType: VectorSourceCommitPoint,
					Epoch:      -1,
				})
			}
			for _, c := range cp.Constraints {
				snap.Constraints = append(snap.Constraints, VectorItem{
					Text:       c,
					Source:     mi.SessionLabel,
					SourceType: VectorSourceCommitPoint,
					Epoch:      -1,
				})
			}
			for _, q := range cp.Questions {
				snap.Questions = append(snap.Questions, VectorItem{
					Text:       q,
					Source:     mi.SessionLabel,
					SourceType: VectorSourceCommitPoint,
					Epoch:      -1,
				})
			}
			for _, f := range cp.Files {
				fileSet[f] = true
			}
		}
	}

	// Phase 2: Archaeology data from topics
	for _, t := range ts.Topics {
		if t.Compaction == nil {
			continue
		}
		sessionLabel := t.SessionSlug
		if sessionLabel == "" && len(t.SessionID) >= 8 {
			sessionLabel = t.SessionID[:8]
		}
		epoch := t.Compaction.CompactionIndex

		for _, d := range t.Compaction.Before.DecisionHints {
			snap.Decisions = append(snap.Decisions, VectorItem{
				Text:       d,
				Source:     sessionLabel,
				SourceType: VectorSourceArchaeology,
				Epoch:      epoch,
			})
		}
		for _, q := range t.Compaction.Before.UserQuestions {
			snap.Questions = append(snap.Questions, VectorItem{
				Text:       q,
				Source:     sessionLabel,
				SourceType: VectorSourceArchaeology,
				Epoch:      epoch,
			})
		}
		for _, f := range t.Compaction.Before.FilesReferenced {
			fileSet[f] = true
		}
	}

	snap.Decisions = DeduplicateItems(snap.Decisions)
	snap.Constraints = DeduplicateItems(snap.Constraints)
	snap.Questions = DeduplicateItems(snap.Questions)

	for f := range fileSet {
		snap.Files = append(snap.Files, f)
	}
	sort.Strings(snap.Files)
	if len(snap.Files) > 20 {
		snap.Files = snap.Files[:20]
	}

	return snap
}

// DeduplicateItems removes items with identical normalized text.
// First occurrence wins, preserving source priority ordering.
func DeduplicateItems(items []VectorItem) []VectorItem {
	seen := make(map[string]bool, len(items))
	var result []VectorItem
	for _, item := range items {
		key := normalizeForDedup(item.Text)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func normalizeForDedup(s string) string {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	return strings.ToLower(strings.Join(fields, " "))
}

// RenderVector writes the snapshot to a compact markdown file.
func RenderVector(snap *VectorSnapshot, outputPath string) error {
	var b strings.Builder

	fmt.Fprintf(&b, "# Project Vector — %s\n\n", snap.ProjectName)
	fmt.Fprintf(&b, "Snapshot: %s | %d sessions scanned\n\n",
		snap.SnapshotDate.Format("2006-01-02"), snap.SessionsScanned)

	if len(snap.Decisions) > 0 {
		b.WriteString("## Decisions\n\n")
		for _, item := range snap.Decisions {
			fmt.Fprintf(&b, "- %s (%s)\n", item.Text, formatVectorSource(item))
		}
		b.WriteString("\n")
	}

	if len(snap.Constraints) > 0 {
		b.WriteString("## Constraints\n\n")
		for _, item := range snap.Constraints {
			fmt.Fprintf(&b, "- %s (%s)\n", item.Text, formatVectorSource(item))
		}
		b.WriteString("\n")
	}

	if len(snap.Questions) > 0 {
		b.WriteString("## Open Questions\n\n")
		for _, item := range snap.Questions {
			fmt.Fprintf(&b, "- %s (%s)\n", item.Text, formatVectorSource(item))
		}
		b.WriteString("\n")
	}

	if len(snap.Files) > 0 {
		b.WriteString("## Key Files\n\n")
		for _, f := range snap.Files {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}

	return os.WriteFile(outputPath, []byte(b.String()), 0o644)
}

func formatVectorSource(item VectorItem) string {
	switch item.SourceType {
	case VectorSourceCommitPoint:
		return fmt.Sprintf("session: %s, commit point", item.Source)
	case VectorSourceArchaeology:
		if item.Epoch >= 0 {
			return fmt.Sprintf("session: %s, epoch %d", item.Source, item.Epoch)
		}
		return fmt.Sprintf("session: %s", item.Source)
	default:
		return fmt.Sprintf("session: %s", item.Source)
	}
}
