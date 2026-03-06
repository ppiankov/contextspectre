package analyzer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestDiagnose_NoIssues(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", Message: &jsonl.Message{Content: json.RawMessage(`"hello"`)}},
		{Type: jsonl.TypeAssistant, UUID: "a1", ParentUUID: "u1", Message: &jsonl.Message{Content: json.RawMessage(`"world"`)}},
	}
	result := Diagnose(entries)
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(result.Issues))
	}
}

func TestDiagnose_FilterBlock(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", Message: &jsonl.Message{Content: json.RawMessage(`"please help"`)}},
		{Type: jsonl.TypeAssistant, UUID: "a1", ParentUUID: "u1", Message: &jsonl.Message{Content: json.RawMessage(`""`)}},
	}
	result := Diagnose(entries)
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}
	if result.Issues[0].Kind != IssueFilterBlock {
		t.Errorf("expected filter_block, got %s", result.Issues[0].Kind)
	}
	if result.Issues[0].EntryIndex != 1 {
		t.Errorf("expected entry index 1, got %d", result.Issues[0].EntryIndex)
	}
	if result.Issues[0].RelatedIndex != 0 {
		t.Errorf("expected related index 0, got %d", result.Issues[0].RelatedIndex)
	}
}

func TestDiagnose_EmptyArray(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", Message: &jsonl.Message{Content: json.RawMessage(`"test"`)}},
		{Type: jsonl.TypeAssistant, UUID: "a1", ParentUUID: "u1", Message: &jsonl.Message{Content: json.RawMessage(`[]`)}},
	}
	result := Diagnose(entries)
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}
	if result.Issues[0].Kind != IssueFilterBlock {
		t.Errorf("expected filter_block, got %s", result.Issues[0].Kind)
	}
}

func TestDiagnose_OversizedImage(t *testing.T) {
	// Create image data larger than 5MB threshold
	imgData := strings.Repeat("A", OversizedImageThreshold+1000)
	entries := []jsonl.Entry{
		{
			Type: jsonl.TypeUser, UUID: "u1",
			Message: &jsonl.Message{
				Content: json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + imgData + `"}}]`),
			},
		},
	}
	result := Diagnose(entries)
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}
	if result.Issues[0].Kind != IssueOversizedImage {
		t.Errorf("expected oversized_image, got %s", result.Issues[0].Kind)
	}
}

func TestDiagnose_NormalImage(t *testing.T) {
	// Image under threshold
	imgData := strings.Repeat("A", 1000)
	entries := []jsonl.Entry{
		{
			Type: jsonl.TypeUser, UUID: "u1",
			Message: &jsonl.Message{
				Content: json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + imgData + `"}}]`),
			},
		},
	}
	result := Diagnose(entries)
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues for normal-sized image, got %d", len(result.Issues))
	}
}

func TestDiagnose_OrphanedToolResult(t *testing.T) {
	entries := []jsonl.Entry{
		{
			Type: jsonl.TypeUser, UUID: "u0",
			Message: &jsonl.Message{Content: json.RawMessage(`"hello"`)},
		},
		{
			Type: jsonl.TypeAssistant, UUID: "a1", ParentUUID: "u0",
			Message: &jsonl.Message{
				Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_existing","name":"Read","input":{}}]`),
			},
		},
		{
			Type: jsonl.TypeUser, UUID: "u1", ParentUUID: "a1",
			Message: &jsonl.Message{
				Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_missing","content":"output"}]`),
			},
		},
	}
	result := Diagnose(entries)
	// Expect 2 issues: orphaned_result from content scan + chain_broken from integrity check
	hasOrphan := false
	for _, issue := range result.Issues {
		if issue.Kind == IssueOrphanedResult {
			hasOrphan = true
		}
	}
	if !hasOrphan {
		t.Errorf("expected orphaned_result issue, got kinds: %v", result.Issues)
	}
}

func TestDiagnose_ValidToolResult(t *testing.T) {
	entries := []jsonl.Entry{
		{
			Type: jsonl.TypeUser, UUID: "u0",
			Message: &jsonl.Message{Content: json.RawMessage(`"hello"`)},
		},
		{
			Type: jsonl.TypeAssistant, UUID: "a1", ParentUUID: "u0",
			Message: &jsonl.Message{
				Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_1","name":"Read","input":{}}]`),
			},
		},
		{
			Type: jsonl.TypeUser, UUID: "u1", ParentUUID: "a1",
			Message: &jsonl.Message{
				Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"output"}]`),
			},
		},
	}
	result := Diagnose(entries)
	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues for valid tool result, got %d", len(result.Issues))
	}
}

func TestDiagnosisResult_IssuesByIndex(t *testing.T) {
	result := &DiagnosisResult{
		Issues: []Issue{
			{Kind: IssueFilterBlock, EntryIndex: 1, RelatedIndex: 0},
			{Kind: IssueOversizedImage, EntryIndex: 3, RelatedIndex: 0},
		},
	}
	byIndex := result.IssuesByIndex()
	if len(byIndex[1]) != 1 {
		t.Errorf("expected 1 issue at index 1, got %d", len(byIndex[1]))
	}
	// Filter block's related index (0) should also have an entry
	if len(byIndex[0]) != 2 {
		t.Errorf("expected 2 issues at index 0 (related), got %d", len(byIndex[0]))
	}
}
