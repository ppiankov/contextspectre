package editor

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// StreamStripResult holds the result of a streaming strip operation.
type StreamStripResult struct {
	LinesRemoved int
}

// StreamStripType removes JSONL lines matching a given type marker without
// full JSON deserialization. It scans line-by-line, checking for the raw byte
// pattern `"type":"<entryType>"` (with and without spaces), and writes
// non-matching lines to a temp file, then atomic-renames.
func StreamStripType(path string, entryType string) (*StreamStripResult, error) {
	// Build both marker variants: with and without space after colon
	markerNoSpace := []byte(fmt.Sprintf(`"type":"%s"`, entryType))
	markerSpace := []byte(fmt.Sprintf(`"type": "%s"`, entryType))

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	// Create temp file in same directory for atomic rename
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".streamstrip-*")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	writer := bufio.NewWriter(tmp)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 10<<20), 10<<20) // 10MB buffer

	removed := 0
	first := true
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) > 0 && (bytes.Contains(raw, markerNoSpace) || bytes.Contains(raw, markerSpace)) {
			removed++
			continue
		}
		if !first {
			if err := writer.WriteByte('\n'); err != nil {
				_ = tmp.Close()
				_ = os.Remove(tmpPath)
				return nil, fmt.Errorf("write newline: %w", err)
			}
		}
		if _, err := writer.Write(raw); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("write line: %w", err)
		}
		first = false
	}
	if err := scanner.Err(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("scan: %w", err)
	}

	if err := writer.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("flush: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("close temp: %w", err)
	}

	if removed == 0 {
		_ = os.Remove(tmpPath)
		return &StreamStripResult{LinesRemoved: 0}, nil
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("rename: %w", err)
	}

	return &StreamStripResult{LinesRemoved: removed}, nil
}
