package editor

import (
	"encoding/json"
	"fmt"

	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/safecopy"
)

// TransparentPNG1x1 is a 1x1 transparent PNG encoded as base64.
const TransparentPNG1x1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVQI12NgAAIABQABNjN9GQAAAAlwSFlzAAAWJQAAFiUBSVIk8AAAAA1JREFUCNdjYGBg+A8AAQIBAEK0UNsAAAAASUVORK5CYII="

// DeleteResult holds the result of a delete operation.
type DeleteResult struct {
	EntriesRemoved int
	ChainRepairs   int
	BytesBefore    int64
	BytesAfter     int64
}

// Delete removes selected entries from a JSONL file, repairing parentUuid chains.
// Always creates a backup before modifying.
func Delete(path string, toDelete map[int]bool) (*DeleteResult, error) {
	if len(toDelete) == 0 {
		return &DeleteResult{}, nil
	}

	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	// Create backup
	if err := safecopy.Create(path); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}

	result := &DeleteResult{}
	for _, raw := range rawLines {
		result.BytesBefore += int64(len(raw))
	}

	// Build deletion set by UUID
	deleteUUIDs := make(map[string]bool)
	for idx := range toDelete {
		if toDelete[idx] && idx < len(entries) && entries[idx].UUID != "" {
			deleteUUIDs[entries[idx].UUID] = true
		}
	}

	// Build parent remap: deleted UUID → its parent UUID
	parentRemap := make(map[string]string)
	for idx := range toDelete {
		if toDelete[idx] && idx < len(entries) {
			parentRemap[entries[idx].UUID] = entries[idx].ParentUUID
		}
	}

	// Filter and repair
	var newLines [][]byte
	for i, e := range entries {
		if toDelete[i] {
			result.EntriesRemoved++
			continue
		}

		// Repair parentUuid chain
		if deleteUUIDs[e.ParentUUID] {
			newParent := resolveParent(e.ParentUUID, parentRemap)
			if newParent != e.ParentUUID {
				// Re-serialize with updated parentUuid
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(rawLines[i], &raw); err == nil {
					newParentJSON, _ := json.Marshal(newParent)
					raw["parentUuid"] = newParentJSON
					if updated, err := json.Marshal(raw); err == nil {
						newLines = append(newLines, updated)
						result.ChainRepairs++
						continue
					}
				}
			}
		}

		newLines = append(newLines, rawLines[i])
	}

	for _, raw := range newLines {
		result.BytesAfter += int64(len(raw))
	}

	if err := jsonl.WriteLines(path, newLines); err != nil {
		// Try to restore from backup on failure
		safecopy.Restore(path)
		return nil, fmt.Errorf("write: %w", err)
	}

	return result, nil
}

// ReplaceImagesResult holds the result of an image replacement operation.
type ReplaceImagesResult struct {
	ImagesReplaced int
	BytesSaved     int64
}

// ReplaceImages replaces all base64 images with 1x1 transparent PNG placeholders.
// Always creates a backup before modifying.
func ReplaceImages(path string) (*ReplaceImagesResult, error) {
	entries, rawLines, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	result := &ReplaceImagesResult{}
	modified := false

	for i, e := range entries {
		if !e.HasImages() {
			continue
		}

		blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
		if err != nil {
			continue
		}

		lineModified := false
		for j := range blocks {
			if blocks[j].Type == "image" && blocks[j].Source != nil && len(blocks[j].Source.Data) > len(TransparentPNG1x1) {
				saved := len(blocks[j].Source.Data) - len(TransparentPNG1x1)
				result.BytesSaved += int64(saved)
				blocks[j].Source.Data = TransparentPNG1x1
				result.ImagesReplaced++
				lineModified = true
			}
		}

		if lineModified {
			// Re-serialize the content blocks into the raw line
			newContent, err := json.Marshal(blocks)
			if err != nil {
				continue
			}

			var raw map[string]json.RawMessage
			if err := json.Unmarshal(rawLines[i], &raw); err != nil {
				continue
			}

			// Update message.content
			var msg map[string]json.RawMessage
			if err := json.Unmarshal(raw["message"], &msg); err != nil {
				continue
			}
			msg["content"] = newContent
			newMsg, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			raw["message"] = newMsg
			updated, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			rawLines[i] = updated
			modified = true
		}
	}

	if !modified {
		return result, nil
	}

	// Create backup before writing
	if err := safecopy.Create(path); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}

	if err := jsonl.WriteLines(path, rawLines); err != nil {
		safecopy.Restore(path)
		return nil, fmt.Errorf("write: %w", err)
	}

	return result, nil
}

// RemoveProgress removes all progress messages from a JSONL file.
// Always creates a backup before modifying.
func RemoveProgress(path string) (*DeleteResult, error) {
	entries, _, err := jsonl.ParseRaw(path)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	toDelete := make(map[int]bool)
	for i, e := range entries {
		if e.Type == jsonl.TypeProgress {
			toDelete[i] = true
		}
	}

	if len(toDelete) == 0 {
		return &DeleteResult{}, nil
	}

	return Delete(path, toDelete)
}

// resolveParent walks up the deletion chain to find the nearest surviving ancestor.
func resolveParent(uuid string, parentRemap map[string]string) string {
	visited := make(map[string]bool)
	current := uuid
	for {
		newParent, isDeleted := parentRemap[current]
		if !isDeleted {
			return current
		}
		if visited[current] {
			return "" // cycle protection
		}
		visited[current] = true
		current = newParent
	}
}
