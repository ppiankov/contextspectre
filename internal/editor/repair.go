package editor

import (
	"fmt"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// RepairResult holds the outcome of a repair operation.
type RepairResult struct {
	IssuesFixed    int
	EntriesRemoved int
	ImagesReplaced int
	ChainRepairs   int
}

// Repair applies fixes for detected issues.
// Creates a backup before modifying. Uses dry-run via Diagnose first.
func Repair(path string, issues []analyzer.Issue) (*RepairResult, error) {
	if len(issues) == 0 {
		return &RepairResult{}, nil
	}

	// Collect entries to delete and images to replace
	toDelete := make(map[int]bool)
	oversizedEntries := make(map[int]bool)

	for _, issue := range issues {
		switch issue.Kind {
		case analyzer.IssueFilterBlock:
			toDelete[issue.EntryIndex] = true
			if issue.RelatedIndex >= 0 {
				toDelete[issue.RelatedIndex] = true
			}
		case analyzer.IssueOrphanedResult:
			toDelete[issue.EntryIndex] = true
		case analyzer.IssueMalformed:
			toDelete[issue.EntryIndex] = true
		case analyzer.IssueOversizedImage:
			oversizedEntries[issue.EntryIndex] = true
		}
	}

	result := &RepairResult{IssuesFixed: len(issues)}

	// Handle oversized images first (modifies content, not deletion)
	if len(oversizedEntries) > 0 {
		imgResult, err := replaceOversizedImages(path, oversizedEntries)
		if err != nil {
			return nil, fmt.Errorf("replace oversized images: %w", err)
		}
		result.ImagesReplaced = imgResult
	}

	// Handle deletions
	if len(toDelete) > 0 {
		delResult, err := Delete(path, toDelete)
		if err != nil {
			return nil, fmt.Errorf("delete entries: %w", err)
		}
		result.EntriesRemoved = delResult.EntriesRemoved
		result.ChainRepairs = delResult.ChainRepairs
	}

	return result, nil
}

// replaceOversizedImages replaces only images exceeding the size threshold.
func replaceOversizedImages(path string, indices map[int]bool) (int, error) {
	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return 0, fmt.Errorf("parse: %w", err)
	}

	replaced := 0
	modified := false

	for i := range entries {
		if !indices[i] {
			continue
		}
		e := entries[i]
		if !e.HasImages() {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		lineModified := false
		for j := range blocks {
			if blocks[j].Type == "image" && blocks[j].Source != nil && len(blocks[j].Source.Data) > analyzer.OversizedImageThreshold {
				blocks[j].Source.Data = TransparentPNG1x1
				replaced++
				lineModified = true
			}
		}

		if lineModified {
			updated, err := reserializeContent(rawLines[i], blocks)
			if err != nil {
				continue
			}
			rawLines[i] = updated
			modified = true
		}
	}

	if !modified {
		return 0, nil
	}

	// Backup before writing
	if err := safecopy.Create(path); err != nil {
		return 0, fmt.Errorf("backup: %w", err)
	}

	if err := jsonl.WriteLines(path, rawLines); err != nil {
		_ = safecopy.Restore(path)
		return 0, fmt.Errorf("write: %w", err)
	}

	return replaced, nil
}
