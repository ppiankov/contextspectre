package analyzer

import (
	"encoding/json"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func makeReadEntry(idx int, toolUseID, filePath string) jsonl.Entry {
	input, _ := json.Marshal(map[string]string{"file_path": filePath})
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: toolUseID, Name: "Read", Input: input},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeAssistant,
		Message: &jsonl.Message{Content: content},
		RawSize: 200,
	}
}

func makeToolResult(toolUseID string) jsonl.Entry {
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_result", ToolUseID: toolUseID, Content: json.RawMessage(`"file contents here"`)},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeUser,
		Message: &jsonl.Message{Content: content},
		RawSize: 800,
	}
}

func TestFindDuplicateReads_NoDuplicates(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadEntry(0, "t1", "/path/a.go"),
		makeToolResult("t1"),
		makeReadEntry(2, "t2", "/path/b.go"),
		makeToolResult("t2"),
	}
	result := FindDuplicateReads(entries)
	if len(result.Groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(result.Groups))
	}
	if result.TotalStale != 0 {
		t.Errorf("expected 0 stale, got %d", result.TotalStale)
	}
}

func TestFindDuplicateReads_BasicDuplicate(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadEntry(0, "t1", "/path/a.go"), // stale
		makeToolResult("t1"),                 // stale result
		makeReadEntry(2, "t2", "/path/a.go"), // latest
		makeToolResult("t2"),
	}
	result := FindDuplicateReads(entries)
	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	g := result.Groups[0]
	if g.FilePath != "/path/a.go" {
		t.Errorf("expected /path/a.go, got %s", g.FilePath)
	}
	if g.LatestIndex != 2 {
		t.Errorf("expected latest index 2, got %d", g.LatestIndex)
	}
	if len(g.StaleReads) != 1 {
		t.Fatalf("expected 1 stale read, got %d", len(g.StaleReads))
	}
	if g.StaleReads[0].AssistantIdx != 0 {
		t.Errorf("expected stale assistant idx 0, got %d", g.StaleReads[0].AssistantIdx)
	}
	if g.StaleReads[0].ToolUseID != "t1" {
		t.Errorf("expected tool_use_id t1, got %s", g.StaleReads[0].ToolUseID)
	}
	if g.StaleReads[0].ResultIdx != 1 {
		t.Errorf("expected result idx 1, got %d", g.StaleReads[0].ResultIdx)
	}
	if result.TotalStale != 1 {
		t.Errorf("expected 1 total stale, got %d", result.TotalStale)
	}
	if result.UniqueFiles != 1 {
		t.Errorf("expected 1 unique file, got %d", result.UniqueFiles)
	}
}

func TestFindDuplicateReads_MultipleFiles(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadEntry(0, "t1", "/path/a.go"),
		makeToolResult("t1"),
		makeReadEntry(2, "t2", "/path/b.go"),
		makeToolResult("t2"),
		makeReadEntry(4, "t3", "/path/a.go"),
		makeToolResult("t3"),
		makeReadEntry(6, "t4", "/path/b.go"),
		makeToolResult("t4"),
	}
	result := FindDuplicateReads(entries)
	if result.UniqueFiles != 2 {
		t.Errorf("expected 2 unique files, got %d", result.UniqueFiles)
	}
	if result.TotalStale != 2 {
		t.Errorf("expected 2 total stale, got %d", result.TotalStale)
	}
}

func TestFindDuplicateReads_TripleRead(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadEntry(0, "t1", "/path/a.go"), // stale
		makeToolResult("t1"),
		makeReadEntry(2, "t2", "/path/a.go"), // stale
		makeToolResult("t2"),
		makeReadEntry(4, "t3", "/path/a.go"), // latest
		makeToolResult("t3"),
	}
	result := FindDuplicateReads(entries)
	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	g := result.Groups[0]
	if len(g.StaleReads) != 2 {
		t.Errorf("expected 2 stale reads, got %d", len(g.StaleReads))
	}
	if g.LatestIndex != 4 {
		t.Errorf("expected latest index 4, got %d", g.LatestIndex)
	}
}

func TestFindDuplicateReads_DifferentToolNames(t *testing.T) {
	// View tool should also be detected
	input, _ := json.Marshal(map[string]string{"file_path": "/path/a.go"})
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: "t1", Name: "View", Input: input},
	})
	entries := []jsonl.Entry{
		{Type: jsonl.TypeAssistant, Message: &jsonl.Message{Content: content}, RawSize: 200},
		makeToolResult("t1"),
		makeReadEntry(2, "t2", "/path/a.go"),
		makeToolResult("t2"),
	}
	result := FindDuplicateReads(entries)
	if len(result.Groups) != 1 {
		t.Errorf("expected 1 group for mixed Read/View, got %d", len(result.Groups))
	}
}

func TestFindDuplicateReads_PathFieldFallback(t *testing.T) {
	// Some tools use "path" instead of "file_path"
	input, _ := json.Marshal(map[string]string{"path": "/path/a.go"})
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: "t1", Name: "Read", Input: input},
	})
	entries := []jsonl.Entry{
		{Type: jsonl.TypeAssistant, Message: &jsonl.Message{Content: content}, RawSize: 200},
		makeToolResult("t1"),
		makeReadEntry(2, "t2", "/path/a.go"),
		makeToolResult("t2"),
	}
	result := FindDuplicateReads(entries)
	if len(result.Groups) != 1 {
		t.Errorf("expected 1 group with path fallback, got %d", len(result.Groups))
	}
}

func TestFindDuplicateReads_MissingToolResult(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadEntry(0, "t1", "/path/a.go"), // stale, no result
		makeReadEntry(1, "t2", "/path/a.go"), // latest
		makeToolResult("t2"),
	}
	result := FindDuplicateReads(entries)
	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	g := result.Groups[0]
	if g.StaleReads[0].ResultIdx != -1 {
		t.Errorf("expected result idx -1 for missing result, got %d", g.StaleReads[0].ResultIdx)
	}
}

func TestFindDuplicateReads_NonReadToolIgnored(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"file_path": "/path/a.go"})
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: "t1", Name: "Write", Input: input},
	})
	entries := []jsonl.Entry{
		{Type: jsonl.TypeAssistant, Message: &jsonl.Message{Content: content}, RawSize: 200},
		makeToolResult("t1"),
		makeReadEntry(2, "t2", "/path/a.go"),
		makeToolResult("t2"),
	}
	result := FindDuplicateReads(entries)
	if len(result.Groups) != 0 {
		t.Errorf("expected 0 groups for Write tool, got %d", len(result.Groups))
	}
}

func TestDuplicateGroup_StaleIndices(t *testing.T) {
	g := DuplicateGroup{
		StaleReads: []StaleRead{
			{AssistantIdx: 0, ResultIdx: 1},
			{AssistantIdx: 4, ResultIdx: -1},
		},
	}
	indices := g.StaleIndices()
	if len(indices) != 3 {
		t.Fatalf("expected 3 stale indices, got %d", len(indices))
	}
	expected := map[int]bool{0: true, 1: true, 4: true}
	for _, idx := range indices {
		if !expected[idx] {
			t.Errorf("unexpected stale index %d", idx)
		}
	}
}

func TestDuplicateReadResult_AllStaleIndices(t *testing.T) {
	result := &DuplicateReadResult{
		Groups: []DuplicateGroup{
			{StaleReads: []StaleRead{{AssistantIdx: 0, ResultIdx: 1}}},
			{StaleReads: []StaleRead{{AssistantIdx: 4, ResultIdx: 5}}},
		},
	}
	all := result.AllStaleIndices()
	if len(all) != 4 {
		t.Errorf("expected 4 stale indices, got %d", len(all))
	}
}

func makeWriteEntry(filePath string) jsonl.Entry {
	input, _ := json.Marshal(map[string]string{"file_path": filePath})
	content, _ := json.Marshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: "w1", Name: "Edit", Input: input},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeAssistant,
		Message: &jsonl.Message{Content: content},
		RawSize: 200,
	}
}

func TestFindDuplicateReads_ReadWriteRead_NotStale(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadEntry(0, "t1", "/path/a.go"),
		makeToolResult("t1"),
		makeWriteEntry("/path/a.go"), // write invalidates
		makeToolResult("w1"),
		makeReadEntry(4, "t2", "/path/a.go"), // fresh — file was modified
		makeToolResult("t2"),
	}
	result := FindDuplicateReads(entries)
	if result.TotalStale != 0 {
		t.Errorf("expected 0 stale after write, got %d", result.TotalStale)
	}
}

func TestFindDuplicateReads_ReadReadWriteReadRead(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadEntry(0, "t1", "/path/a.go"), // stale (group 1)
		makeToolResult("t1"),
		makeReadEntry(2, "t2", "/path/a.go"), // fresh at write time
		makeToolResult("t2"),
		makeWriteEntry("/path/a.go"), // write resets
		makeToolResult("w1"),
		makeReadEntry(6, "t3", "/path/a.go"), // stale (group 2)
		makeToolResult("t3"),
		makeReadEntry(8, "t4", "/path/a.go"), // fresh
		makeToolResult("t4"),
	}
	result := FindDuplicateReads(entries)
	if result.TotalStale != 2 {
		t.Errorf("expected 2 stale reads, got %d", result.TotalStale)
	}
}

func TestFindDuplicateReads_PathNormalization(t *testing.T) {
	entries := []jsonl.Entry{
		makeReadEntry(0, "t1", "/a/../b.go"), // stale — same as /b.go
		makeToolResult("t1"),
		makeReadEntry(2, "t2", "/b.go"), // fresh
		makeToolResult("t2"),
	}
	result := FindDuplicateReads(entries)
	if result.TotalStale != 1 {
		t.Errorf("expected 1 stale (normalized paths), got %d", result.TotalStale)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(result.Groups))
	}
	if result.Groups[0].FilePath != "/b.go" {
		t.Errorf("expected normalized path /b.go, got %s", result.Groups[0].FilePath)
	}
}

func TestIsFileReadTool(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Read", true},
		{"View", true},
		{"read_file", true},
		{"ReadFile", true},
		{"Write", false},
		{"Edit", false},
		{"Bash", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isFileReadTool(tt.name)
		if got != tt.want {
			t.Errorf("isFileReadTool(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestExtractFilePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"file_path field", `{"file_path":"/a/b.go"}`, "/a/b.go"},
		{"path field", `{"path":"/a/b.go"}`, "/a/b.go"},
		{"file_path preferred", `{"file_path":"/a.go","path":"/b.go"}`, "/a.go"},
		{"empty", `{}`, ""},
		{"invalid json", `invalid`, ""},
	}
	for _, tt := range tests {
		got := extractFilePath(json.RawMessage(tt.input))
		if got != tt.want {
			t.Errorf("extractFilePath(%s) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
