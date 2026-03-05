package editor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestReconnectSidechains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reconnect.jsonl")
	lines := [][]byte{
		mustEntryJSON(t, jsonl.Entry{
			Type: jsonl.TypeUser,
			UUID: "u1",
			Message: &jsonl.Message{
				Content: mustContent(t, []jsonl.ContentBlock{{Type: "text", Text: "hello"}}),
			},
		}),
		mustEntryJSON(t, jsonl.Entry{
			Type:       jsonl.TypeAssistant,
			UUID:       "a1",
			ParentUUID: "u1",
			Message: &jsonl.Message{
				Content: mustContent(t, []jsonl.ContentBlock{{Type: "text", Text: "ok"}}),
			},
		}),
		mustEntryJSON(t, jsonl.Entry{
			Type:       jsonl.TypeUser,
			UUID:       "u2",
			ParentUUID: "missing-parent",
			Message: &jsonl.Message{
				Content: mustContent(t, []jsonl.ContentBlock{{Type: "text", Text: "broken"}}),
			},
		}),
	}
	if err := jsonl.WriteLines(path, lines); err != nil {
		t.Fatalf("write lines: %v", err)
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	report := analyzer.DetectSidechains(entries)
	if report.TotalEntries != 1 {
		t.Fatalf("expected 1 sidechain, got %d", report.TotalEntries)
	}

	res, err := ReconnectSidechains(path, report)
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if res.EntriesReconnected != 1 {
		t.Fatalf("expected 1 reconnected entry, got %d", res.EntriesReconnected)
	}

	updated, err := jsonl.Parse(path)
	if err != nil {
		t.Fatalf("parse updated: %v", err)
	}
	if updated[2].ParentUUID != "a1" {
		t.Fatalf("expected parentUuid to be repaired to a1, got %q", updated[2].ParentUUID)
	}

	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("expected backup file: %v", err)
	}
}

func mustEntryJSON(t *testing.T, e jsonl.Entry) []byte {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	return b
}

func mustContent(t *testing.T, blocks []jsonl.ContentBlock) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	return b
}
