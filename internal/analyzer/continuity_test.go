package analyzer

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestAnalyzeContinuity_CostAndIndex(t *testing.T) {
	s1 := ContinuitySessionInput{
		SessionID:   "aaaaaaaa-1111-1111-1111-111111111111",
		SessionSlug: "s1",
		Model:       "claude-sonnet-4-6",
		Entries: []jsonl.Entry{
			assistantReadEntry("tu-1", "/repo/a.go"),
			userToolResultEntry("tu-1", 400),
			userTextEntry(strings.Repeat("architecture constraints ", 8), 800),
		},
	}
	s2 := ContinuitySessionInput{
		SessionID:   "bbbbbbbb-2222-2222-2222-222222222222",
		SessionSlug: "s2",
		Model:       "claude-sonnet-4-6",
		Entries: []jsonl.Entry{
			assistantReadEntry("tu-2", "/repo/a.go"),
			userToolResultEntry("tu-2", 800),
			userTextEntry(strings.Repeat("architecture constraints ", 8), 1200),
		},
	}

	report := AnalyzeContinuity([]ContinuitySessionInput{s1, s2})
	if report == nil {
		t.Fatal("expected report")
	}
	if report.ContinuityIndex < 49.9 || report.ContinuityIndex > 50.1 {
		t.Fatalf("continuity index = %.2f, want ~50", report.ContinuityIndex)
	}
	if len(report.RepeatedFiles) != 1 {
		t.Fatalf("repeated files = %d, want 1", len(report.RepeatedFiles))
	}
	if len(report.RepeatedTexts) != 1 {
		t.Fatalf("repeated texts = %d, want 1", len(report.RepeatedTexts))
	}

	// Redundant file tokens = (400/4 + 800/4) - min(100,200) = 200.
	fileTokens := report.RepeatedFiles[0].EstimatedTokens
	if fileTokens != 200 {
		t.Fatalf("file tokens = %d, want 200", fileTokens)
	}
	fileCostWant := float64(fileTokens) / 1_000_000 * PricingForModel("claude-sonnet-4-6").CacheReadPerMillion
	if math.Abs(report.RepeatedFiles[0].EstimatedCost-fileCostWant) > 1e-9 {
		t.Fatalf("file cost = %.12f, want %.12f", report.RepeatedFiles[0].EstimatedCost, fileCostWant)
	}

	// Text cost uses input pricing and should dominate file cache-read cost here.
	if report.TotalTextCost <= report.TotalFileCost {
		t.Fatalf("expected text cost > file cost, got text %.8f file %.8f", report.TotalTextCost, report.TotalFileCost)
	}
}

func TestAnalyzeContinuity_TopicsAndSuggestions(t *testing.T) {
	var sessions []ContinuitySessionInput
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("session-%d-aaaaaaaa-bbbb-cccc-dddddddddddd", i)
		tuA := fmt.Sprintf("tu-a-%d", i)
		tuB := fmt.Sprintf("tu-b-%d", i)
		sessions = append(sessions, ContinuitySessionInput{
			SessionID:   id,
			SessionSlug: fmt.Sprintf("s%d", i),
			Model:       "claude-sonnet-4-6",
			Entries: []jsonl.Entry{
				assistantReadEntry(tuA, "/repo/a.go"),
				userToolResultEntry(tuA, 8000),
				assistantReadEntry(tuB, "/repo/b.go"),
				userToolResultEntry(tuB, 8000),
			},
		})
	}

	report := AnalyzeContinuity(sessions)
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.RepeatTopics) == 0 {
		t.Fatal("expected at least one repeat topic")
	}
	if len(report.Suggestions) < 2 {
		t.Fatalf("expected suggestions for both high-repeat files, got %d", len(report.Suggestions))
	}
}

func assistantReadEntry(toolID, path string) jsonl.Entry {
	content := fmt.Sprintf(`[{"type":"tool_use","id":"%s","name":"Read","input":{"file_path":"%s"}}]`, toolID, path)
	return jsonl.Entry{
		Type:    jsonl.TypeAssistant,
		RawSize: len(content),
		Message: &jsonl.Message{
			Content: []byte(content),
		},
	}
}

func userToolResultEntry(toolID string, rawSize int) jsonl.Entry {
	content := fmt.Sprintf(`[{"type":"tool_result","tool_use_id":"%s","content":"%s"}]`, toolID, strings.Repeat("x", 32))
	return jsonl.Entry{
		Type:    jsonl.TypeUser,
		RawSize: rawSize,
		Message: &jsonl.Message{
			Content: []byte(content),
		},
	}
}

func userTextEntry(text string, rawSize int) jsonl.Entry {
	content := fmt.Sprintf(`[{"type":"text","text":%q}]`, text)
	return jsonl.Entry{
		Type:    jsonl.TypeUser,
		RawSize: rawSize,
		Message: &jsonl.Message{
			Content: []byte(content),
		},
	}
}
