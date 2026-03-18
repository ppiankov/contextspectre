package commands

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setTempDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("TMP", dir)
		t.Setenv("TEMP", dir)
	} else {
		t.Setenv("TMPDIR", dir)
	}
}

func TestWriteContextPercentCache(t *testing.T) {
	tmp := t.TempDir()
	setTempDir(t, tmp)

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
	setTempDir(t, tmp)

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
