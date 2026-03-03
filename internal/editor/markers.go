package editor

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

// MarkerType represents an explicit user intent label for a session entry.
type MarkerType string

const (
	MarkerKeep      MarkerType = "keep"
	MarkerCandidate MarkerType = "candidate"
	MarkerNoise     MarkerType = "noise"
)

// PhaseType represents a reasoning phase label for a session entry.
type PhaseType string

const (
	PhaseExploratory PhaseType = "exploratory"
	PhaseDecision    PhaseType = "decision"
	PhaseOperational PhaseType = "operational"
)

// CommitPoint represents a user-marked decision boundary in a session.
type CommitPoint struct {
	UUID        string    `json:"uuid"`
	Timestamp   time.Time `json:"timestamp"`
	Goal        string    `json:"goal"`
	Decisions   []string  `json:"decisions"`
	Constraints []string  `json:"constraints,omitempty"`
	Questions   []string  `json:"questions,omitempty"`
	Files       []string  `json:"files,omitempty"`
}

// MarkerFile holds persisted markers in a sidecar file alongside a session JSONL.
type MarkerFile struct {
	Version      int                   `json:"version"`
	Markers      map[string]MarkerType `json:"markers"`
	Phases       map[string]PhaseType  `json:"phases,omitempty"`
	CommitPoints []CommitPoint         `json:"commit_points,omitempty"`
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
	if mf.Phases == nil {
		mf.Phases = map[string]PhaseType{}
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

// GetPhase returns the phase for a UUID, or "" if unset.
func (mf *MarkerFile) GetPhase(uuid string) PhaseType {
	if mf == nil || mf.Phases == nil {
		return ""
	}
	return mf.Phases[uuid]
}

// SetPhase assigns a phase to a UUID.
func (mf *MarkerFile) SetPhase(uuid string, phase PhaseType) {
	if mf.Phases == nil {
		mf.Phases = map[string]PhaseType{}
	}
	mf.Phases[uuid] = phase
}

// ClearPhase removes a phase from a UUID.
func (mf *MarkerFile) ClearPhase(uuid string) {
	delete(mf.Phases, uuid)
}

// TogglePhase sets the phase if unset or different, clears it if same.
func (mf *MarkerFile) TogglePhase(uuid string, phase PhaseType) {
	if mf.GetPhase(uuid) == phase {
		mf.ClearPhase(uuid)
	} else {
		mf.SetPhase(uuid, phase)
	}
}

// AddCommitPoint appends a commit point.
func (mf *MarkerFile) AddCommitPoint(cp CommitPoint) {
	mf.CommitPoints = append(mf.CommitPoints, cp)
}

// RemoveCommitPoint removes a commit point by UUID.
func (mf *MarkerFile) RemoveCommitPoint(uuid string) {
	var filtered []CommitPoint
	for _, cp := range mf.CommitPoints {
		if cp.UUID != uuid {
			filtered = append(filtered, cp)
		}
	}
	mf.CommitPoints = filtered
}

// HasCommitPoint returns true if a commit point exists at the given UUID.
func (mf *MarkerFile) HasCommitPoint(uuid string) bool {
	if mf == nil {
		return false
	}
	for _, cp := range mf.CommitPoints {
		if cp.UUID == uuid {
			return true
		}
	}
	return false
}

// GetCommitPoint returns the commit point for a UUID, or nil if not found.
func (mf *MarkerFile) GetCommitPoint(uuid string) *CommitPoint {
	if mf == nil {
		return nil
	}
	for i := range mf.CommitPoints {
		if mf.CommitPoints[i].UUID == uuid {
			return &mf.CommitPoints[i]
		}
	}
	return nil
}
