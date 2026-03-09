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

func makeBashToolUseCmd(id, command string) jsonl.Entry {
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: id, Name: "Bash", Input: json.RawMessage(`{"command":"` + command + `"}`)},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeAssistant,
		Message: &jsonl.Message{Content: content},
		RawSize: 200,
	}
}

func makeReadToolUse(id, filePath string) jsonl.Entry {
	input, _ := json.Marshal(map[string]string{"file_path": filePath})
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: id, Name: "Read", Input: input},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeAssistant,
		Message: &jsonl.Message{Content: content},
		RawSize: 200,
	}
}

func TestFindFailedRetries_DifferentBashCommand_NotRetry(t *testing.T) {
	entries := []jsonl.Entry{
		makeBashToolUseCmd("t1", "ls /tmp"), // 0: failed
		makeErrorResult("t1"),               // 1: error
		makeBashToolUseCmd("t2", "go test"), // 2: different command — not a retry
		makeSuccessResult("t2"),             // 3
	}
	result := FindFailedRetries(entries)
	if len(result.Sequences) != 0 {
		t.Errorf("expected 0 sequences for different Bash commands, got %d", len(result.Sequences))
	}
}

func TestFindFailedRetries_SameBashCommand_IsRetry(t *testing.T) {
	entries := []jsonl.Entry{
		makeBashToolUseCmd("t1", "go test ./..."), // 0: failed
		makeErrorResult("t1"),                     // 1: error
		makeBashToolUseCmd("t2", "go test ./..."), // 2: same command — retry
		makeSuccessResult("t2"),                   // 3
	}
	result := FindFailedRetries(entries)
	if len(result.Sequences) != 1 {
		t.Fatalf("expected 1 sequence for same Bash command, got %d", len(result.Sequences))
	}
	if result.Sequences[0].ToolName != "Bash" {
		t.Errorf("expected Bash, got %s", result.Sequences[0].ToolName)
	}
}

func TestFindFailedRetries_SameReadDifferentFile_NotRetry(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadToolUse("t1", "/path/a.go"), // 0: failed read
		makeErrorResult("t1"),               // 1: error
		makeReadToolUse("t2", "/path/b.go"), // 2: different file — not a retry
		makeSuccessResult("t2"),             // 3
	}
	result := FindFailedRetries(entries)
	if len(result.Sequences) != 0 {
		t.Errorf("expected 0 sequences for Read of different files, got %d", len(result.Sequences))
	}
}

func TestFindFailedRetries_SameReadSameFile_IsRetry(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadToolUse("t1", "/path/a.go"), // 0: failed
		makeErrorResult("t1"),               // 1: error
		makeReadToolUse("t2", "/path/a.go"), // 2: same file — retry
		makeSuccessResult("t2"),             // 3
	}
	result := FindFailedRetries(entries)
	if len(result.Sequences) != 1 {
		t.Fatalf("expected 1 sequence for same Read file, got %d", len(result.Sequences))
	}
	if result.Sequences[0].ToolName != "Read" {
		t.Errorf("expected Read, got %s", result.Sequences[0].ToolName)
	}
}

func TestRetrySignature(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input string
		want  string
	}{
		{"read file", "Read", `{"file_path":"/a/b.go"}`, "Read:/a/b.go"},
		{"read with dots", "Read", `{"file_path":"/a/../b.go"}`, "Read:/b.go"},
		{"bash command", "Bash", `{"command":"go test ./..."}`, "Bash:go test ./..."},
		{"grep pattern", "Grep", `{"pattern":"foo","path":"/src"}`, "Grep:foo:/src"},
		{"unknown tool", "Agent", `{"prompt":"do stuff"}`, "Agent"},
		{"write file", "Edit", `{"file_path":"/a/b.go"}`, "Edit:/a/b.go"},
	}
	for _, tt := range tests {
		got := retrySignature(tt.tool, json.RawMessage(tt.input))
		if got != tt.want {
			t.Errorf("retrySignature(%s) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestIsGrepLikeTool(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Grep", true},
		{"Glob", true},
		{"grep", true},
		{"search", true},
		{"Read", false},
		{"Bash", false},
	}
	for _, tt := range tests {
		got := isGrepLikeTool(tt.name)
		if got != tt.want {
			t.Errorf("isGrepLikeTool(%q) = %v, want %v", tt.name, got, tt.want)
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
