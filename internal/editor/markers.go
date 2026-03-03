package editor

import (
	"encoding/json"
	"errors"
	"os"
)

// MarkerType represents an explicit user intent label for a session entry.
type MarkerType string

const (
	MarkerKeep      MarkerType = "keep"
	MarkerCandidate MarkerType = "candidate"
	MarkerNoise     MarkerType = "noise"
)

// MarkerFile holds persisted markers in a sidecar file alongside a session JSONL.
type MarkerFile struct {
	Version int                   `json:"version"`
	Markers map[string]MarkerType `json:"markers"`
}

// MarkerPath returns the sidecar file path for a given session JSONL path.
func MarkerPath(sessionPath string) string {
	return sessionPath + ".markers.json"
}

// LoadMarkers reads the sidecar marker file. Returns an empty MarkerFile if missing.
func LoadMarkers(sessionPath string) (*MarkerFile, error) {
	path := MarkerPath(sessionPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &MarkerFile{Version: 1, Markers: map[string]MarkerType{}}, nil
		}
		return nil, err
	}

	var mf MarkerFile
	if err := json.Unmarshal(data, &mf); err != nil {
		// Corrupt file — start fresh
		return &MarkerFile{Version: 1, Markers: map[string]MarkerType{}}, nil
	}
	if mf.Markers == nil {
		mf.Markers = map[string]MarkerType{}
	}
	return &mf, nil
}

// SaveMarkers writes the marker file to disk.
func SaveMarkers(sessionPath string, mf *MarkerFile) error {
	if mf.Version == 0 {
		mf.Version = 1
	}
	data, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(MarkerPath(sessionPath), data, 0o644)
}

// Get returns the marker for a UUID, or "" if unset.
func (mf *MarkerFile) Get(uuid string) MarkerType {
	if mf == nil || mf.Markers == nil {
		return ""
	}
	return mf.Markers[uuid]
}

// Set assigns a marker to a UUID.
func (mf *MarkerFile) Set(uuid string, marker MarkerType) {
	if mf.Markers == nil {
		mf.Markers = map[string]MarkerType{}
	}
	mf.Markers[uuid] = marker
}

// Clear removes a marker from a UUID.
func (mf *MarkerFile) Clear(uuid string) {
	delete(mf.Markers, uuid)
}

// Toggle sets the marker if unset or different, clears it if same.
func (mf *MarkerFile) Toggle(uuid string, marker MarkerType) {
	if mf.Get(uuid) == marker {
		mf.Clear(uuid)
	} else {
		mf.Set(uuid, marker)
	}
}

// IsKeep returns true if the UUID is marked as KEEP.
func (mf *MarkerFile) IsKeep(uuid string) bool {
	return mf.Get(uuid) == MarkerKeep
}

// IsNoise returns true if the UUID is marked as NOISE.
func (mf *MarkerFile) IsNoise(uuid string) bool {
	return mf.Get(uuid) == MarkerNoise
}
