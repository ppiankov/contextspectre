package jsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

const maxLineSize = 10 << 20 // 10MB buffer per line

// Parse reads a JSONL file and returns all entries with computed metadata.
func Parse(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, maxLineSize), maxLineSize)

	var entries []Entry
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			// Skip malformed lines rather than failing the whole file
			continue
		}
		e.LineNumber = lineNum
		e.RawSize = len(raw)
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("scan %s: %w", path, err)
	}
	return entries, nil
}

// ParseRaw reads a JSONL file and returns raw JSON lines alongside entries.
// This preserves the exact original bytes for faithful rewriting.
func ParseRaw(path string) ([]Entry, [][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, maxLineSize), maxLineSize)

	var entries []Entry
	var rawLines [][]byte
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		// Keep a copy of the raw bytes (scanner reuses buffer)
		lineCopy := make([]byte, len(raw))
		copy(lineCopy, raw)

		var e Entry
		if err := json.Unmarshal(lineCopy, &e); err != nil {
			// Keep raw line even if parse fails — preserve file structure
			rawLines = append(rawLines, lineCopy)
			entries = append(entries, Entry{LineNumber: lineNum, RawSize: len(lineCopy)})
			continue
		}
		e.LineNumber = lineNum
		e.RawSize = len(lineCopy)
		entries = append(entries, e)
		rawLines = append(rawLines, lineCopy)
	}
	if err := scanner.Err(); err != nil {
		return entries, rawLines, fmt.Errorf("scan %s: %w", path, err)
	}
	return entries, rawLines, nil
}

// LightStats holds minimal stats extracted without full parsing.
type LightStats struct {
	LineCount             int
	AssistantCount        int
	FileSizeBytes         int64
	TypeCounts            map[MessageType]int
	LastUsage             *Usage
	MaxContext            int
	Slug                  string
	ImageCount            int
	ImageBytesEstimate    int64 // total raw bytes of JSONL entries containing images
	CompactionCount       int
	LastCompactionBefore  int
	LastCompactionAfter   int
	TotalInputTokens      int
	TotalOutputTokens     int
	TotalCacheWriteTokens int
	TotalCacheReadTokens  int
	Model                 string
	SignalPercent         int // 0-100, estimated signal/noise ratio
}

// ScanLight reads a JSONL file extracting only stats-level data.
// It avoids full deserialization of large content fields.
func ScanLight(path string) (*LightStats, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	stats := &LightStats{
		FileSizeBytes: info.Size(),
		TypeCounts:    make(map[MessageType]int),
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, maxLineSize), maxLineSize)

	var prevContextTokens int
	var noiseBytes int

	for scanner.Scan() {
		stats.LineCount++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		stats.TypeCounts[e.Type]++

		if stats.Slug == "" && e.Slug != "" {
			stats.Slug = e.Slug
		}

		// Track noise entries (progress, snapshots) for signal percent
		if e.Type == TypeProgress || e.Type == TypeFileHistorySnapshot {
			noiseBytes += len(raw)
		}

		if e.Type == TypeAssistant {
			stats.AssistantCount++
		}

		if e.Type == TypeAssistant && e.Message != nil && e.Message.Usage != nil {
			stats.LastUsage = e.Message.Usage
			ctx := e.Message.Usage.TotalContextTokens()
			if ctx > stats.MaxContext {
				stats.MaxContext = ctx
			}

			// Accumulate token counts for cost attribution
			u := e.Message.Usage
			stats.TotalInputTokens += u.InputTokens
			stats.TotalOutputTokens += u.OutputTokens
			stats.TotalCacheWriteTokens += u.CacheCreationInputTokens
			stats.TotalCacheReadTokens += u.CacheReadInputTokens

			if stats.Model == "" && e.Message.Model != "" {
				stats.Model = e.Message.Model
			}

			// Detect compaction: large drop in context tokens
			if prevContextTokens > 0 && prevContextTokens-ctx > 50000 {
				stats.CompactionCount++
				stats.LastCompactionBefore = prevContextTokens
				stats.LastCompactionAfter = ctx
			}
			prevContextTokens = ctx
		}

		// Quick image detection via string search (faster than full content parse)
		if e.Type == TypeUser && e.Message != nil {
			if containsImage(raw) {
				stats.ImageCount++
				stats.ImageBytesEstimate += int64(len(raw))
			}
		}
	}
	// Compute signal percent from noise bytes vs total context tokens
	if stats.LastUsage != nil {
		totalTokens := stats.LastUsage.TotalContextTokens()
		noiseTokens := noiseBytes / 4 // rough estimate
		if totalTokens > 0 {
			signal := totalTokens - noiseTokens
			if signal < 0 {
				signal = 0
			}
			stats.SignalPercent = signal * 100 / totalTokens
		} else {
			stats.SignalPercent = 100
		}
	} else {
		stats.SignalPercent = 100
	}

	return stats, scanner.Err()
}

// containsImage is a fast heuristic check for base64 image content.
func containsImage(raw []byte) bool {
	// Look for the image source marker in raw bytes
	return json.Valid(raw) && bytesContains(raw, []byte(`"type":"image"`)) ||
		bytesContains(raw, []byte(`"type": "image"`))
}

func bytesContains(haystack, needle []byte) bool {
	return len(haystack) >= len(needle) && bytesIndex(haystack, needle) >= 0
}

func bytesIndex(haystack, needle []byte) int {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
