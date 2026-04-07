package jsonl

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// heldMu guards the refcount and fd maps for process-level re-entrancy.
var (
	heldMu sync.Mutex
	held   = map[string]int{}
	heldFd = map[string]*os.File{}
)

// LockFile acquires an exclusive advisory lock on <path>.lock.
// Returns an unlock function that must be called (typically via defer).
//
// Re-entrant within the same process: if the lock is already held for
// the same absolute path, the refcount increments and the returned
// unlock decrements it without releasing the underlying flock.
// This prevents deadlocks when CleanLive acquires the lock and then
// calls StreamStripType or WriteLines which also request it.
func LockFile(path string) (unlock func(), err error) {
	lockPath := path + ".lock"
	absPath, err := filepath.Abs(lockPath)
	if err != nil {
		return nil, fmt.Errorf("abs lock path: %w", err)
	}

	heldMu.Lock()
	if held[absPath] > 0 {
		held[absPath]++
		heldMu.Unlock()
		return func() {
			heldMu.Lock()
			held[absPath]--
			heldMu.Unlock()
		}, nil
	}
	heldMu.Unlock()

	f, err := os.OpenFile(absPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}

	heldMu.Lock()
	held[absPath] = 1
	heldFd[absPath] = f
	heldMu.Unlock()

	return func() {
		heldMu.Lock()
		held[absPath]--
		if held[absPath] == 0 {
			delete(held, absPath)
			delete(heldFd, absPath)
			heldMu.Unlock()
			_ = unlockFile(f)
			_ = f.Close()
		} else {
			heldMu.Unlock()
		}
	}, nil
}
