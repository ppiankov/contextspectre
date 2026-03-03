package analyzer

import (
	"time"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// TimeGapThreshold is the minimum time gap between consecutive entries
// that triggers a new branch within the same epoch.
const TimeGapThreshold = 30 * time.Minute

// Branch represents a contiguous segment of conversation entries,
// bounded by compaction events or significant time gaps.
type Branch struct {
	Index         int
	StartIdx      int    // first entry index (inclusive)
	EndIdx        int    // last entry index (inclusive)
	Summary       string // first user message text, truncated 60 chars
	TokenCost     int    // sum of RawSize/4
	EntryCount    int
	UserTurns     int // count of user messages
	TimeStart     time.Time
	TimeEnd       time.Time
	FileCount     int  // unique files from extractAllPaths
	HasCompaction bool // branch starts at a compaction boundary
	IsLast        bool
}

// FindBranches segments session entries into logical branches based on
// compaction boundaries and time gaps exceeding TimeGapThreshold.
func FindBranches(entries []jsonl.Entry, compactions []CompactionEvent) []Branch {
	if len(entries) == 0 {
		return nil
	}

	// Build compaction boundary set
	compactionAt := make(map[int]bool, len(compactions))
	for _, c := range compactions {
		compactionAt[c.LineIndex] = true
	}

	// Segment entries into branches
	type segment struct {
		startIdx      int
		endIdx        int
		hasCompaction bool
	}

	var segments []segment
	curStart := 0
	curCompaction := compactionAt[0]

	for i := 1; i < len(entries); i++ {
		isCompBoundary := compactionAt[i]
		isTimeGap := false

		if !isCompBoundary && !entries[i].Timestamp.IsZero() && !entries[i-1].Timestamp.IsZero() {
			gap := entries[i].Timestamp.Sub(entries[i-1].Timestamp)
			if gap >= TimeGapThreshold {
				isTimeGap = true
			}
		}

		if isCompBoundary || isTimeGap {
			segments = append(segments, segment{
				startIdx:      curStart,
				endIdx:        i - 1,
				hasCompaction: curCompaction,
			})
			curStart = i
			curCompaction = isCompBoundary
		}
	}

	// Final segment
	segments = append(segments, segment{
		startIdx:      curStart,
		endIdx:        len(entries) - 1,
		hasCompaction: curCompaction,
	})

	// Build branches from segments
	branches := make([]Branch, len(segments))
	for idx, seg := range segments {
		b := Branch{
			Index:         idx,
			StartIdx:      seg.startIdx,
			EndIdx:        seg.endIdx,
			EntryCount:    seg.endIdx - seg.startIdx + 1,
			HasCompaction: seg.hasCompaction,
		}

		fileSet := make(map[string]bool)
		for i := seg.startIdx; i <= seg.endIdx; i++ {
			e := entries[i]

			// Token cost
			b.TokenCost += e.RawSize / 4

			// User turns and summary
			if e.Type == jsonl.TypeUser {
				b.UserTurns++
				if b.Summary == "" {
					b.Summary = e.ContentPreview(60)
				}
			}

			// Timestamps
			if !e.Timestamp.IsZero() {
				if b.TimeStart.IsZero() || e.Timestamp.Before(b.TimeStart) {
					b.TimeStart = e.Timestamp
				}
				if e.Timestamp.After(b.TimeEnd) {
					b.TimeEnd = e.Timestamp
				}
			}

			// File count
			paths, _ := extractAllPaths(e)
			for _, p := range paths {
				fileSet[p] = true
			}
		}
		b.FileCount = len(fileSet)

		branches[idx] = b
	}

	if len(branches) > 0 {
		branches[len(branches)-1].IsLast = true
	}

	return branches
}
