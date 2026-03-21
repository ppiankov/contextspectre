package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", name)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dst := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatalf("write fixture copy: %v", err)
	}
	return dst
}

func TestDelete_Basic(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	// Delete first user message (index 0)
	result, err := Delete(path, map[int]bool{0: true})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if result.EntriesRemoved != 1 {
		t.Errorf("expected 1 removed, got %d", result.EntriesRemoved)
	}

	// Verify the file has fewer entries
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(entries) != 9 {
		t.Errorf("expected 9 entries after deletion, got %d", len(entries))
	}

	// Backup should exist
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Error("expected backup file to exist")
	}
}

func TestDelete_ChainRepair(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	// Delete assistant a1 (index 1) — u2 and p1 have parentUuid=a1
	result, err := Delete(path, map[int]bool{1: true})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if result.ChainRepairs == 0 {
		t.Error("expected chain repairs")
	}

	// Verify the chain was repaired
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	// Find the entry that used to point to a1
	for _, e := range entries {
		if e.UUID == "u2" {
			if e.ParentUUID == "a1" {
				t.Error("u2 should no longer point to deleted a1")
			}
			if e.ParentUUID != "u1" {
				t.Errorf("u2 should point to u1 (a1's parent), got %s", e.ParentUUID)
			}
		}
	}
}

func TestDelete_Empty(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")
	result, err := Delete(path, nil)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if result.EntriesRemoved != 0 {
		t.Errorf("expected 0 removed, got %d", result.EntriesRemoved)
	}
}

func TestDelete_MultipleEntries(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	// Delete progress messages (indices 2, 7) and file-history-snapshot (index 8)
	result, err := Delete(path, map[int]bool{2: true, 7: true, 8: true})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if result.EntriesRemoved != 3 {
		t.Errorf("expected 3 removed, got %d", result.EntriesRemoved)
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(entries) != 7 {
		t.Errorf("expected 7 entries, got %d", len(entries))
	}
}

func TestReplaceImages(t *testing.T) {
	path := copyFixture(t, "with_images.jsonl")

	result, err := ReplaceImages(path)
	if err != nil {
		t.Fatalf("replace images: %v", err)
	}
	if result.ImagesReplaced != 2 {
		t.Errorf("expected 2 images replaced, got %d", result.ImagesReplaced)
	}
	if result.BytesSaved <= 0 {
		t.Error("expected positive bytes saved")
	}

	// Verify images were replaced with text placeholders
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	placeholders := 0
	for _, e := range entries {
		blocks, _ := jsonl.ParseContentBlocks(e.Message.Content)
		for _, b := range blocks {
			if b.Type == "image" {
				t.Error("expected image blocks to be replaced with text blocks")
			}
			if b.Type == "text" && b.Text == "[image removed by contextspectre]" {
				placeholders++
			}
		}
	}
	if placeholders != 2 {
		t.Errorf("expected 2 text placeholders, got %d", placeholders)
	}

	// Backup should exist
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Error("expected backup file after image replacement")
	}
}

func TestReplaceImages_NoImages(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	result, err := ReplaceImages(path)
	if err != nil {
		t.Fatalf("replace images: %v", err)
	}
	if result.ImagesReplaced != 0 {
		t.Errorf("expected 0 images replaced, got %d", result.ImagesReplaced)
	}

	// No backup should be created when no changes are made
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("expected no backup when no changes made")
	}
}

func TestRemoveProgress(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	result, err := RemoveProgress(path)
	if err != nil {
		t.Fatalf("remove progress: %v", err)
	}
	if result.EntriesRemoved != 2 {
		t.Errorf("expected 2 progress messages removed, got %d", result.EntriesRemoved)
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	for _, e := range entries {
		if e.Type == jsonl.TypeProgress {
			t.Error("expected no progress messages after removal")
		}
	}
}

func TestResolveParent(t *testing.T) {
	// Chain: a → b → c → d (d is not deleted)
	remap := map[string]string{
		"a": "b",
		"b": "c",
		"c": "d",
	}

	got := resolveParent("a", remap)
	if got != "d" {
		t.Errorf("expected d, got %s", got)
	}

	// Non-deleted entry
	got = resolveParent("d", remap)
	if got != "d" {
		t.Errorf("expected d (unchanged), got %s", got)
	}
}

func TestResolveParent_Cycle(t *testing.T) {
	remap := map[string]string{
		"a": "b",
		"b": "a",
	}
	got := resolveParent("a", remap)
	if got != "" {
		t.Errorf("expected empty string for cycle, got %s", got)
	}
}

func TestPatchParentUUID_Basic(t *testing.T) {
	// PatchParentUUID finds and patches all entries whose parentUuid
	// references a non-existent UUID. The indices parameter signals that
	// patching is needed but entries are identified by missing parent scan.
	lines := []string{
		`{"type":"user","uuid":"u1","parentUuid":"","message":{"role":"user","content":"q1"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"r1"}}`,
		`{"type":"user","uuid":"u2","parentUuid":"GONE","message":{"role":"user","content":"q2"}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"u2","message":{"role":"assistant","content":"r2"}}`,
	}
	path := filepath.Join(t.TempDir(), "patch_basic.jsonl")
	var data []byte
	for _, l := range lines {
		data = append(data, []byte(l+"\n")...)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Index 2 has parentUuid=GONE (missing)
	patched, err := PatchParentUUID(path, map[int]bool{2: true})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched != 1 {
		t.Errorf("expected 1 patched, got %d", patched)
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	// u2 should have empty parentUuid
	if entries[2].ParentUUID != "" {
		t.Errorf("expected empty parentUuid after patch, got %q", entries[2].ParentUUID)
	}
	// a2 should be unchanged (its parent u2 exists)
	if entries[3].ParentUUID != "u2" {
		t.Errorf("expected a2.parentUuid=u2 unchanged, got %q", entries[3].ParentUUID)
	}
}

func TestPatchParentUUID_Empty(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")
	patched, err := PatchParentUUID(path, nil)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched != 0 {
		t.Errorf("expected 0 patched, got %d", patched)
	}
}

func TestRepair_MissingParentConverges(t *testing.T) {
	// Build a session where an entry references a non-existent parent UUID,
	// simulating the state after clean removes entries. Repair should fix it
	// in ONE pass by patching parentUuid, not deleting.
	lines := []string{
		`{"type":"user","uuid":"u1","parentUuid":"","message":{"role":"user","content":"first"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"reply1"}}`,
		// u2 references a parent that doesn't exist (simulates post-clean state)
		`{"type":"user","uuid":"u2","parentUuid":"GONE","message":{"role":"user","content":"second"}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"u2","message":{"role":"assistant","content":"reply2"}}`,
		`{"type":"user","uuid":"u3","parentUuid":"a2","message":{"role":"user","content":"third"}}`,
	}
	path := filepath.Join(t.TempDir(), "broken_chain.jsonl")
	var data []byte
	for _, l := range lines {
		data = append(data, []byte(l+"\n")...)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// First pass: diagnose + repair
	entries, _ := jsonl.Parse(path)
	diag := analyzer.Diagnose(entries)
	if len(diag.Issues) == 0 {
		t.Fatal("expected issues for broken chain")
	}

	result, err := Repair(path, diag.Issues, false)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if result.ParentPatches == 0 {
		t.Error("expected parent patches, got 0")
	}

	// Second pass: should find NO issues (convergence in one pass)
	entries, _ = jsonl.Parse(path)
	diag = analyzer.Diagnose(entries)
	if len(diag.Issues) != 0 {
		for _, issue := range diag.Issues {
			t.Errorf("unexpected issue after repair: %s (entry %d)", issue.Description, issue.EntryIndex)
		}
		t.Fatalf("expected 0 issues after single repair pass, got %d", len(diag.Issues))
	}

	// All 5 entries should still exist (nothing deleted)
	if len(entries) != 5 {
		t.Errorf("expected 5 entries preserved, got %d", len(entries))
	}

	// The patched entry should have empty parentUuid
	for _, e := range entries {
		if e.UUID == "u2" && e.ParentUUID != "" {
			t.Errorf("expected u2.parentUuid=\"\" after patch, got %q", e.ParentUUID)
		}
	}
}

func TestRepair_MissingParentSidechains(t *testing.T) {
	// Multiple sidechains reference the same missing parent UUID.
	// The active chain only walks through one, but PatchParentUUID
	// should patch ALL of them so the fix converges in one pass.
	//
	// Chain structure:
	//   GONE (deleted)
	//    ├── a1 (assistant, sidechain)
	//    ├── a2 (assistant, active chain root) → u1 → a3 → u2
	//    └── a4 (assistant, sidechain)
	lines := []string{
		`{"type":"assistant","uuid":"a1","parentUuid":"GONE","isSidechain":true,"message":{"role":"assistant","content":"side1"}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"GONE","message":{"role":"assistant","content":"root"}}`,
		`{"type":"user","uuid":"u1","parentUuid":"a2","message":{"role":"user","content":"q1"}}`,
		`{"type":"assistant","uuid":"a3","parentUuid":"u1","message":{"role":"assistant","content":"r1"}}`,
		`{"type":"assistant","uuid":"a4","parentUuid":"GONE","isSidechain":true,"message":{"role":"assistant","content":"side2"}}`,
		`{"type":"user","uuid":"u2","parentUuid":"a3","message":{"role":"user","content":"last"}}`,
	}
	path := filepath.Join(t.TempDir(), "sidechains.jsonl")
	var data []byte
	for _, l := range lines {
		data = append(data, []byte(l+"\n")...)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Diagnose: should find missing_parent + bad_chain_start for a2
	entries, _ := jsonl.Parse(path)
	diag := analyzer.Diagnose(entries)
	if len(diag.Issues) == 0 {
		t.Fatal("expected issues")
	}

	// Repair
	result, err := Repair(path, diag.Issues, false)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	// Should patch a1, a2, and a4 (all reference GONE)
	if result.ParentPatches < 3 {
		t.Errorf("expected at least 3 parent patches (all refs to GONE), got %d", result.ParentPatches)
	}

	// After repair + re-diagnosis, no entry should reference GONE
	entries, _ = jsonl.Parse(path)
	for _, e := range entries {
		if e.ParentUUID == "GONE" {
			t.Errorf("entry %s still references GONE after repair", e.UUID)
		}
	}

	// Second repair pass should converge (no issues about GONE)
	diag = analyzer.Diagnose(entries)
	for _, issue := range diag.Issues {
		if issue.Kind == analyzer.IssueChainMissingParent {
			t.Errorf("missing_parent issue persists after repair: %s", issue.Description)
		}
	}
}

func TestRepair_MissingParentWithMalformedLines(t *testing.T) {
	// When malformed lines exist, Parse skips them but ParseRaw keeps them
	// as placeholders. This shifts indices. PatchParentUUID must find
	// entries by their actual parentUuid, not by Parse index.
	lines := []string{
		`{"type":"user","uuid":"u1","parentUuid":"","message":{"role":"user","content":"first"}}`,
		`{MALFORMED LINE}`, // Parse skips, ParseRaw keeps → index shift
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","message":{"role":"assistant","content":"reply"}}`,
		`{ALSO BROKEN}`, // another malformed line
		// u2 references missing parent — Parse index 2, ParseRaw index 4
		`{"type":"user","uuid":"u2","parentUuid":"GONE","message":{"role":"user","content":"second"}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"u2","message":{"role":"assistant","content":"reply2"}}`,
		`{"type":"user","uuid":"u3","parentUuid":"a2","message":{"role":"user","content":"third"}}`,
	}
	path := filepath.Join(t.TempDir(), "malformed.jsonl")
	var data []byte
	for _, l := range lines {
		data = append(data, []byte(l+"\n")...)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Diagnose uses Parse (skips malformed lines)
	entries, _ := jsonl.Parse(path)
	diag := analyzer.Diagnose(entries)
	if len(diag.Issues) == 0 {
		t.Fatal("expected issues")
	}

	// Repair must patch the correct entry despite index mismatch
	result, err := Repair(path, diag.Issues, false)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if result.ParentPatches == 0 {
		t.Error("expected parent patches")
	}

	// Verify convergence: re-diagnose should find no missing_parent
	entries, _ = jsonl.Parse(path)
	diag = analyzer.Diagnose(entries)
	for _, issue := range diag.Issues {
		if issue.Kind == analyzer.IssueChainMissingParent {
			t.Errorf("missing_parent persists after repair: %s", issue.Description)
		}
	}

	// Verify u2's parentUuid was actually patched
	for _, e := range entries {
		if e.UUID == "u2" && e.ParentUUID != "" {
			t.Errorf("u2 still has parentUuid=%q, expected empty", e.ParentUUID)
		}
	}
}

func TestTombstone_Basic(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	beforeCount := len(entries)

	// Tombstone the first user entry (index 0)
	result, err := Tombstone(path, map[int]bool{0: true})
	if err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	if result.EntriesTombstoned != 1 {
		t.Errorf("expected 1 tombstoned, got %d", result.EntriesTombstoned)
	}

	// Entry count should be preserved (tombstone replaces content, doesn't delete)
	entries, err = jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(entries) != beforeCount {
		t.Errorf("expected %d entries (preserved), got %d", beforeCount, len(entries))
	}

	// The tombstoned entry should have placeholder text
	blocks, err := jsonl.ParseContentBlocks(entries[0].Message.Content)
	if err != nil {
		t.Fatalf("parse content: %v", err)
	}
	found := false
	for _, b := range blocks {
		if b.Type == "text" && b.Text == "[removed by contextspectre: orphaned tool result]" {
			found = true
		}
	}
	if !found {
		t.Error("expected tombstone placeholder text in entry content")
	}

	// Backup should exist
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Error("expected backup file after tombstone")
	}
}

func TestTombstone_Empty(t *testing.T) {
	path := copyFixture(t, "small_session.jsonl")
	result, err := Tombstone(path, nil)
	if err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	if result.EntriesTombstoned != 0 {
		t.Errorf("expected 0 tombstoned, got %d", result.EntriesTombstoned)
	}
}
