package analyzer

import (
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestDetectZombie_NotZombie(t *testing.T) {
	stats := &jsonl.LightStats{
		FileSizeBytes:   5 * 1024 * 1024, // 5 MB — well under threshold
		AssistantCount:  20,
		CompactionCount: 3,
		LastUsage:       &jsonl.Usage{InputTokens: 50000},
	}
	z := DetectZombie(stats)
	if z.IsZombie {
		t.Error("expected non-zombie for small healthy session")
	}
}

func TestDetectZombie_LargeAndNoTokens(t *testing.T) {
	stats := &jsonl.LightStats{
		FileSizeBytes:   40 * 1024 * 1024, // 40 MB
		AssistantCount:  50,
		CompactionCount: 5,
		// No LastUsage — 0 current tokens
	}
	z := DetectZombie(stats)
	if !z.IsZombie {
		t.Error("expected zombie: large file + 0 tokens")
	}
	if z.Signals != 2 {
		t.Errorf("expected 2 signals, got %d", z.Signals)
	}
}

func TestDetectZombie_LargeAndManyCompactions(t *testing.T) {
	stats := &jsonl.LightStats{
		FileSizeBytes:   35 * 1024 * 1024,
		AssistantCount:  100,
		CompactionCount: 15,
		LastUsage:       &jsonl.Usage{InputTokens: 80000},
	}
	z := DetectZombie(stats)
	if !z.IsZombie {
		t.Error("expected zombie: large file + 15 compactions")
	}
	if z.Signals != 2 {
		t.Errorf("expected 2 signals, got %d", z.Signals)
	}
}

func TestDetectZombie_AllThreeSignals(t *testing.T) {
	stats := &jsonl.LightStats{
		FileSizeBytes:   43 * 1024 * 1024,
		AssistantCount:  200,
		CompactionCount: 17,
		// No LastUsage — 0 tokens
	}
	z := DetectZombie(stats)
	if !z.IsZombie {
		t.Error("expected zombie: all three signals")
	}
	if z.Signals != 3 {
		t.Errorf("expected 3 signals, got %d", z.Signals)
	}
}

func TestDetectZombie_OnlyOneSignal(t *testing.T) {
	stats := &jsonl.LightStats{
		FileSizeBytes:   35 * 1024 * 1024, // large but...
		AssistantCount:  50,
		CompactionCount: 5,                               // not many compactions
		LastUsage:       &jsonl.Usage{InputTokens: 9000}, // has tokens
	}
	z := DetectZombie(stats)
	if z.IsZombie {
		t.Error("expected non-zombie: only 1 signal (size)")
	}
}

func TestDetectZombieFromFull(t *testing.T) {
	stats := &ContextStats{
		ConversationalTurns:  100,
		CompactionCount:      15,
		CurrentContextTokens: 0,
	}
	z := DetectZombieFromFull(stats, 40*1024*1024)
	if !z.IsZombie {
		t.Error("expected zombie from full stats")
	}
}
