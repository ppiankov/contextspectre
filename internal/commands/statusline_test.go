package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteContextPercentCache(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	if err := writeContextPercentCache(12345, 54.4); err != nil {
		t.Fatalf("writeContextPercentCache failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmp, "claude-ctx-12345"))
	if err != nil {
		t.Fatalf("read cache file: %v", err)
	}
	if string(got) != "54" {
		t.Fatalf("cache content = %q, want %q", string(got), "54")
	}
}

func TestWriteContextPercentCacheNoPPID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	if err := writeContextPercentCache(0, 99); err != nil {
		t.Fatalf("writeContextPercentCache failed: %v", err)
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files, got %d", len(entries))
	}
}
