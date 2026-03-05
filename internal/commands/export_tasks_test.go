package commands

import "testing"

func TestClassifyExportText(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"TODO: add retry", "todo"},
		{"need to wire this later", "todo"},
		{"Should we split this command?", "todo"},
		{"not sure if this is done", "question"},
		{"Is this complete?", "question"},
		{"plain statement", ""},
	}

	for _, tt := range tests {
		if got := classifyExportText(tt.line); got != tt.want {
			t.Fatalf("classifyExportText(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestAddExportItemDedupSources(t *testing.T) {
	index := map[string]*exportItem{}
	src := exportSource{SessionID: "abc", Epoch: 1, EntryIndex: 10}
	addExportItem(index, "todo", "TODO: add tests", src)
	addExportItem(index, "todo", "TODO: add tests", src)

	if len(index) != 1 {
		t.Fatalf("len(index) = %d, want 1", len(index))
	}
	for _, it := range index {
		if len(it.Sources) != 1 {
			t.Fatalf("len(sources) = %d, want 1", len(it.Sources))
		}
	}
}
