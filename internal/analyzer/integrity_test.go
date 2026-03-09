package analyzer

import (
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestCheckIntegrity_Healthy(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1"},
		{Type: jsonl.TypeAssistant, UUID: "a1", ParentUUID: "u1"},
	}
	report := CheckIntegrity(entries)
	if !report.Healthy {
		t.Error("expected healthy chain")
	}
	if report.ActiveChainLen != 2 {
		t.Errorf("ActiveChainLen = %d, want 2", report.ActiveChainLen)
	}
}

func TestCheckIntegrity_BadChainStart_Single(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeAssistant, UUID: "a1"},
	}
	report := CheckIntegrity(entries)
	if report.Healthy {
		t.Error("expected unhealthy")
	}
	if len(report.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(report.Issues))
	}
	if report.Issues[0].Kind != IntegrityBadChainStart {
		t.Errorf("expected bad_chain_start, got %s", report.Issues[0].Kind)
	}
}

func TestCheckIntegrity_BadChainStart_Consecutive(t *testing.T) {
	// Multiple consecutive assistant entries at chain start — all should be reported.
	entries := []jsonl.Entry{
		{Type: jsonl.TypeAssistant, UUID: "a1"},
		{Type: jsonl.TypeAssistant, UUID: "a2", ParentUUID: "a1"},
		{Type: jsonl.TypeAssistant, UUID: "a3", ParentUUID: "a2"},
		{Type: jsonl.TypeUser, UUID: "u1", ParentUUID: "a3"},
		{Type: jsonl.TypeAssistant, UUID: "a4", ParentUUID: "u1"},
	}
	report := CheckIntegrity(entries)
	if report.Healthy {
		t.Error("expected unhealthy")
	}
	// Should report 3 issues (one per consecutive assistant at start)
	chainStartIssues := 0
	for _, issue := range report.Issues {
		if issue.Kind == IntegrityBadChainStart {
			chainStartIssues++
		}
	}
	if chainStartIssues != 3 {
		t.Errorf("expected 3 bad_chain_start issues, got %d", chainStartIssues)
	}
}

func TestCheckIntegrity_MissingParent(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1"},
		{Type: jsonl.TypeAssistant, UUID: "a1", ParentUUID: "missing"},
	}
	report := CheckIntegrity(entries)
	if report.Healthy {
		t.Error("expected unhealthy")
	}
	if report.Issues[0].Kind != IntegrityMissingParent {
		t.Errorf("expected missing_parent, got %s", report.Issues[0].Kind)
	}
}
