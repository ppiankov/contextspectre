package editor

import (
	"testing"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestRemoveFailedRetries_Basic(t *testing.T) {
	path := copyFixture(t, "failed_retry.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	retryResult := analyzer.FindFailedRetries(entries)
	if len(retryResult.Sequences) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(retryResult.Sequences))
	}

	result, err := RemoveFailedRetries(path, retryResult)
	if err != nil {
		t.Fatalf("remove retries: %v", err)
	}

	if result.FailedRemoved != 1 {
		t.Errorf("expected 1 failed removed, got %d", result.FailedRemoved)
	}
	if result.BytesAfter >= result.BytesBefore {
		t.Error("expected bytes after < bytes before")
	}

	// Verify the failed attempt is gone, retry is preserved
	after, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	for _, e := range after {
		if e.Message == nil {
			continue
		}
		blocks, _ := jsonl.ParseContentBlocks(e.Message.Content)
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID == "toolu_f1" {
				t.Error("failed tool_use toolu_f1 should have been removed")
			}
			if b.Type == "tool_result" && b.ToolUseID == "toolu_f1" {
				t.Error("failed tool_result for toolu_f1 should have been removed")
			}
		}
	}

	// Retry should still exist
	foundRetry := false
	for _, e := range after {
		if e.Message == nil {
			continue
		}
		blocks, _ := jsonl.ParseContentBlocks(e.Message.Content)
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID == "toolu_f2" {
				foundRetry = true
			}
		}
	}
	if !foundRetry {
		t.Error("retry tool_use toolu_f2 should be preserved")
	}
}

func TestRemoveFailedRetries_NilResult(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	result, err := RemoveFailedRetries(path, nil)
	if err != nil {
		t.Fatalf("remove retries: %v", err)
	}
	if result.FailedRemoved != 0 {
		t.Errorf("expected 0, got %d", result.FailedRemoved)
	}
}

func TestRemoveFailedRetries_NoFailures(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	retryResult := analyzer.FindFailedRetries(entries)
	result, err := RemoveFailedRetries(path, retryResult)
	if err != nil {
		t.Fatalf("remove retries: %v", err)
	}
	if result.FailedRemoved != 0 {
		t.Errorf("expected 0, got %d", result.FailedRemoved)
	}
}

func TestRemoveFailedRetries_ChainRepair(t *testing.T) {
	path := copyFixture(t, "failed_retry.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	retryResult := analyzer.FindFailedRetries(entries)
	_, err = RemoveFailedRetries(path, retryResult)
	if err != nil {
		t.Fatalf("remove retries: %v", err)
	}

	// Check all parentUuid references point to existing entries
	after, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	uuids := make(map[string]bool)
	for _, e := range after {
		if e.UUID != "" {
			uuids[e.UUID] = true
		}
	}
	for _, e := range after {
		if e.ParentUUID != "" && !uuids[e.ParentUUID] {
			t.Errorf("orphaned parentUuid: entry %s points to missing %s", e.UUID, e.ParentUUID)
		}
	}
}
