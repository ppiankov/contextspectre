package commands

import "testing"

func TestInferTimelineTopicFromFiles(t *testing.T) {
	files := []string{
		"/repo/internal/analyzer/a.go",
		"/repo/internal/analyzer/b.go",
		"/repo/internal/commands/c.go",
	}
	got := inferTimelineTopic(files, nil, 2)
	if got != "/repo/internal/analyzer" {
		t.Fatalf("inferTimelineTopic = %q, want %q", got, "/repo/internal/analyzer")
	}
}

func TestInferTimelineTopicFromTools(t *testing.T) {
	got := inferTimelineTopic(nil, map[string]int{"Bash": 4, "Read": 2}, 1)
	if got != "debugging/testing" {
		t.Fatalf("inferTimelineTopic = %q, want %q", got, "debugging/testing")
	}
}
