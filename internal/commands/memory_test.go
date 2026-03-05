package commands

import "testing"

func TestTopCountLabels(t *testing.T) {
	counts := map[string]int{
		"Read": 8,
		"Edit": 8,
		"Bash": 3,
	}
	got := topCountLabels(counts, 2, true)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != "Edit(8)" || got[1] != "Read(8)" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestJoinOrPlaceholder(t *testing.T) {
	if got := joinOrPlaceholder(nil); got != "n/a" {
		t.Fatalf("joinOrPlaceholder(nil) = %q, want %q", got, "n/a")
	}
	if got := joinOrPlaceholder([]string{"a", "b"}); got != "a, b" {
		t.Fatalf("joinOrPlaceholder = %q, want %q", got, "a, b")
	}
}
