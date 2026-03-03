package editor

import (
	"os"
	"path/filepath"
	"testing"

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

	// Verify images were replaced
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	for _, e := range entries {
		if e.HasImages() {
			blocks, _ := jsonl.ParseContentBlocks(e.Message.Content)
			for _, b := range blocks {
				if b.Type == "image" && b.Source != nil {
					if b.Source.Data != TransparentPNG1x1 {
						t.Error("expected image to be replaced with placeholder")
					}
					if b.Source.MediaType != "image/png" {
						t.Errorf("expected media_type image/png after replacement, got %s", b.Source.MediaType)
					}
				}
			}
		}
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
