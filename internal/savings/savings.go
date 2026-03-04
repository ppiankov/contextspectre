package savings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Event records a single cleanup savings event.
type Event struct {
	SessionID      string    `json:"session_id"`
	Slug           string    `json:"slug,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
	TokensRemoved  int       `json:"tokens_removed"`
	TurnsRemaining int       `json:"turns_remaining"`
	Model          string    `json:"model"`
	AvoidedTokens  int       `json:"avoided_tokens"`
	AvoidedCost    float64   `json:"avoided_cost"`
}

// Summary aggregates savings across all events.
type Summary struct {
	TotalCleanups  int     `json:"total_cleanups"`
	TotalRemoved   int     `json:"total_tokens_removed"`
	TotalAvoided   int     `json:"total_avoided_tokens"`
	TotalSavedCost float64 `json:"total_saved_cost"`
	AvgPerCleanup  float64 `json:"avg_per_cleanup"`
	RecentEvents   []Event `json:"recent_events,omitempty"`
}

// LogPath returns the path to the savings log within the given claude dir.
func LogPath(claudeDir string) string {
	return filepath.Join(claudeDir, "contextspectre-savings.json")
}

// Load reads all savings events from disk. Returns empty slice if file does not exist.
func Load(claudeDir string) ([]Event, error) {
	data, err := os.ReadFile(LogPath(claudeDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read savings: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("parse savings: %w", err)
	}
	return events, nil
}

// Append adds a new savings event to the log.
func Append(claudeDir string, event Event) error {
	events, err := Load(claudeDir)
	if err != nil {
		// If corrupted, start fresh
		events = nil
	}
	events = append(events, event)

	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal savings: %w", err)
	}
	data = append(data, '\n')

	path := LogPath(claudeDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write savings: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename savings: %w", err)
	}
	return nil
}

// Aggregate computes a summary from all events. Includes up to recentN most recent events.
func Aggregate(events []Event, recentN int) *Summary {
	s := &Summary{}
	s.TotalCleanups = len(events)
	for _, e := range events {
		s.TotalRemoved += e.TokensRemoved
		s.TotalAvoided += e.AvoidedTokens
		s.TotalSavedCost += e.AvoidedCost
	}
	if s.TotalCleanups > 0 {
		s.AvgPerCleanup = s.TotalSavedCost / float64(s.TotalCleanups)
	}

	// Recent events (last N)
	start := len(events) - recentN
	if start < 0 {
		start = 0
	}
	s.RecentEvents = events[start:]
	if s.RecentEvents == nil {
		s.RecentEvents = []Event{}
	}
	return s
}

// ForSessions filters events by session ID and returns per-session summary.
func ForSession(events []Event, sessionID string) *Summary {
	var filtered []Event
	for _, e := range events {
		if e.SessionID == sessionID {
			filtered = append(filtered, e)
		}
	}
	return Aggregate(filtered, 5)
}
