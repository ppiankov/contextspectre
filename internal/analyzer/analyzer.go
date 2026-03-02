package analyzer

import (
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// ContextWindowSize is the standard Claude context window size in tokens.
const ContextWindowSize = 200000

// CompactionThreshold is the observed token count where Claude Code
// triggers automatic compaction (~165K-170K tokens).
const CompactionThreshold = 165000

// CompactionDropThreshold is the minimum token decrease between consecutive
// assistant messages that indicates a compaction event occurred.
const CompactionDropThreshold = 50000

// ContextStats holds comprehensive analysis results for a session.
type ContextStats struct {
	TotalLines           int
	MessageCounts        map[jsonl.MessageType]int
	CurrentContextTokens int
	MaxContextTokens     int
	UsagePercent         float64
	CompactionCount      int
	Compactions          []CompactionEvent
	TokenGrowthRate      float64
	EstimatedTurnsLeft   int
	FileSizeBytes        int64
	ImageCount           int
	ImageBytesTotal      int64
	SnapshotCount        int
	SnapshotBytesTotal   int64
	ConversationalTurns  int
	LastCompactionLine   int
}

// CompactionEvent records a detected context compaction.
type CompactionEvent struct {
	LineIndex    int
	BeforeTokens int
	AfterTokens  int
	TokensDrop   int
}

// Analyze performs a full analysis of parsed session entries.
func Analyze(entries []jsonl.Entry) *ContextStats {
	stats := &ContextStats{
		TotalLines:    len(entries),
		MessageCounts: make(map[jsonl.MessageType]int),
	}

	var prevContextTokens int
	var postCompactionTokens int
	var turnsSinceCompaction int

	for i, e := range entries {
		stats.MessageCounts[e.Type]++
		stats.FileSizeBytes += int64(e.RawSize)

		if e.IsConversational() {
			stats.ConversationalTurns++
		}

		// Track snapshots
		if e.Type == jsonl.TypeFileHistorySnapshot {
			stats.SnapshotCount++
			stats.SnapshotBytesTotal += int64(e.RawSize)
		}

		// Track images
		if e.HasImages() {
			blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
			if err == nil {
				for _, b := range blocks {
					if b.Type == "image" && b.Source != nil {
						stats.ImageCount++
						stats.ImageBytesTotal += int64(len(b.Source.Data) * 3 / 4)
					}
				}
			}
		}

		// Track context usage from assistant messages
		if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Usage != nil {
			ctx := e.Message.Usage.TotalContextTokens()

			if ctx > stats.MaxContextTokens {
				stats.MaxContextTokens = ctx
			}

			// Detect compaction: large drop in context tokens
			if prevContextTokens > 0 && prevContextTokens-ctx > CompactionDropThreshold {
				event := CompactionEvent{
					LineIndex:    i,
					BeforeTokens: prevContextTokens,
					AfterTokens:  ctx,
					TokensDrop:   prevContextTokens - ctx,
				}
				stats.Compactions = append(stats.Compactions, event)
				stats.CompactionCount++
				stats.LastCompactionLine = i
				postCompactionTokens = ctx
				turnsSinceCompaction = 0
			} else if stats.CompactionCount > 0 {
				turnsSinceCompaction++
			}

			stats.CurrentContextTokens = ctx
			prevContextTokens = ctx
		}
	}

	// Calculate usage percentage
	if ContextWindowSize > 0 {
		stats.UsagePercent = float64(stats.CurrentContextTokens) / float64(ContextWindowSize) * 100
	}

	// Calculate token growth rate (avg tokens per conversational turn since last compaction)
	if stats.CompactionCount > 0 && turnsSinceCompaction > 0 {
		growth := stats.CurrentContextTokens - postCompactionTokens
		stats.TokenGrowthRate = float64(growth) / float64(turnsSinceCompaction)
	} else if stats.ConversationalTurns > 0 {
		// No compaction yet — use overall growth rate
		stats.TokenGrowthRate = float64(stats.CurrentContextTokens) / float64(stats.ConversationalTurns)
	}

	// Estimate turns until next compaction
	stats.EstimatedTurnsLeft = CompactionDistance(stats)

	return stats
}

// CompactionDistance estimates the number of conversational turns
// remaining before the next automatic compaction triggers.
func CompactionDistance(stats *ContextStats) int {
	if stats.TokenGrowthRate <= 0 {
		return -1
	}
	tokensRemaining := CompactionThreshold - stats.CurrentContextTokens
	if tokensRemaining <= 0 {
		return 0
	}
	return int(float64(tokensRemaining) / stats.TokenGrowthRate)
}
