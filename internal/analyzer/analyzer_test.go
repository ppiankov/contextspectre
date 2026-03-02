package analyzer

import (
	"path/filepath"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func testdataPath(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

func TestAnalyze_SmallSession(t *testing.T) {
	entries, err := jsonl.Parse(testdataPath("small_session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	stats := Analyze(entries)

	if stats.TotalLines != 10 {
		t.Errorf("expected 10 lines, got %d", stats.TotalLines)
	}
	if stats.MessageCounts[jsonl.TypeUser] != 4 {
		t.Errorf("expected 4 user messages, got %d", stats.MessageCounts[jsonl.TypeUser])
	}
	if stats.MessageCounts[jsonl.TypeAssistant] != 3 {
		t.Errorf("expected 3 assistant messages, got %d", stats.MessageCounts[jsonl.TypeAssistant])
	}

	// Last assistant has 300+5500+200 = 6000
	if stats.CurrentContextTokens != 6000 {
		t.Errorf("expected 6000 current context tokens, got %d", stats.CurrentContextTokens)
	}
	if stats.MaxContextTokens != 6000 {
		t.Errorf("expected 6000 max context tokens, got %d", stats.MaxContextTokens)
	}

	// No compaction in small session
	if stats.CompactionCount != 0 {
		t.Errorf("expected 0 compactions, got %d", stats.CompactionCount)
	}

	// Usage percent: 6000/200000 = 3%
	if stats.UsagePercent < 2.9 || stats.UsagePercent > 3.1 {
		t.Errorf("expected ~3%% usage, got %.1f%%", stats.UsagePercent)
	}
}

func TestAnalyze_Compaction(t *testing.T) {
	entries, err := jsonl.Parse(testdataPath("compaction.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	stats := Analyze(entries)

	// Should detect the compaction event (165300 → 35100)
	if stats.CompactionCount != 1 {
		t.Errorf("expected 1 compaction, got %d", stats.CompactionCount)
	}

	if len(stats.Compactions) != 1 {
		t.Fatalf("expected 1 compaction event, got %d", len(stats.Compactions))
	}

	event := stats.Compactions[0]
	if event.BeforeTokens != 165300 {
		t.Errorf("expected before=165300, got %d", event.BeforeTokens)
	}
	if event.AfterTokens != 35100 {
		t.Errorf("expected after=35100, got %d", event.AfterTokens)
	}

	// Current should be the last assistant's tokens (65200)
	if stats.CurrentContextTokens != 65200 {
		t.Errorf("expected 65200 current tokens, got %d", stats.CurrentContextTokens)
	}

	// Max should be 165300 (pre-compaction peak)
	if stats.MaxContextTokens != 165300 {
		t.Errorf("expected 165300 max tokens, got %d", stats.MaxContextTokens)
	}

	// Should have positive growth rate and turns estimate
	if stats.TokenGrowthRate <= 0 {
		t.Errorf("expected positive growth rate, got %f", stats.TokenGrowthRate)
	}
	if stats.EstimatedTurnsLeft <= 0 {
		t.Errorf("expected positive turns left, got %d", stats.EstimatedTurnsLeft)
	}
}

func TestAnalyze_WithImages(t *testing.T) {
	entries, err := jsonl.Parse(testdataPath("with_images.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	stats := Analyze(entries)

	if stats.ImageCount != 2 {
		t.Errorf("expected 2 images, got %d", stats.ImageCount)
	}
	if stats.ImageBytesTotal <= 0 {
		t.Error("expected positive image bytes")
	}
}

func TestCompactionDistance(t *testing.T) {
	tests := []struct {
		name  string
		stats *ContextStats
		want  int
	}{
		{
			name:  "zero growth rate",
			stats: &ContextStats{CurrentContextTokens: 100000, TokenGrowthRate: 0},
			want:  -1,
		},
		{
			name:  "already at threshold",
			stats: &ContextStats{CurrentContextTokens: 170000, TokenGrowthRate: 1000},
			want:  0,
		},
		{
			name:  "50K remaining at 5K per turn",
			stats: &ContextStats{CurrentContextTokens: 115000, TokenGrowthRate: 5000},
			want:  10,
		},
		{
			name:  "low usage",
			stats: &ContextStats{CurrentContextTokens: 5000, TokenGrowthRate: 2000},
			want:  80,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompactionDistance(tt.stats)
			if got != tt.want {
				t.Errorf("CompactionDistance() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAnalyze_Empty(t *testing.T) {
	stats := Analyze(nil)
	if stats.TotalLines != 0 {
		t.Errorf("expected 0 lines, got %d", stats.TotalLines)
	}
	if stats.EstimatedTurnsLeft != -1 {
		t.Errorf("expected -1 turns left for empty, got %d", stats.EstimatedTurnsLeft)
	}
}
