package analyzer

import (
	"encoding/json"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestFindTangents_NoCWD(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1"},
	}
	result := FindTangents(entries)
	if len(result.Groups) != 0 {
		t.Errorf("expected 0 groups with no CWD, got %d", len(result.Groups))
	}
}

func TestFindTangents_NoTangents(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/myproject"},
		makeToolUseEntry("a1", "Read", "/home/user/dev/myproject/main.go"),
		makeToolResultEntry("u2"),
		makeToolUseEntry("a2", "Edit", "/home/user/dev/myproject/main.go"),
	}
	result := FindTangents(entries)
	if len(result.Groups) != 0 {
		t.Errorf("expected 0 tangent groups, got %d", len(result.Groups))
	}
}

func TestFindTangents_BasicTangent(t *testing.T) {
	entries := []jsonl.Entry{
		// Main project work
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/myproject"},
		makeToolUseEntry("a1", "Read", "/home/user/dev/myproject/main.go"),
		// Tangent: references external repo
		makeToolUseEntry("a2", "Read", "/home/user/dev/other-repo/server.go"),
		makeToolResultEntry("u3"),
		// Back to main project
		makeToolUseEntry("a3", "Read", "/home/user/dev/myproject/handler.go"),
	}

	result := FindTangents(entries)
	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 tangent group, got %d", len(result.Groups))
	}

	g := result.Groups[0]
	if g.StartIndex != 2 {
		t.Errorf("expected start index 2, got %d", g.StartIndex)
	}
	if len(g.EntryIndices) < 2 {
		t.Errorf("expected at least 2 entries in tangent, got %d", len(g.EntryIndices))
	}
	if result.SessionCWD != "/home/user/dev/myproject" {
		t.Errorf("expected CWD /home/user/dev/myproject, got %s", result.SessionCWD)
	}
}

func TestFindTangents_TangentWithResponse(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/myproject"},
		makeToolUseEntry("a1", "Read", "/home/user/dev/myproject/main.go"),
		// Tangent starts: external tool_use
		makeToolUseEntry("a2", "Read", "/home/user/dev/other-repo/config.yaml"),
		// tool_result (no path refs, but part of tangent)
		makeToolResultEntry("u3"),
		// Assistant text response about external repo (no paths)
		{Type: jsonl.TypeAssistant, UUID: "a3", Message: &jsonl.Message{
			Role:    "assistant",
			Content: mustMarshal([]jsonl.ContentBlock{{Type: "text", Text: "The config looks correct."}}),
		}, RawSize: 200},
		// Back to main project
		makeToolUseEntry("a4", "Read", "/home/user/dev/myproject/handler.go"),
	}

	result := FindTangents(entries)
	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 tangent group, got %d", len(result.Groups))
	}

	g := result.Groups[0]
	// Should include the external read, tool_result, and assistant response
	if len(g.EntryIndices) < 3 {
		t.Errorf("expected at least 3 entries in tangent (read + result + response), got %d", len(g.EntryIndices))
	}
}

func TestFindTangents_MixedPathsNotTangent(t *testing.T) {
	// Entry references both external and CWD paths — not a tangent
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/myproject"},
		makeMultiToolEntry("a1",
			toolUse("Read", "/home/user/dev/other-repo/config.yaml"),
			toolUse("Read", "/home/user/dev/myproject/main.go"),
		),
	}

	result := FindTangents(entries)
	if len(result.Groups) != 0 {
		t.Errorf("expected 0 tangent groups for mixed paths, got %d", len(result.Groups))
	}
}

func TestFindTangents_CWDModificationNotTangent(t *testing.T) {
	// Entry has external read but also modifies CWD — not a tangent
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/myproject"},
		makeToolUseEntry("a1", "Read", "/home/user/dev/other-repo/server.go"),
		makeToolUseEntry("a2", "Write", "/home/user/dev/myproject/output.go"),
	}

	result := FindTangents(entries)
	// The first entry refs external, second refs CWD — tangent should stop before CWD ref
	// Since there's only 1 entry before the CWD ref, it's less than 2, so no tangent
	if len(result.Groups) != 0 {
		t.Errorf("expected 0 tangent groups when CWD is modified, got %d", len(result.Groups))
	}
}

func TestFindTangents_MultipleTangents(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/myproject"},
		makeToolUseEntry("a1", "Read", "/home/user/dev/myproject/main.go"),
		// First tangent
		makeToolUseEntry("a2", "Read", "/home/user/dev/repo-a/file.go"),
		makeToolResultEntry("u3"),
		// Back to main
		makeToolUseEntry("a3", "Read", "/home/user/dev/myproject/util.go"),
		// Second tangent
		makeToolUseEntry("a4", "Read", "/home/user/dev/repo-b/config.yaml"),
		makeToolResultEntry("u5"),
		// Back to main
		makeToolUseEntry("a5", "Read", "/home/user/dev/myproject/handler.go"),
	}

	result := FindTangents(entries)
	if len(result.Groups) != 2 {
		t.Errorf("expected 2 tangent groups, got %d", len(result.Groups))
	}
	if result.ExternalDirs != 2 {
		t.Errorf("expected 2 external dirs, got %d", result.ExternalDirs)
	}
}

func TestFindTangents_AllTangentIndices(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/myproject"},
		makeToolUseEntry("a1", "Read", "/home/user/dev/myproject/main.go"),
		makeToolUseEntry("a2", "Read", "/home/user/dev/other-repo/file.go"),
		makeToolResultEntry("u3"),
		makeToolUseEntry("a3", "Read", "/home/user/dev/myproject/handler.go"),
	}

	result := FindTangents(entries)
	indices := result.AllTangentIndices()
	if !indices[2] {
		t.Error("expected index 2 to be tangent")
	}
	if !indices[3] {
		t.Error("expected index 3 to be tangent")
	}
	if indices[0] || indices[1] || indices[4] {
		t.Error("non-tangent indices should not be in set")
	}
}

func TestDetectSessionCWD(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser},
		{Type: jsonl.TypeAssistant, CWD: "/home/user/dev/myproject"},
		{Type: jsonl.TypeUser, CWD: "/home/user/dev/myproject"},
	}
	cwd := detectSessionCWD(entries)
	if cwd != "/home/user/dev/myproject" {
		t.Errorf("expected /home/user/dev/myproject, got %s", cwd)
	}
}

func TestDetectSessionCWD_NoCWD(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser},
		{Type: jsonl.TypeAssistant},
	}
	cwd := detectSessionCWD(entries)
	if cwd != "" {
		t.Errorf("expected empty CWD, got %s", cwd)
	}
}

func TestIsOutsideCWD(t *testing.T) {
	tests := []struct {
		path    string
		cwd     string
		outside bool
	}{
		{"/home/user/dev/myproject/main.go", "/home/user/dev/myproject", false},
		{"/home/user/dev/myproject/internal/pkg/file.go", "/home/user/dev/myproject", false},
		{"/home/user/dev/other-repo/main.go", "/home/user/dev/myproject", true},
		{"/home/user/dev/myproject", "/home/user/dev/myproject", false},
		{"/tmp/file.txt", "/home/user/dev/myproject", true},
		{"", "/home/user/dev/myproject", false},
		{"/home/user/dev/myproject/main.go", "", false},
	}

	for _, tt := range tests {
		got := isOutsideCWD(tt.path, tt.cwd)
		if got != tt.outside {
			t.Errorf("isOutsideCWD(%q, %q) = %v, want %v", tt.path, tt.cwd, got, tt.outside)
		}
	}
}

func TestIsModifyingTool(t *testing.T) {
	modifying := []string{"Write", "Edit", "Bash", "bash", "write_file", "NotebookEdit"}
	for _, name := range modifying {
		if !isModifyingTool(name) {
			t.Errorf("expected %q to be modifying tool", name)
		}
	}

	nonModifying := []string{"Read", "Grep", "Glob", "View", "WebSearch"}
	for _, name := range nonModifying {
		if isModifyingTool(name) {
			t.Errorf("expected %q to NOT be modifying tool", name)
		}
	}
}

func TestIsSystemPath(t *testing.T) {
	if !isSystemPath("/usr/bin/go") {
		t.Error("expected /usr/bin/go to be system path")
	}
	if !isSystemPath("/tmp/test.txt") {
		t.Error("expected /tmp/test.txt to be system path")
	}
	if isSystemPath("/home/user/dev/project/main.go") {
		t.Error("expected project path to NOT be system path")
	}
}

func TestExternalRootDir(t *testing.T) {
	got := externalRootDir("/home/user/dev/other-repo/src/main.go", "/home/user/dev/myproject")
	if got != "/home/user/dev/other-repo" {
		t.Errorf("expected /home/user/dev/other-repo, got %s", got)
	}
}

func TestExtractBashCommandPaths(t *testing.T) {
	input := mustMarshalRaw(map[string]string{
		"command": `cat /home/user/dev/other-repo/README.md && ls /usr/bin`,
	})
	paths := extractBashCommandPaths(input)
	// Should find /home/user/dev/other-repo/README.md but NOT /usr/bin (system path)
	found := false
	for _, p := range paths {
		if p == "/home/user/dev/other-repo/README.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find external path in bash command, got %v", paths)
	}
}

// --- helpers ---

func makeToolUseEntry(uuid, toolName, path string) jsonl.Entry {
	input := mustMarshalRaw(map[string]string{"file_path": path})
	content := mustMarshal([]jsonl.ContentBlock{
		{Type: "tool_use", ID: "toolu_" + uuid, Name: toolName, Input: input},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeAssistant,
		UUID:    uuid,
		RawSize: len(content) + 100,
		Message: &jsonl.Message{Role: "assistant", Content: content},
	}
}

func makeToolResultEntry(uuid string) jsonl.Entry {
	content := mustMarshal([]jsonl.ContentBlock{
		{Type: "tool_result", ToolUseID: "toolu_prev", Content: mustMarshalRaw("result content")},
	})
	return jsonl.Entry{
		Type:    jsonl.TypeUser,
		UUID:    uuid,
		RawSize: len(content) + 100,
		Message: &jsonl.Message{Role: "user", Content: content},
	}
}

type toolUseSpec struct {
	Name string
	Path string
}

func toolUse(name, path string) toolUseSpec {
	return toolUseSpec{Name: name, Path: path}
}

func makeMultiToolEntry(uuid string, tools ...toolUseSpec) jsonl.Entry {
	var blocks []jsonl.ContentBlock
	for i, t := range tools {
		input := mustMarshalRaw(map[string]string{"file_path": t.Path})
		blocks = append(blocks, jsonl.ContentBlock{
			Type:  "tool_use",
			ID:    "toolu_" + uuid + "_" + string(rune('a'+i)),
			Name:  t.Name,
			Input: input,
		})
	}
	content := mustMarshal(blocks)
	return jsonl.Entry{
		Type:    jsonl.TypeAssistant,
		UUID:    uuid,
		RawSize: len(content) + 100,
		Message: &jsonl.Message{Role: "assistant", Content: content},
	}
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func mustMarshalRaw(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
