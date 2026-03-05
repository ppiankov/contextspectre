package analyzer

import (
	"encoding/json"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestDetectSidechains_MissingParentRepairable(t *testing.T) {
	entries := []jsonl.Entry{
		{
			Type:       jsonl.TypeUser,
			UUID:       "u1",
			LineNumber: 1,
			RawSize:    400,
			Message:    &jsonl.Message{Content: mustRaw(t, []jsonl.ContentBlock{{Type: "text", Text: "start"}})},
		},
		{
			Type:       jsonl.TypeAssistant,
			UUID:       "a1",
			ParentUUID: "u1",
			LineNumber: 2,
			RawSize:    500,
			Message:    &jsonl.Message{Content: mustRaw(t, []jsonl.ContentBlock{{Type: "text", Text: "ok"}})},
		},
		{
			Type:       jsonl.TypeUser,
			UUID:       "u2",
			ParentUUID: "missing-parent",
			LineNumber: 3,
			RawSize:    600,
			Message:    &jsonl.Message{Content: mustRaw(t, []jsonl.ContentBlock{{Type: "text", Text: "broken chain"}})},
		},
	}

	report := DetectSidechains(entries)
	if report.TotalEntries != 1 {
		t.Fatalf("expected 1 sidechain entry, got %d", report.TotalEntries)
	}
	sc := report.Entries[0]
	if sc.EntryIndex != 2 {
		t.Fatalf("expected entry index 2, got %d", sc.EntryIndex)
	}
	if sc.Classification != "repairable" {
		t.Fatalf("expected repairable classification, got %q", sc.Classification)
	}
	if sc.ReconnectParent != "a1" {
		t.Fatalf("expected reconnect parent a1, got %q", sc.ReconnectParent)
	}
	if len(sc.Reasons) != 1 || sc.Reasons[0] != SidechainMissingParent {
		t.Fatalf("expected reason missing_parent, got %#v", sc.Reasons)
	}
}

func TestDetectSidechains_OrphanedToolResultPruneOnly(t *testing.T) {
	entries := []jsonl.Entry{
		{
			Type:       jsonl.TypeUser,
			UUID:       "u1",
			LineNumber: 1,
			RawSize:    400,
			Message:    &jsonl.Message{Content: mustRaw(t, []jsonl.ContentBlock{{Type: "text", Text: "start"}})},
		},
		{
			Type:       jsonl.TypeUser,
			UUID:       "u2",
			ParentUUID: "u1",
			LineNumber: 2,
			RawSize:    700,
			Message: &jsonl.Message{
				Content: mustRaw(t, []jsonl.ContentBlock{
					{
						Type:      "tool_result",
						ToolUseID: "missing-tool-use",
						Content:   mustJSON(t, "error"),
					},
				}),
			},
		},
	}

	report := DetectSidechains(entries)
	if report.TotalEntries != 1 {
		t.Fatalf("expected 1 sidechain entry, got %d", report.TotalEntries)
	}
	sc := report.Entries[0]
	if sc.Classification != "prune-only" {
		t.Fatalf("expected prune-only classification, got %q", sc.Classification)
	}
	if sc.ToolUseID != "missing-tool-use" {
		t.Fatalf("expected tool use id missing-tool-use, got %q", sc.ToolUseID)
	}
	if len(sc.Reasons) != 1 || sc.Reasons[0] != SidechainOrphanResult {
		t.Fatalf("expected reason orphaned_tool_result, got %#v", sc.Reasons)
	}
}

func TestDetectSidechains_FlaggedAndGrouping(t *testing.T) {
	entries := []jsonl.Entry{
		{
			Type:       jsonl.TypeUser,
			UUID:       "u1",
			LineNumber: 1,
			RawSize:    200,
			Message:    &jsonl.Message{Content: mustRaw(t, []jsonl.ContentBlock{{Type: "text", Text: "start"}})},
		},
		{
			Type:        jsonl.TypeProgress,
			UUID:        "p1",
			ParentUUID:  "u1",
			IsSidechain: true,
			LineNumber:  2,
			RawSize:     200,
		},
		{
			Type:        jsonl.TypeProgress,
			UUID:        "p2",
			ParentUUID:  "p1",
			IsSidechain: true,
			LineNumber:  3,
			RawSize:     200,
		},
		{
			Type:       jsonl.TypeAssistant,
			UUID:       "a2",
			ParentUUID: "u1",
			LineNumber: 4,
			RawSize:    300,
			Message:    &jsonl.Message{Content: mustRaw(t, []jsonl.ContentBlock{{Type: "text", Text: "normal"}})},
		},
		{
			Type:        jsonl.TypeProgress,
			UUID:        "p3",
			ParentUUID:  "a2",
			IsSidechain: true,
			LineNumber:  6,
			RawSize:     200,
		},
	}

	report := DetectSidechains(entries)
	if report.TotalEntries != 3 {
		t.Fatalf("expected 3 sidechain entries, got %d", report.TotalEntries)
	}
	if report.GroupCount != 2 {
		t.Fatalf("expected 2 sidechain groups, got %d", report.GroupCount)
	}
	if report.TotalTokens <= 0 {
		t.Fatal("expected positive token count")
	}

	indexSet := SidechainIndexSet(report)
	if len(indexSet) != 3 || !indexSet[1] || !indexSet[2] || !indexSet[4] {
		t.Fatalf("unexpected sidechain index set: %#v", indexSet)
	}
}

func mustRaw(t *testing.T, blocks []jsonl.ContentBlock) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal raw blocks: %v", err)
	}
	return b
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return b
}
