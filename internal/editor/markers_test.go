package editor

import (
	"path/filepath"
	"testing"
)

func TestMarkerFile_BookmarksRoundTrip(t *testing.T) {
	sessionPath := filepath.Join(t.TempDir(), "session.jsonl")

	mf, err := LoadMarkers(sessionPath)
	if err != nil {
		t.Fatalf("load markers: %v", err)
	}
	if mf.Bookmarks == nil {
		t.Fatal("expected bookmarks map initialized")
	}

	mf.SetBookmark("uuid-1", BookmarkCheckpoint, "pre-release state")
	mf.SetBookmark("uuid-2", BookmarkMilestone, "phase complete")
	if err := SaveMarkers(sessionPath, mf); err != nil {
		t.Fatalf("save markers: %v", err)
	}

	loaded, err := LoadMarkers(sessionPath)
	if err != nil {
		t.Fatalf("reload markers: %v", err)
	}

	b1, ok := loaded.GetBookmark("uuid-1")
	if !ok {
		t.Fatal("expected uuid-1 bookmark")
	}
	if b1.Type != BookmarkCheckpoint || b1.Label != "pre-release state" {
		t.Fatalf("unexpected bookmark 1: %+v", b1)
	}

	b2, ok := loaded.GetBookmark("uuid-2")
	if !ok {
		t.Fatal("expected uuid-2 bookmark")
	}
	if b2.Type != BookmarkMilestone || b2.Label != "phase complete" {
		t.Fatalf("unexpected bookmark 2: %+v", b2)
	}

	loaded.ClearBookmark("uuid-1")
	if _, ok := loaded.GetBookmark("uuid-1"); ok {
		t.Fatal("expected uuid-1 bookmark cleared")
	}
}
