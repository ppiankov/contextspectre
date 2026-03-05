package commands

import "testing"

func TestNormalizeSyncHeading(t *testing.T) {
	got := normalizeSyncHeading("Architecture-Decisions!")
	if got != "architecture decisions" {
		t.Fatalf("normalizeSyncHeading = %q, want %q", got, "architecture decisions")
	}
}

func TestFindSyncHeadingMatch(t *testing.T) {
	targets := []string{"Architecture decisions", "Active constraints"}
	match, ok := findSyncHeadingMatch("Architecture-decisions", targets)
	if !ok {
		t.Fatal("expected match")
	}
	if match != "Architecture decisions" {
		t.Fatalf("match = %q, want %q", match, "Architecture decisions")
	}
}
