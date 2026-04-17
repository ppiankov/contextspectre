//go:build windows

package jsonl

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(f *os.File) error {
	// Lock the first byte range exclusively. The range is arbitrary —
	// advisory locks only need both sides to agree on the same range.
	var ol windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, // reserved
		1, // nNumberOfBytesToLockLow
		0, // nNumberOfBytesToLockHigh
		&ol,
	)
}

func unlockFile(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0, // reserved
		1, // nNumberOfBytesToUnlockLow
		0, // nNumberOfBytesToUnlockHigh
		&ol,
	)
}

// tryLockFile attempts a non-blocking exclusive lock.
// Returns true if the lock was acquired, false if already held by another process.
func tryLockFile(f *os.File) (bool, error) {
	var ol windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &ol,
	)
	if err != nil {
		// ERROR_LOCK_VIOLATION = 33
		if errno, ok := err.(windows.Errno); ok && errno == 33 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
