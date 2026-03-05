package editor

import (
	"encoding/json"
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// ReconnectResult holds reconnect-sidechains outcome.
type ReconnectResult struct {
	EntriesReconnected int
}

// ReconnectSidechains rewrites missing parentUuid references to nearest surviving
// ancestors for sidechain entries classified as repairable.
func ReconnectSidechains(path string, report *analyzer.SidechainReport) (*ReconnectResult, error) {
	if report == nil || len(report.Entries) == 0 {
		return &ReconnectResult{}, nil
	}

	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	uuidSet := make(map[string]bool, len(entries))
	for i := range entries {
		if entries[i].UUID != "" {
			uuidSet[entries[i].UUID] = true
		}
	}

	res := &ReconnectResult{}
	modified := false

	for _, sc := range report.Entries {
		if sc.Classification != "repairable" {
			continue
		}
		if sc.EntryIndex < 0 || sc.EntryIndex >= len(entries) {
			continue
		}

		e := entries[sc.EntryIndex]
		if e.ParentUUID == "" || uuidSet[e.ParentUUID] {
			continue
		}
		if sc.ReconnectParent == "" {
			continue
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(rawLines[sc.EntryIndex], &raw); err != nil {
			continue
		}
		parentJSON, _ := json.Marshal(sc.ReconnectParent)
		raw["parentUuid"] = parentJSON
		updated, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		rawLines[sc.EntryIndex] = updated
		entries[sc.EntryIndex].ParentUUID = sc.ReconnectParent
		res.EntriesReconnected++
		modified = true
	}

	if !modified {
		return res, nil
	}

	if err := safecopy.CreateIfMissing(path); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	if err := jsonl.WriteLines(path, rawLines); err != nil {
		_ = safecopy.Restore(path)
		return nil, fmt.Errorf("write: %w", err)
	}

	return res, nil
}
