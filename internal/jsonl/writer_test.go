package jsonl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLines_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := [][]byte{
		[]byte(`{"type":"user","uuid":"u1"}`),
		[]byte(`{"type":"assistant","uuid":"a1"}`),
	}
	if err := WriteLines(path, lines); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify file contents
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	expected := "{\"type\":\"user\",\"uuid\":\"u1\"}\n{\"type\":\"assistant\",\"uuid\":\"a1\"}\n"
	if string(data) != expected {
		t.Errorf("content mismatch:\ngot:  %q\nwant: %q", string(data), expected)
	}
}

func TestWriteLines_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	// Write initial content
	if err := os.WriteFile(path, []byte("old content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	lines := [][]byte{[]byte(`{"type":"user"}`)}
	if err := WriteLines(path, lines); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\"type\":\"user\"}\n" {
		t.Errorf("expected overwrite, got %q", string(data))
	}
}

func TestWriteLines_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	if err := os.WriteFile(path, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}

	lines := [][]byte{[]byte(`{"new":"data"}`)}
	if err := WriteLines(path, lines); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestWriteLines_EmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")

	if err := WriteLines(path, nil); err != nil {
		t.Fatalf("write empty: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(data))
	}
}

func TestWriteLines_RoundTrip(t *testing.T) {
	// Parse a real fixture, write it back, parse again
	entries, rawLines, err := ParseRaw(testdataPath("small_session.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.jsonl")
	if err := WriteLines(path, rawLines); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries2, _, err := ParseRaw(path)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	if len(entries) != len(entries2) {
		t.Fatalf("entry count mismatch: %d vs %d", len(entries), len(entries2))
	}
	for i := range entries {
		if entries[i].UUID != entries2[i].UUID {
			t.Errorf("entry %d UUID mismatch: %s vs %s", i, entries[i].UUID, entries2[i].UUID)
		}
	}
}
