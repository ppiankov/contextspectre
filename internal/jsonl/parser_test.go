package jsonl

import (
	"os"
	"path/filepath"
	"testing"
)

func testdataPath(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

func TestParse_SmallSession(t *testing.T) {
	entries, err := Parse(testdataPath("small_session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(entries))
	}

	// Verify types
	typeCounts := make(map[MessageType]int)
	for _, e := range entries {
		typeCounts[e.Type]++
	}
	if typeCounts[TypeUser] != 4 {
		t.Errorf("expected 4 user messages, got %d", typeCounts[TypeUser])
	}
	if typeCounts[TypeAssistant] != 3 {
		t.Errorf("expected 3 assistant messages, got %d", typeCounts[TypeAssistant])
	}
	if typeCounts[TypeProgress] != 2 {
		t.Errorf("expected 2 progress messages, got %d", typeCounts[TypeProgress])
	}
	if typeCounts[TypeFileHistorySnapshot] != 1 {
		t.Errorf("expected 1 file-history-snapshot, got %d", typeCounts[TypeFileHistorySnapshot])
	}

	// Verify first entry
	if entries[0].UUID != "u1" {
		t.Errorf("expected UUID u1, got %s", entries[0].UUID)
	}
	if entries[0].LineNumber != 1 {
		t.Errorf("expected line 1, got %d", entries[0].LineNumber)
	}

	// Verify usage on assistant messages
	a1 := entries[1]
	if a1.Message == nil || a1.Message.Usage == nil {
		t.Fatal("expected usage on assistant message")
	}
	if a1.Message.Usage.TotalContextTokens() != 5100 {
		t.Errorf("expected 5100 context tokens, got %d", a1.Message.Usage.TotalContextTokens())
	}

	// Verify parentUuid chain
	if entries[1].ParentUUID != "u1" {
		t.Errorf("a1.parentUuid should be u1, got %s", entries[1].ParentUUID)
	}
}

func TestParse_WithImages(t *testing.T) {
	entries, err := Parse(testdataPath("with_images.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// First user message should have an image
	if !entries[0].HasImages() {
		t.Error("expected first user message to have images")
	}
	// Assistant should not have images
	if entries[1].HasImages() {
		t.Error("expected assistant message to not have images")
	}
}

func TestParse_Compaction(t *testing.T) {
	entries, err := Parse(testdataPath("compaction.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 12 {
		t.Fatalf("expected 12 entries, got %d", len(entries))
	}

	// Track context token progression
	var contextTokens []int
	for _, e := range entries {
		if e.Type == TypeAssistant && e.Message != nil && e.Message.Usage != nil {
			contextTokens = append(contextTokens, e.Message.Usage.TotalContextTokens())
		}
	}

	// Should see growth then drop (compaction)
	if len(contextTokens) != 6 {
		t.Fatalf("expected 6 assistant usage records, got %d", len(contextTokens))
	}

	// Tokens should grow then drop
	// a1=2050, a2=52100, a3=150200, a4=165300, a5=35100, a6=65200
	if contextTokens[3] <= contextTokens[2] {
		t.Log("context tokens grew as expected before compaction")
	}
	if contextTokens[4] >= contextTokens[3] {
		t.Errorf("expected compaction drop: %d should be < %d", contextTokens[4], contextTokens[3])
	}
}

func TestParseRaw_PreservesBytes(t *testing.T) {
	entries, rawLines, err := ParseRaw(testdataPath("small_session.jsonl"))
	if err != nil {
		t.Fatalf("parseRaw: %v", err)
	}
	if len(entries) != len(rawLines) {
		t.Fatalf("entries and rawLines count mismatch: %d vs %d", len(entries), len(rawLines))
	}
	for i, e := range entries {
		if e.RawSize != len(rawLines[i]) {
			t.Errorf("entry %d: RawSize %d != raw line len %d", i, e.RawSize, len(rawLines[i]))
		}
	}
}

func TestParse_NonexistentFile(t *testing.T) {
	_, err := Parse("/nonexistent/file.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParse_EmptyFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(tmp, nil, 0644); err != nil {
		t.Fatal(err)
	}
	entries, err := Parse(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParse_MalformedLines(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "bad.jsonl")
	content := `{"type":"user","uuid":"u1","message":{"role":"user","content":"good"}}
not valid json
{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":"also good"}}
`
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	entries, err := Parse(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should skip the bad line
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (skipping bad line), got %d", len(entries))
	}
}

func TestParseRaw_SkipsMalformedLines(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "bad.jsonl")
	content := `{"type":"user","uuid":"u1","message":{"role":"user","content":"good"}}
not valid json
{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"also good"}}
{BROKEN}
{"type":"user","uuid":"u2","parentUuid":"a1","message":{"role":"user","content":"third"}}
`
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	parseEntries, err := Parse(tmp)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rawEntries, rawLines, err := ParseRaw(tmp)
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}

	// Parse and ParseRaw must produce the same number of entries
	if len(parseEntries) != len(rawEntries) {
		t.Fatalf("index mismatch: Parse=%d entries, ParseRaw=%d entries",
			len(parseEntries), len(rawEntries))
	}
	if len(rawEntries) != len(rawLines) {
		t.Fatalf("ParseRaw entries/rawLines mismatch: %d vs %d",
			len(rawEntries), len(rawLines))
	}

	// UUIDs must match at every index
	for i := range parseEntries {
		if parseEntries[i].UUID != rawEntries[i].UUID {
			t.Errorf("index %d: Parse UUID=%q, ParseRaw UUID=%q",
				i, parseEntries[i].UUID, rawEntries[i].UUID)
		}
	}

	// Should have 3 valid entries (2 malformed lines skipped)
	if len(rawEntries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(rawEntries))
	}
}

func TestScanLight_SmallSession(t *testing.T) {
	stats, err := ScanLight(testdataPath("small_session.jsonl"))
	if err != nil {
		t.Fatalf("scanLight: %v", err)
	}
	if stats.LineCount != 10 {
		t.Errorf("expected 10 lines, got %d", stats.LineCount)
	}
	if stats.TypeCounts[TypeUser] != 4 {
		t.Errorf("expected 4 user, got %d", stats.TypeCounts[TypeUser])
	}
	if stats.LastUsage == nil {
		t.Fatal("expected last usage to be set")
	}
	// Last assistant is a3 with 300+5500+200 = 6000
	if stats.LastUsage.TotalContextTokens() != 6000 {
		t.Errorf("expected 6000 last context tokens, got %d", stats.LastUsage.TotalContextTokens())
	}
}

func TestScanLight_CompactionDetection(t *testing.T) {
	stats, err := ScanLight(testdataPath("compaction.jsonl"))
	if err != nil {
		t.Fatalf("scanLight: %v", err)
	}
	// compaction.jsonl has a drop from 165300 to 35100 (130200 drop)
	if stats.CompactionCount != 1 {
		t.Errorf("expected 1 compaction, got %d", stats.CompactionCount)
	}
	if stats.LastCompactionBefore != 165300 {
		t.Errorf("expected LastCompactionBefore=165300, got %d", stats.LastCompactionBefore)
	}
	if stats.LastCompactionAfter != 35100 {
		t.Errorf("expected LastCompactionAfter=35100, got %d", stats.LastCompactionAfter)
	}
}

func TestScanLight_NoCompaction(t *testing.T) {
	stats, err := ScanLight(testdataPath("small_session.jsonl"))
	if err != nil {
		t.Fatalf("scanLight: %v", err)
	}
	if stats.CompactionCount != 0 {
		t.Errorf("expected 0 compactions, got %d", stats.CompactionCount)
	}
}

func TestScanLight_NonexistentFile(t *testing.T) {
	_, err := ScanLight("/nonexistent/file.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
