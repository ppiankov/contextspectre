package analyzer

import (
	"encoding/json"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func makeBashToolUse(id string) jsonl.Entry {
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: id, Name: "Bash", Input: json.RawMessage(`{"command":"test"}`)},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeAssistant,
		Message: &jsonl.Message{Content: content},
		RawSize: 200,
	}
}

func makeErrorResult(toolUseID string) jsonl.Entry {
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_result", ToolUseID: toolUseID, IsError: true, Content: json.RawMessage(`"Error: command failed"`)},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeUser,
		Message: &jsonl.Message{Content: content},
		RawSize: 400,
	}
}

func makeSuccessResult(toolUseID string) jsonl.Entry {
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_result", ToolUseID: toolUseID, Content: json.RawMessage(`"success"`)},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeUser,
		Message: &jsonl.Message{Content: content},
		RawSize: 200,
	}
}

func TestFindFailedRetries_Basic(t *testing.T) {
	entries := []jsonl.Entry{
		makeBashToolUse("t1"),   // 0: failed attempt
		makeErrorResult("t1"),   // 1: error result
		makeBashToolUse("t2"),   // 2: retry
		makeSuccessResult("t2"), // 3: success
	}
	result := FindFailedRetries(entries)
	if len(result.Sequences) != 1 {
		t.Fatalf("expected 1 sequence, got %d", len(result.Sequences))
	}
	s := result.Sequences[0]
	if s.FailedToolUseIdx != 0 {
		t.Errorf("expected failed idx 0, got %d", s.FailedToolUseIdx)
	}
	if s.FailedResultIdx != 1 {
		t.Errorf("expected result idx 1, got %d", s.FailedResultIdx)
	}
	if s.RetryToolUseIdx != 2 {
		t.Errorf("expected retry idx 2, got %d", s.RetryToolUseIdx)
	}
	if s.ToolName != "Bash" {
		t.Errorf("expected Bash, got %s", s.ToolName)
	}
}

func TestFindFailedRetries_NoRetry(t *testing.T) {
	// Error with no subsequent retry — should NOT be flagged
	entries := []jsonl.Entry{
		makeBashToolUse("t1"),
		makeErrorResult("t1"),
		// No retry follows — user changed approach
	}
	result := FindFailedRetries(entries)
	if len(result.Sequences) != 0 {
		t.Errorf("expected 0 sequences when no retry, got %d", len(result.Sequences))
	}
}

func TestFindFailedRetries_NoError(t *testing.T) {
	// Successful result — should NOT be flagged
	entries := []jsonl.Entry{
		makeBashToolUse("t1"),
		makeSuccessResult("t1"),
		makeBashToolUse("t2"),
		makeSuccessResult("t2"),
	}
	result := FindFailedRetries(entries)
	if len(result.Sequences) != 0 {
		t.Errorf("expected 0 sequences for successful runs, got %d", len(result.Sequences))
	}
}

func TestFindFailedRetries_ErrorByContent(t *testing.T) {
	// Error detected by content, not is_error flag
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_result", ToolUseID: "t1", Content: json.RawMessage(`"Permission denied: /etc/shadow"`)},
	})
	entries := []jsonl.Entry{
		makeBashToolUse("t1"),
		{Type: jsonl.TypeUser, Message: &jsonl.Message{Content: content}, RawSize: 300},
		makeBashToolUse("t2"),
		makeSuccessResult("t2"),
	}
	result := FindFailedRetries(entries)
	if len(result.Sequences) != 1 {
		t.Errorf("expected 1 sequence for content-detected error, got %d", len(result.Sequences))
	}
}

func TestFindFailedRetries_DifferentTool(t *testing.T) {
	// Error in Bash, but retry is Read — different tool, not a retry
	readContent, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: "t2", Name: "Read", Input: json.RawMessage(`{"file_path":"/tmp/test"}`)},
	})
	entries := []jsonl.Entry{
		makeBashToolUse("t1"),
		makeErrorResult("t1"),
		{Type: jsonl.TypeAssistant, Message: &jsonl.Message{Content: readContent}, RawSize: 200},
		makeSuccessResult("t2"),
	}
	result := FindFailedRetries(entries)
	if len(result.Sequences) != 0 {
		t.Errorf("expected 0 sequences for different tool retry, got %d", len(result.Sequences))
	}
}

func TestRetryResult_AllFailedIndices(t *testing.T) {
	result := &RetryResult{
		Sequences: []RetrySequence{
			{FailedToolUseIdx: 0, FailedResultIdx: 1},
			{FailedToolUseIdx: 4, FailedResultIdx: 5},
		},
	}
	indices := result.AllFailedIndices()
	if len(indices) != 4 {
		t.Errorf("expected 4 indices, got %d", len(indices))
	}
	for _, idx := range []int{0, 1, 4, 5} {
		if !indices[idx] {
			t.Errorf("expected index %d in failed set", idx)
		}
	}
}

func TestContainsErrorIndicator(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"Error: file not found", true},
		{"Permission denied: /root", true},
		{"No such file or directory", true},
		{"exit status 1", true},
		{"fatal: not a git repository", true},
		{"panic: runtime error", true},
		{"success", false},
		{"PASS ok", false},
		{"", false},
	}
	for _, tt := range tests {
		got := containsErrorIndicator(tt.text)
		if got != tt.want {
			t.Errorf("containsErrorIndicator(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}
