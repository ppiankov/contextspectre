package jsonl

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteLines writes raw JSONL lines to a file atomically.
// It writes to a temp file first, then renames to the target path.
func WriteLines(path string, lines [][]byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".contextspectre-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Clean up temp file on error
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	for _, line := range lines {
		if _, err := tmp.Write(line); err != nil {
			return fmt.Errorf("write line: %w", err)
		}
		if _, err := tmp.Write([]byte("\n")); err != nil {
			return fmt.Errorf("write newline: %w", err)
		}
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Preserve original file permissions
	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chmod(tmpPath, info.Mode())
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp to target: %w", err)
	}

	success = true
	return nil
}
