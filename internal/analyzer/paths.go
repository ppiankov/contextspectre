package analyzer

import "path/filepath"

// NormalizePath returns a lexically cleaned path.
// Resolves .. and . components, collapses double slashes.
// Does not access the filesystem (no symlink resolution).
func NormalizePath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

// isFileWriteTool returns true for tools that mutate file contents.
func isFileWriteTool(name string) bool {
	switch name {
	case "Write", "Edit", "write_file", "edit_file", "WriteFile",
		"NotebookEdit", "MultiEdit":
		return true
	}
	return false
}
