package editor

import (
	"strings"
	"testing"
)

func TestIsSeparatorLine_BoxDrawing(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{strings.Repeat("─", 20), true},
		{strings.Repeat("━", 30), true},
		{strings.Repeat("═", 25), true},
		{"  " + strings.Repeat("─", 40) + "  ", true},
		{strings.Repeat("─", 10), false},                // too short
		{"some text " + strings.Repeat("─", 20), false}, // mixed content
		{"", false},
		{"   ", false}, // blank line
	}
	for _, tt := range tests {
		got := isSeparatorLine(tt.line)
		if got != tt.want {
			t.Errorf("isSeparatorLine(%q) = %v, want %v", tt.line[:min(len(tt.line), 40)], got, tt.want)
		}
	}
}

func TestIsSeparatorLine_ASCII(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{strings.Repeat("-", 40), true},
		{strings.Repeat("=", 40), true},
		{strings.Repeat("~", 40), true},
		{strings.Repeat("_", 40), true},
		{strings.Repeat("-", 39), false}, // too short for ASCII (needs 40+)
		{"func main() {", false},
	}
	for _, tt := range tests {
		got := isSeparatorLine(tt.line)
		if got != tt.want {
			t.Errorf("isSeparatorLine(%q) = %v, want %v", tt.line[:min(len(tt.line), 40)], got, tt.want)
		}
	}
}

func TestStripSeparatorLines(t *testing.T) {
	text := "Hello world\n" +
		strings.Repeat("─", 40) + "\n" +
		"Some content\n" +
		strings.Repeat("-", 50) + "\n" +
		"More content"

	cleaned, removed, saved := stripSeparatorLines(text)
	if removed != 2 {
		t.Errorf("expected 2 lines removed, got %d", removed)
	}
	// 40 box-drawing chars (3 bytes each = 120) + 50 ASCII dashes = 170 bytes
	if saved != 170 {
		t.Errorf("expected 170 chars saved, got %d", saved)
	}
	if strings.Contains(cleaned, strings.Repeat("─", 40)) {
		t.Error("expected box-drawing separator to be removed")
	}
	if !strings.Contains(cleaned, "Hello world") {
		t.Error("expected 'Hello world' to be preserved")
	}
	if !strings.Contains(cleaned, "Some content") {
		t.Error("expected 'Some content' to be preserved")
	}
}

func TestStripSeparatorLines_NoSeparators(t *testing.T) {
	text := "Hello world\nSome code\nMore text"
	cleaned, removed, saved := stripSeparatorLines(text)
	if removed != 0 {
		t.Errorf("expected 0 lines removed, got %d", removed)
	}
	if saved != 0 {
		t.Errorf("expected 0 chars saved, got %d", saved)
	}
	if cleaned != text {
		t.Error("expected text unchanged")
	}
}

func TestStripSeparatorLines_PreservesBlankLines(t *testing.T) {
	text := "Hello\n\nWorld"
	cleaned, removed, _ := stripSeparatorLines(text)
	if removed != 0 {
		t.Errorf("expected 0 lines removed, got %d", removed)
	}
	if cleaned != text {
		t.Error("expected blank lines preserved")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
