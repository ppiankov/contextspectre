package editor

import (
	"strings"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestTruncateText_Basic(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line " + strings.Repeat("x", 50)
	}
	text := strings.Join(lines, "\n")

	truncated, removed := truncateText(text, 5)
	if removed != 20 {
		t.Errorf("expected 20 lines removed, got %d", removed)
	}
	if !strings.Contains(truncated, "[... 20 lines truncated by contextspectre ...]") {
		t.Error("expected truncation marker")
	}
	// Should have 5 head + marker + 5 tail = 11 lines
	resultLines := strings.Split(truncated, "\n")
	if len(resultLines) != 11 {
		t.Errorf("expected 11 lines, got %d", len(resultLines))
	}
}

func TestTruncateText_TooShort(t *testing.T) {
	text := "line1\nline2\nline3"
	truncated, removed := truncateText(text, 5)
	if removed != 0 {
		t.Errorf("expected 0 lines removed for short text, got %d", removed)
	}
	if truncated != text {
		t.Error("short text should be unchanged")
	}
}

func TestTruncateText_ExactThreshold(t *testing.T) {
	// 21 lines with keepLines=10: exactly 21 > 20+1, so 1 line removed
	lines := make([]string, 22)
	for i := range lines {
		lines[i] = "line"
	}
	text := strings.Join(lines, "\n")
	_, removed := truncateText(text, 10)
	if removed != 2 {
		t.Errorf("expected 2 lines removed, got %d", removed)
	}
}

func TestTruncateOutputs_Basic(t *testing.T) {
	path := copyFixture(t, "large_output.jsonl")

	result, err := TruncateOutputs(path, 4096, 5)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if result.OutputsTruncated != 1 {
		t.Errorf("expected 1 output truncated, got %d", result.OutputsTruncated)
	}
	if result.TokensSaved <= 0 {
		t.Error("expected positive tokens saved")
	}
	if result.BytesAfter >= result.BytesBefore {
		t.Error("expected bytes after < bytes before")
	}

	// Verify the truncated content has the marker
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	for _, e := range entries {
		if e.Message == nil {
			continue
		}
		blocks, _ := jsonl.ParseContentBlocks(e.Message.Content)
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID == "toolu_b1" {
				text, _ := extractToolResultText(b.Content)
				if !strings.Contains(text, "truncated by contextspectre") {
					t.Error("expected truncation marker in output")
				}
			}
		}
	}
}

func TestTruncateOutputs_SmallOutputUntouched(t *testing.T) {
	path := copyFixture(t, "large_output.jsonl")

	result, err := TruncateOutputs(path, 4096, 5)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Only 1 should be truncated (the large one), not the small "hi" output
	if result.OutputsTruncated != 1 {
		t.Errorf("expected 1 truncated, got %d", result.OutputsTruncated)
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	for _, e := range entries {
		if e.Message == nil {
			continue
		}
		blocks, _ := jsonl.ParseContentBlocks(e.Message.Content)
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID == "toolu_b2" {
				text, _ := extractToolResultText(b.Content)
				if text != "hi" {
					t.Errorf("small output should be unchanged, got %q", text)
				}
			}
		}
	}
}

func TestTruncateOutputs_NoLargeOutputs(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	result, err := TruncateOutputs(path, 4096, 10)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if result.OutputsTruncated != 0 {
		t.Errorf("expected 0 truncated, got %d", result.OutputsTruncated)
	}
}

func TestIsBashTool(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Bash", true},
		{"bash", true},
		{"execute_command", true},
		{"run_command", true},
		{"Read", false},
		{"Write", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isBashTool(tt.name)
		if got != tt.want {
			t.Errorf("isBashTool(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestExtractToolResultText_String(t *testing.T) {
	text, isString := extractToolResultText([]byte(`"hello world"`))
	if text != "hello world" {
		t.Errorf("expected 'hello world', got %q", text)
	}
	if !isString {
		t.Error("expected isString=true")
	}
}

func TestExtractToolResultText_Array(t *testing.T) {
	text, isString := extractToolResultText([]byte(`[{"type":"text","text":"hello"}]`))
	if text != "hello" {
		t.Errorf("expected 'hello', got %q", text)
	}
	if isString {
		t.Error("expected isString=false for array content")
	}
}

func TestExtractToolResultText_Nil(t *testing.T) {
	text, _ := extractToolResultText(nil)
	if text != "" {
		t.Errorf("expected empty, got %q", text)
	}
}
