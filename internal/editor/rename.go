package editor

import (
	"fmt"
	"os"
	"runtime"
)

// renameOrCopy attempts os.Rename first. On Windows, if the target file is
// locked by another process (e.g. Claude Code has it open), rename fails with
// "Access is denied." In that case, fall back to read src → write dst → remove src.
// This is not atomic but is safe because callers always maintain a backup.
func renameOrCopy(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Only fall back on Windows — on Unix, rename over open files works fine.
	if runtime.GOOS != "windows" {
		return err
	}

	// Fallback: copy contents then remove source.
	data, readErr := os.ReadFile(src)
	if readErr != nil {
		return fmt.Errorf("rename failed: %w; fallback read: %w", err, readErr)
	}

	// Preserve permissions of the destination if it exists.
	perm := os.FileMode(0644)
	if info, statErr := os.Stat(dst); statErr == nil {
		perm = info.Mode().Perm()
	}

	if writeErr := os.WriteFile(dst, data, perm); writeErr != nil {
		return fmt.Errorf("rename failed: %w; fallback write: %w", err, writeErr)
	}

	_ = os.Remove(src)
	return nil
}
