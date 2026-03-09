package analyzer

import (
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestBuildActiveCWDMap_Static(t *testing.T) {
	entries := []jsonl.Entry{
		{CWD: "/home/user/dev/proj"},
		{},
		{},
	}
	cwds := buildActiveCWDMap(entries, "")
	if cwds[0] != "/home/user/dev/proj" {
		t.Errorf("cwds[0] = %q, want /home/user/dev/proj", cwds[0])
	}
	if cwds[2] != "/home/user/dev/proj" {
		t.Errorf("cwds[2] = %q, want /home/user/dev/proj (inherited)", cwds[2])
	}
}

func TestBuildActiveCWDMap_Dynamic(t *testing.T) {
	entries := []jsonl.Entry{
		{CWD: "/home/user/dev/projA"},
		{},
		{CWD: "/home/user/dev/projB"},
		{},
	}
	cwds := buildActiveCWDMap(entries, "")
	if cwds[1] != "/home/user/dev/projA" {
		t.Errorf("cwds[1] = %q, want projA", cwds[1])
	}
	if cwds[2] != "/home/user/dev/projB" {
		t.Errorf("cwds[2] = %q, want projB", cwds[2])
	}
	if cwds[3] != "/home/user/dev/projB" {
		t.Errorf("cwds[3] = %q, want projB (inherited)", cwds[3])
	}
}

func TestBuildActiveCWDMap_InitialCWD(t *testing.T) {
	entries := []jsonl.Entry{{}, {}, {}}
	cwds := buildActiveCWDMap(entries, "/initial")
	for i, cwd := range cwds {
		if cwd != "/initial" {
			t.Errorf("cwds[%d] = %q, want /initial", i, cwd)
		}
	}
}

func TestPathMixRatio(t *testing.T) {
	tests := []struct {
		name     string
		info     entryScopeInfo
		wantZero bool
		wantHalf bool
	}{
		{"no paths", entryScopeInfo{}, true, false},
		{"all CWD", entryScopeInfo{cwdPathCount: 3}, true, false},
		{"all external", entryScopeInfo{externalPathCount: 2}, false, false},
		{"half and half", entryScopeInfo{cwdPathCount: 1, externalPathCount: 1}, false, true},
	}
	for _, tt := range tests {
		ratio := pathMixRatio(tt.info)
		if tt.wantZero && ratio != 0 {
			t.Errorf("%s: ratio = %f, want 0", tt.name, ratio)
		}
		if tt.wantHalf && ratio != 0.5 {
			t.Errorf("%s: ratio = %f, want 0.5", tt.name, ratio)
		}
	}
}

func TestClassifyEntryScopes_PathCounts(t *testing.T) {
	entries := []jsonl.Entry{
		// Entry with 1 CWD path and 1 external path (mixed)
		makeMultiToolEntry("a1",
			toolUse("Read", "/home/user/dev/proj/main.go"),
			toolUse("Read", "/home/user/dev/other/lib.go"),
		),
	}
	cwds := []string{"/home/user/dev/proj"}
	infos := classifyEntryScopes(entries, cwds)
	if infos[0].cwdPathCount != 1 {
		t.Errorf("cwdPathCount = %d, want 1", infos[0].cwdPathCount)
	}
	if infos[0].externalPathCount != 1 {
		t.Errorf("externalPathCount = %d, want 1", infos[0].externalPathCount)
	}
	if !infos[0].hasCWD || !infos[0].hasExternal {
		t.Error("expected both hasCWD and hasExternal to be true")
	}
}

func TestComputeEpochScopes_Proportional(t *testing.T) {
	// 3 entries: 1 pure CWD, 1 mixed (50/50), 1 pure external
	entries := []jsonl.Entry{
		makeToolUseEntry("a1", "Read", "/home/user/dev/proj/main.go"),
		makeMultiToolEntry("a2",
			toolUse("Read", "/home/user/dev/proj/util.go"),
			toolUse("Read", "/home/user/dev/other/lib.go"),
		),
		makeToolUseEntry("a3", "Read", "/home/user/dev/other/server.go"),
	}
	cwds := []string{
		"/home/user/dev/proj",
		"/home/user/dev/proj",
		"/home/user/dev/proj",
	}
	infos := classifyEntryScopes(entries, cwds)

	scopes := computeEpochScopes(entries, infos, nil, "")

	if len(scopes) != 1 {
		t.Fatalf("expected 1 epoch, got %d", len(scopes))
	}
	es := scopes[0]

	// Pure CWD = 1.0, mixed = 0.5 in-scope, pure external = 0
	// InScope = round(1.5) = 2
	if es.InScope != 2 {
		t.Errorf("InScope = %d, want 2 (1 pure + 0.5 mixed)", es.InScope)
	}
	// Pure external = 1.0, mixed = 0.5 out-scope
	// OutScope = round(1.5) = 2
	if es.OutScope != 2 {
		t.Errorf("OutScope = %d, want 2 (1 pure + 0.5 mixed)", es.OutScope)
	}
	if es.MixedScope != 1 {
		t.Errorf("MixedScope = %d, want 1", es.MixedScope)
	}
}

func TestComputeEpochScopes_PureCWD(t *testing.T) {
	entries := []jsonl.Entry{
		makeToolUseEntry("a1", "Read", "/home/user/dev/proj/main.go"),
		makeToolUseEntry("a2", "Read", "/home/user/dev/proj/util.go"),
	}
	cwds := []string{"/home/user/dev/proj", "/home/user/dev/proj"}
	infos := classifyEntryScopes(entries, cwds)
	scopes := computeEpochScopes(entries, infos, nil, "")

	es := scopes[0]
	if es.InScope != 2 {
		t.Errorf("InScope = %d, want 2", es.InScope)
	}
	if es.OutScope != 0 {
		t.Errorf("OutScope = %d, want 0", es.OutScope)
	}
	if es.MixedScope != 0 {
		t.Errorf("MixedScope = %d, want 0", es.MixedScope)
	}
}

func TestFindTangents_MixedDoesNotTerminate(t *testing.T) {
	// Tangent with a mixed-scope entry in the middle should not split
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/proj"},
		makeToolUseEntry("a1", "Read", "/home/user/dev/proj/main.go"), // CWD
		// Tangent starts
		makeToolUseEntry("a2", "Read", "/home/user/dev/other/file.go"), // external
		// Mixed entry: reads both CWD and external
		makeMultiToolEntry("a3",
			toolUse("Read", "/home/user/dev/proj/util.go"),
			toolUse("Read", "/home/user/dev/other/lib.go"),
		),
		makeToolUseEntry("a4", "Read", "/home/user/dev/other/config.go"), // external
		// Back to CWD
		makeToolUseEntry("a5", "Read", "/home/user/dev/proj/handler.go"),
	}

	result := FindTangents(entries)
	// Should be 1 tangent group spanning entries 2-4 (mixed doesn't split)
	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 tangent group (mixed doesn't split), got %d", len(result.Groups))
	}
	g := result.Groups[0]
	if g.StartIndex != 2 {
		t.Errorf("StartIndex = %d, want 2", g.StartIndex)
	}
	if g.EndIndex != 4 {
		t.Errorf("EndIndex = %d, want 4", g.EndIndex)
	}
	if g.MixedScopeEntries != 1 {
		t.Errorf("MixedScopeEntries = %d, want 1", g.MixedScopeEntries)
	}
}

func TestFindTangents_CWDModificationStillTerminates(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/proj"},
		makeToolUseEntry("a1", "Read", "/home/user/dev/other/file.go"),   // external
		makeToolUseEntry("a2", "Write", "/home/user/dev/proj/output.go"), // CWD modification
		makeToolUseEntry("a3", "Read", "/home/user/dev/other/config.go"), // external
	}

	result := FindTangents(entries)
	// CWD modification at index 2 terminates — only 1 external entry before it
	// (less than 2 entries required), so no tangent group
	if len(result.Groups) != 0 {
		t.Errorf("expected 0 groups (CWD modification terminates), got %d", len(result.Groups))
	}
}

func TestAnalyzeScopeDrift_DynamicCWD(t *testing.T) {
	entries := []jsonl.Entry{
		{Type: jsonl.TypeUser, UUID: "u1", CWD: "/home/user/dev/projA"},
		makeToolUseEntry("a1", "Read", "/home/user/dev/projA/main.go"), // in-scope for projA
		// CWD changes to projB
		{Type: jsonl.TypeUser, UUID: "u2", CWD: "/home/user/dev/projB"},
		makeToolUseEntry("a2", "Read", "/home/user/dev/projB/main.go"), // in-scope for projB
		makeToolUseEntry("a3", "Read", "/home/user/dev/projA/main.go"), // out-of-scope for projB
	}

	drift := AnalyzeScopeDrift(entries, nil, "")
	if drift.SessionProject != "/home/user/dev/projA" {
		t.Errorf("SessionProject = %q, want projA", drift.SessionProject)
	}

	// Entry 1 is in-scope (projA path with projA CWD)
	// Entry 3 is in-scope (projB path with projB CWD)
	// Entry 4 is out-of-scope (projA path with projB CWD)
	if drift.TotalOutScope < 1 {
		t.Errorf("TotalOutScope = %d, want >= 1", drift.TotalOutScope)
	}
}
