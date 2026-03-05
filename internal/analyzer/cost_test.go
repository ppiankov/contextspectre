package analyzer

import "testing"

func TestCacheReadCostForTokens(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		tokens int
		want   float64
	}{
		{
			name:   "opus million tokens",
			model:  "claude-opus-4-6",
			tokens: 1_000_000,
			want:   0.75,
		},
		{
			name:   "sonnet million tokens",
			model:  "claude-sonnet-4-6",
			tokens: 1_000_000,
			want:   0.15,
		},
		{
			name:   "unknown model falls back to default",
			model:  "unknown-model",
			tokens: 1_000_000,
			want:   0.75,
		},
		{
			name:   "zero tokens",
			model:  "claude-opus-4-6",
			tokens: 0,
			want:   0,
		},
		{
			name:   "negative tokens",
			model:  "claude-opus-4-6",
			tokens: -100,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CacheReadCostForTokens(tt.model, tt.tokens)
			if got != tt.want {
				t.Fatalf("CacheReadCostForTokens(%q, %d) = %f, want %f",
					tt.model, tt.tokens, got, tt.want)
			}
		})
	}
}
