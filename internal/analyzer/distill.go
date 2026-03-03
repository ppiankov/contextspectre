package analyzer

import (
	"sort"
	"time"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// Topic represents a conversation branch annotated with session-level metadata.
type Topic struct {
	GlobalIndex int
	SessionID   string
	SessionSlug string
	Branch      Branch
	Entries     []jsonl.Entry
	Compaction  *CompactionArchaeology
	CostDollars float64
}

// TopicSet holds all topics discovered across sessions for a project.
type TopicSet struct {
	ProjectName string
	Topics      []Topic
	Sessions    []TopicSessionInfo
	TotalTokens int
	TotalCost   float64
}

// TopicSessionInfo holds lightweight session metadata for the distill output header.
type TopicSessionInfo struct {
	SessionID  string
	Slug       string
	TopicCount int
	Created    time.Time
	Modified   time.Time
	Cost       float64
}

// TopicSessionInput is the input for CollectTopics.
type TopicSessionInput struct {
	Entries []jsonl.Entry
	Info    SessionInfoLite
}

// SessionInfoLite is the subset of session.Info needed by distill.
// Avoids circular import with session package.
type SessionInfoLite struct {
	SessionID string
	Slug      string
	Created   time.Time
	Modified  time.Time
}

// CollectTopics builds a TopicSet from parsed session data.
func CollectTopics(sessions []TopicSessionInput) *TopicSet {
	ts := &TopicSet{}
	globalIdx := 0

	for _, si := range sessions {
		filtered := filterNoise(si.Entries)
		if len(filtered) == 0 {
			continue
		}

		stats := Analyze(filtered)
		branches := FindBranches(filtered, stats.Compactions)

		var arch *CompactionReport
		if len(stats.Compactions) > 0 {
			arch = AnalyzeCompactions(filtered, stats.Compactions)
		}

		cwd := DetectSessionCWD(si.Entries)

		sessionInfo := TopicSessionInfo{
			SessionID:  si.Info.SessionID,
			Slug:       si.Info.Slug,
			TopicCount: len(branches),
			Created:    si.Info.Created,
			Modified:   si.Info.Modified,
		}

		for _, br := range branches {
			meta := ComputeRangeMetadata(filtered, br.StartIdx, br.EndIdx, cwd)

			topic := Topic{
				GlobalIndex: globalIdx,
				SessionID:   si.Info.SessionID,
				SessionSlug: si.Info.Slug,
				Branch:      br,
				Entries:     filtered[br.StartIdx : br.EndIdx+1],
				CostDollars: meta.DollarCost,
			}

			// Link archaeology for compaction-boundary branches
			if br.HasCompaction && arch != nil {
				for i := range arch.Events {
					if arch.Events[i].LineIndex <= br.StartIdx {
						topic.Compaction = &arch.Events[i]
					}
				}
			}

			ts.Topics = append(ts.Topics, topic)
			ts.TotalTokens += br.TokenCost
			ts.TotalCost += meta.DollarCost
			sessionInfo.Cost += meta.DollarCost
			globalIdx++
		}

		ts.Sessions = append(ts.Sessions, sessionInfo)
	}

	// Sort topics chronologically
	sort.Slice(ts.Topics, func(i, j int) bool {
		return ts.Topics[i].Branch.TimeStart.Before(ts.Topics[j].Branch.TimeStart)
	})

	// Reassign GlobalIndex after sort
	for i := range ts.Topics {
		ts.Topics[i].GlobalIndex = i
	}

	return ts
}

// filterNoise strips non-conversational noise entries in memory.
func filterNoise(entries []jsonl.Entry) []jsonl.Entry {
	result := make([]jsonl.Entry, 0, len(entries)/2)
	for _, e := range entries {
		switch e.Type {
		case jsonl.TypeProgress, jsonl.TypeFileHistorySnapshot:
			continue
		}
		if e.IsSidechain {
			continue
		}
		result = append(result, e)
	}
	return result
}
