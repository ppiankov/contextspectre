package analyzer

import (
	"strings"
	"testing"
)

func TestAbbreviateModel(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"claude-opus-4-6", "O4.6 "},
		{"claude-opus-4-6-20260301", "O4.6 "},
		{"claude-opus-4-8", "O4.8 "},
		{"claude-sonnet-4-6", "S4.6 "},
		{"claude-sonnet-4-5", "S4.5 "},
		{"claude-haiku-4-5-20251001", "H4.5 "},
		{"claude-haiku-4-5", "H4.5 "},
		{"", "?    "},
		{"unknown-model", "?    "},
		{"<synthetic>", "?    "},
	}

	for _, tt := range tests {
		got := AbbreviateModel(tt.model)
		if got != tt.want {
			t.Errorf("AbbreviateModel(%q) = %q, want %q", tt.model, got, tt.want)
		}
		if len(got) != 5 {
			t.Errorf("AbbreviateModel(%q) len = %d, want 5", tt.model, len(got))
		}
	}
}

func TestAbbreviateModelMixedSuffix(t *testing.T) {
	abbr := AbbreviateModel("claude-sonnet-4-6")
	// Simulate mixed: trim space and append "*"
	mixed := strings.TrimRight(abbr, " ") + "*"
	for len(mixed) < 5 {
		mixed += " "
	}
	if len(mixed) != 5 {
		t.Errorf("mixed abbr len = %d, want 5", len(mixed))
	}
	if !strings.HasSuffix(strings.TrimRight(mixed, " "), "*") {
		t.Errorf("mixed abbr %q should end with *", mixed)
	}
}
