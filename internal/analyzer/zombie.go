package analyzer

import "github.com/ppiankov/contextspectre/internal/jsonl"

// ZombieFileSizeThreshold is the file size above which zombie checks apply.
const ZombieFileSizeThreshold = 30 * 1024 * 1024 // 30 MB

// ZombieCompactionThreshold is the compaction count above which a session
// is considered at risk of being unusable.
const ZombieCompactionThreshold = 12

// ZombieState describes the liveness of a session.
type ZombieState struct {
	IsZombie bool   // true if session is likely unusable for active work
	Reason   string // human-readable explanation
	Signals  int    // number of zombie indicators triggered (0-3)
}

// DetectZombie checks if a session is in an unusable "zombie" state.
// A zombie session is too large for the Mac client to reload — it has historical
// value for search and review but cannot serve as an active conversation.
// Only Mac/desktop sessions can become zombies; CLI sessions handle large
// context without hitting "Prompt is too long" errors.
//
// Indicators (need 2+ to flag):
//   - File size > 30 MB
//   - Current context tokens == 0 (client lost the thread)
//   - Compaction count > 12 (heavily compressed, likely empty summaries)
func DetectZombie(stats *jsonl.LightStats) ZombieState {
	// Only desktop (Mac) sessions can become zombies
	if !stats.StartsWithQueueOp {
		return ZombieState{}
	}

	signals := 0

	if stats.FileSizeBytes > ZombieFileSizeThreshold {
		signals++
	}

	currentTokens := 0
	if stats.LastUsage != nil {
		currentTokens = stats.LastUsage.TotalContextTokens()
	}
	if currentTokens == 0 && stats.AssistantCount > 10 {
		signals++
	}

	if stats.CompactionCount > ZombieCompactionThreshold {
		signals++
	}

	if signals < 2 {
		return ZombieState{}
	}

	reason := "Session is too large for active use"
	if stats.FileSizeBytes > ZombieFileSizeThreshold && currentTokens == 0 {
		reason = "Session exceeds 30 MB and client has lost context (0 current tokens)"
	} else if stats.CompactionCount > ZombieCompactionThreshold && currentTokens == 0 {
		reason = "Session has 12+ compactions and client has lost context"
	} else if stats.FileSizeBytes > ZombieFileSizeThreshold && stats.CompactionCount > ZombieCompactionThreshold {
		reason = "Session exceeds 30 MB with 12+ compactions"
	}

	return ZombieState{
		IsZombie: true,
		Reason:   reason,
		Signals:  signals,
	}
}

// DetectZombieFromFull checks zombie state from full analysis stats.
func DetectZombieFromFull(stats *ContextStats, fileSizeBytes int64) ZombieState {
	light := &jsonl.LightStats{
		FileSizeBytes:     fileSizeBytes,
		AssistantCount:    stats.ConversationalTurns / 2,
		CompactionCount:   stats.CompactionCount,
		StartsWithQueueOp: stats.ClientType == "desktop",
	}
	if stats.CurrentContextTokens > 0 {
		light.LastUsage = &jsonl.Usage{
			InputTokens: stats.CurrentContextTokens,
		}
	}
	return DetectZombie(light)
}
