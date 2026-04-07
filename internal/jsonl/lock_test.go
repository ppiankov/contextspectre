package jsonl

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLockExclusion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Two goroutines race to WriteLines on the same file.
	// Each writes 100 lines. The result should be valid (either 100 lines).
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			var lines [][]byte
			for j := 0; j < 100; j++ {
				lines = append(lines, []byte(`{"type":"user","id":"`+string(rune('A'+id))+`"}`))
			}
			if err := WriteLines(path, lines); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("WriteLines error: %v", err)
	}

	// Verify the final file is valid JSONL with exactly 100 lines
	entries, err := Parse(path)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if len(entries) != 100 {
		t.Errorf("expected 100 entries, got %d", len(entries))
	}
}

func TestLockReentrancy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Acquire lock, then acquire again (re-entrant) — must not deadlock
	unlock1, err := LockFile(path)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}

	unlock2, err := LockFile(path)
	if err != nil {
		t.Fatalf("second lock (re-entrant): %v", err)
	}

	// Release inner lock — outer should still hold
	unlock2()

	// Release outer lock
	unlock1()

	// Should be able to lock again after full release
	unlock3, err := LockFile(path)
	if err != nil {
		t.Fatalf("third lock after release: %v", err)
	}
	unlock3()
}

func TestLockRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Acquire and release, then acquire again
	unlock, err := LockFile(path)
	if err != nil {
		t.Fatal(err)
	}
	unlock()

	unlock2, err := LockFile(path)
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	unlock2()
}

func TestLockDifferentPaths(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "a.jsonl")
	path2 := filepath.Join(dir, "b.jsonl")
	for _, p := range []string{path1, path2} {
		if err := os.WriteFile(p, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Locks on different paths should not interfere
	var acquired int64
	var wg sync.WaitGroup

	for _, p := range []string{path1, path2} {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			unlock, err := LockFile(path)
			if err != nil {
				t.Errorf("lock %s: %v", path, err)
				return
			}
			atomic.AddInt64(&acquired, 1)
			unlock()
		}(p)
	}
	wg.Wait()

	if acquired != 2 {
		t.Errorf("expected 2 locks acquired, got %d", acquired)
	}
}
