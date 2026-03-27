package editor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestCleanThinking_RemovesBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	// Assistant with a thinking block and a text block.
	lines := []string{
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"let me think...","signature":"sig123"},{"type":"text","text":"Here is my answer"}]}}`,
	}
	writeLines(t, path, lines)

	result, err := CleanThinking(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.ThinkingRemoved != 1 {
		t.Fatalf("expected 1 thinking removed, got %d", result.ThinkingRemoved)
	}

	// Verify the thinking block is gone.
	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	blocks, _ := jsonl.ParseContentBlocks(entries[0].Message.Content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block remaining, got %d", len(blocks))
	}
	if blocks[0].Type != "text" {
		t.Fatalf("expected text block, got %s", blocks[0].Type)
	}
}

func TestCleanThinking_NoThinking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":[{"type":"text","text":"normal response"}]}}`,
	}
	writeLines(t, path, lines)

	result, err := CleanThinking(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.ThinkingRemoved != 0 {
		t.Fatalf("expected 0, got %d", result.ThinkingRemoved)
	}
}

func TestCleanSystemReminders_Dedup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	reminder := "<system-reminder>Task tools haven't been used recently.</system-reminder>"
	block1, _ := json.Marshal([]map[string]string{{"type": "text", "text": "hello"}, {"type": "text", "text": reminder}})
	block2, _ := json.Marshal([]map[string]string{{"type": "text", "text": "world"}, {"type": "text", "text": reminder}})
	block3, _ := json.Marshal([]map[string]string{{"type": "text", "text": "final"}, {"type": "text", "text": reminder}})

	lines := []string{
		`{"type":"user","uuid":"u1","message":{"role":"user","content":` + string(block1) + `}}`,
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":"response"}}`,
		`{"type":"user","uuid":"u2","message":{"role":"user","content":` + string(block2) + `}}`,
		`{"type":"assistant","uuid":"a2","message":{"role":"assistant","content":"response2"}}`,
		`{"type":"user","uuid":"u3","message":{"role":"user","content":` + string(block3) + `}}`,
	}
	writeLines(t, path, lines)

	result, err := CleanSystemReminders(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same reminder appears 3 times, should remove 2 (keep the last).
	if result.RemindersRemoved != 2 {
		t.Fatalf("expected 2 reminders removed, got %d", result.RemindersRemoved)
	}
}

func TestCleanSystemReminders_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"user","uuid":"u1","message":{"role":"user","content":"just a question"}}`,
	}
	writeLines(t, path, lines)

	result, err := CleanSystemReminders(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.RemindersRemoved != 0 {
		t.Fatalf("expected 0, got %d", result.RemindersRemoved)
	}
}

func TestCleanMegaBlocks_Truncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	// Create a text block >100KB.
	bigText := make([]byte, 150*1024)
	for i := range bigText {
		bigText[i] = 'x'
	}
	content, _ := json.Marshal([]map[string]string{{"type": "text", "text": string(bigText)}})

	lines := []string{
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":` + string(content) + `}}`,
	}
	writeLines(t, path, lines)

	result, err := CleanMegaBlocks(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.BlocksTruncated != 1 {
		t.Fatalf("expected 1 block truncated, got %d", result.BlocksTruncated)
	}

	// Verify it's now smaller.
	fi, _ := os.Stat(path)
	if fi.Size() > 110*1024 {
		t.Fatalf("file still too large after truncation: %d", fi.Size())
	}
}

func TestCleanMegaBlocks_NoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	lines := []string{
		`{"type":"assistant","uuid":"a1","message":{"role":"assistant","content":[{"type":"text","text":"small text"}]}}`,
	}
	writeLines(t, path, lines)

	result, err := CleanMegaBlocks(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.BlocksTruncated != 0 {
		t.Fatalf("expected 0, got %d", result.BlocksTruncated)
	}
}

func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
	var data []byte
	for _, l := range lines {
		data = append(data, []byte(l+"\n")...)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
