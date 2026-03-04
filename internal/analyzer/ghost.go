package analyzer

import (
	"sort"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// GhostFile represents a file where Claude's compaction summary may reference
// stale information because the file was modified in a subsequent epoch.
type GhostFile struct {
	Path            string
	CompactionIndex int
	EpochModified   int
}

// GhostReport holds all ghost context detections for a session.
type GhostReport struct {
	Files       []GhostFile
	TotalGhosts int
}

// DetectGhosts finds files where Claude's compaction summary references
// stale information. For each compaction N, it checks if files from the
// compacted epoch were modified (Write/Edit) in the next epoch.
func DetectGhosts(entries []jsonl.Entry, archaeology *CompactionReport, compactions []CompactionEvent) *GhostReport {
	report := &GhostReport{}

	if archaeology == nil || len(archaeology.Events) == 0 {
		return report
	}

	boundaries := make([]int, len(compactions))
	for i, c := range compactions {
		boundaries[i] = c.LineIndex
	}

	for i, event := range archaeology.Events {
		if len(event.Before.FilesReferenced) == 0 {
			continue
		}

		compactedFiles := make(map[string]bool, len(event.Before.FilesReferenced))
		for _, f := range event.Before.FilesReferenced {
			compactedFiles[f] = true
		}

		start, end := ghostEpochRange(i+1, boundaries, len(entries))
		if start >= end {
			continue
		}

		writtenFiles := extractWrittenFiles(entries[start:end])

		for path := range writtenFiles {
			if compactedFiles[path] {
				report.Files = append(report.Files, GhostFile{
					Path:            path,
					CompactionIndex: event.CompactionIndex,
					EpochModified:   i + 1,
				})
			}
		}
	}

	sort.Slice(report.Files, func(i, j int) bool {
		if report.Files[i].CompactionIndex != report.Files[j].CompactionIndex {
			return report.Files[i].CompactionIndex < report.Files[j].CompactionIndex
		}
		return report.Files[i].Path < report.Files[j].Path
	})

	report.TotalGhosts = len(report.Files)
	return report
}

// ghostEpochRange returns the start and end entry indices for a given epoch.
func ghostEpochRange(epochIdx int, boundaries []int, totalEntries int) (int, int) {
	var start int
	if epochIdx > 0 && epochIdx-1 < len(boundaries) {
		start = boundaries[epochIdx-1]
	}

	end := totalEntries
	if epochIdx < len(boundaries) {
		end = boundaries[epochIdx]
	}

	return start, end
}

// extractWrittenFiles scans entries for Write/Edit tool_use blocks
// and returns the set of file paths that were written.
func extractWrittenFiles(entries []jsonl.Entry) map[string]bool {
	written := make(map[string]bool)

	for _, e := range entries {
		if e.Type != jsonl.TypeAssistant || e.Message == nil {
			continue
		}
		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" && isWriteTool(b.Name) {
				path := ExtractToolInputPath(b.Input)
				if path != "" {
					written[path] = true
				}
			}
		}
	}

	return written
}

// isWriteTool returns true for tool names that explicitly write or edit files.
// Excludes Bash to avoid false positives from read-only shell commands.
func isWriteTool(name string) bool {
	switch name {
	case "Write", "Edit", "write_file", "edit_file", "WriteFile", "NotebookEdit":
		return true
	}
	return false
}
