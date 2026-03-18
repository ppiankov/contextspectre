package commands

import (
	"runtime"
	"testing"
)

func TestConflictPathMatchesScopes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths in test fixtures")
	}
	scopes := []string{"/repo/internal/analyzer"}
	if !conflictPathMatchesScopes("/repo/internal/analyzer/cost.go", scopes) {
		t.Fatal("expected path to match scope")
	}
	if conflictPathMatchesScopes("/repo/internal/tui/view.go", scopes) {
		t.Fatal("did not expect path to match scope")
	}
}

func TestConflictSessionReferencesScopes(t *testing.T) {
	ref := map[string]bool{
		"/repo/internal/analyzer": true,
	}
	if !conflictSessionReferencesScopes(ref, []string{"/repo/internal/analyzer"}) {
		t.Fatal("expected scope reference to match")
	}
	if conflictSessionReferencesScopes(ref, []string{"/repo/internal/tui"}) {
		t.Fatal("did not expect unrelated scope reference")
	}
}

func TestNormalizeConflictPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix paths in test fixtures")
	}
	got := normalizeConflictPath(" /repo/internal/../internal/analyzer/cost.go ")
	if got != "/repo/internal/analyzer/cost.go" {
		t.Fatalf("normalizeConflictPath = %q", got)
	}
}
