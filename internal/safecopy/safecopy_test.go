package safecopy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	os.WriteFile(path, []byte("original content"), 0644)

	if err := Create(path); err != nil {
		t.Fatalf("create backup: %v", err)
	}

	bakPath := path + ".bak"
	data, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(data) != "original content" {
		t.Errorf("backup content mismatch: %q", string(data))
	}
}

func TestCreate_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	os.WriteFile(path, []byte("content"), 0644)
	os.WriteFile(path+".bak", []byte("old backup"), 0644)

	err := Create(path)
	if err == nil {
		t.Error("expected error when backup already exists")
	}
}

func TestCreate_SourceNotFound(t *testing.T) {
	err := Create("/nonexistent/file.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}

func TestRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	os.WriteFile(path, []byte("modified content"), 0644)
	os.WriteFile(path+".bak", []byte("original content"), 0644)

	if err := Restore(path); err != nil {
		t.Fatalf("restore: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "original content" {
		t.Errorf("restore content mismatch: %q", string(data))
	}

	// Backup should be gone
	if Exists(path) {
		t.Error("expected backup to be removed after restore")
	}
}

func TestRestore_NoBackup(t *testing.T) {
	err := Restore("/nonexistent/file.jsonl")
	if err == nil {
		t.Error("expected error when no backup exists")
	}
}

func TestClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	os.WriteFile(path+".bak", []byte("backup"), 0644)

	if err := Clean(path); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if Exists(path) {
		t.Error("expected backup to be removed")
	}
}

func TestClean_NoBackup(t *testing.T) {
	// Clean should not error if no backup exists
	if err := Clean("/nonexistent/file.jsonl"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	if Exists(path) {
		t.Error("expected no backup to exist")
	}

	os.WriteFile(path+".bak", []byte("x"), 0644)
	if !Exists(path) {
		t.Error("expected backup to exist")
	}
}

func TestCreate_PreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	os.WriteFile(path, []byte("content"), 0600)

	if err := Create(path); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}
