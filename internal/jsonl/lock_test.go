package jsonl

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLockExclusion(t *testing.T) {
	// Use a manual temp dir instead of t.TempDir() because on Windows
	// lock file handles may linger briefly after Close(), causing
	// t.TempDir()'s RemoveAll to fail. We retry removal ourselves.
	dir, err := os.MkdirTemp("", "TestLockExclusion")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for i := 0; i < 50; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
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

func TestTryLockFile_NonBlocking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Acquire blocking lock in another goroutine
	unlock, err := LockFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	// TryLockFile from the same process should succeed (re-entrant)
	unlock2, ok, err := TryLockFile(path)
	if err != nil {
		t.Fatalf("TryLockFile error: %v", err)
	}
	if !ok {
		t.Fatal("expected TryLockFile to succeed (re-entrant within same process)")
	}
	unlock2()
}

func TestTryLockFile_ReEntrant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// TryLock, then TryLock again (re-entrant), then unlock both
	unlock1, ok1, err := TryLockFile(path)
	if err != nil || !ok1 {
		t.Fatalf("first TryLockFile: ok=%v err=%v", ok1, err)
	}

	unlock2, ok2, err := TryLockFile(path)
	if err != nil || !ok2 {
		t.Fatalf("second TryLockFile (re-entrant): ok=%v err=%v", ok2, err)
	}

	unlock2()
	unlock1()

	// Should be fully released — lock again to verify
	unlock3, ok3, err := TryLockFile(path)
	if err != nil || !ok3 {
		t.Fatalf("third TryLockFile after release: ok=%v err=%v", ok3, err)
	}
	unlock3()
}
