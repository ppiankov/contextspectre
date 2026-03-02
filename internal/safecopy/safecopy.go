package safecopy

import (
	"fmt"
	"io"
	"os"
)

// Create copies the file at path to path + ".bak".
// Returns error if the backup already exists.
func Create(path string) error {
	bakPath := path + ".bak"
	if _, err := os.Stat(bakPath); err == nil {
		return fmt.Errorf("backup already exists: %s (restore or clean first)", bakPath)
	}
	return copyFile(path, bakPath)
}

// Restore replaces the file at path with the backup file.
func Restore(path string) error {
	bakPath := path + ".bak"
	if _, err := os.Stat(bakPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup found: %s", bakPath)
	}
	return os.Rename(bakPath, path)
}

// Clean removes the backup file.
func Clean(path string) error {
	bakPath := path + ".bak"
	if _, err := os.Stat(bakPath); os.IsNotExist(err) {
		return nil // nothing to clean
	}
	return os.Remove(bakPath)
}

// Exists returns true if a backup file exists.
func Exists(path string) bool {
	_, err := os.Stat(path + ".bak")
	return err == nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = os.Remove(dst) // best-effort cleanup on copy failure
		return fmt.Errorf("copy data: %w", err)
	}

	return dstFile.Sync()
}
