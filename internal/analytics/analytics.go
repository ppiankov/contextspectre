package analytics

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Snapshot records a point-in-time session analytics snapshot.
type Snapshot struct {
	Timestamp          time.Time      `json:"timestamp"`
	SessionID          string         `json:"session_id"`
	Project            string         `json:"project"`
	Slug               string         `json:"slug,omitempty"`
	Client             string         `json:"client,omitempty"`
	DurationHours      float64        `json:"duration_hours"`
	Turns              int            `json:"turns"`
	Compactions        int            `json:"compactions"`
	ContextPct         float64        `json:"context_pct"`
	SignalGrade        string         `json:"signal_grade"`
	SignalPct          float64        `json:"signal_pct"`
	NoiseTokens        int            `json:"noise_tokens"`
	CostTotal          float64        `json:"cost_total"`
	CostPerTurn        float64        `json:"cost_per_turn"`
	CostVelocityHr     float64        `json:"cost_velocity_hr"`
	ModelPrimary       string         `json:"model_primary"`
	Models             map[string]int `json:"models,omitempty"`
	CleanupTokensSaved int            `json:"cleanup_tokens_saved"`
	CleanupCount       int            `json:"cleanup_count"`
	TopNoise           string         `json:"top_noise,omitempty"`
}

// FilterOpts controls filtering of snapshots.
type FilterOpts struct {
	Since   time.Duration
	Project string
}

// Summary aggregates analytics across multiple snapshots.
type Summary struct {
	Sessions       int            `json:"sessions"`
	TotalCost      float64        `json:"total_cost"`
	AvgCost        float64        `json:"avg_cost"`
	TotalTurns     int            `json:"total_turns"`
	AvgSignalPct   float64        `json:"avg_signal_pct"`
	AvgGrade       string         `json:"avg_grade"`
	TokensSaved    int            `json:"tokens_saved"`
	CleanupCount   int            `json:"cleanup_count"`
	ModelMix       map[string]int `json:"model_mix"`
	AvgCompactions float64        `json:"avg_compactions"`
}

// LogPath returns the path to the analytics log within the given claude dir.
func LogPath(claudeDir string) string {
	return filepath.Join(claudeDir, "contextspectre-analytics.json")
}

// Load reads all analytics snapshots from disk.
func Load(claudeDir string) ([]Snapshot, error) {
	data, err := os.ReadFile(LogPath(claudeDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read analytics: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	var snapshots []Snapshot
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return nil, fmt.Errorf("parse analytics: %w", err)
	}
	return snapshots, nil
}

// Append adds a new snapshot to the analytics log.
func Append(claudeDir string, snap Snapshot) error {
	snapshots, err := Load(claudeDir)
	if err != nil {
		snapshots = nil
	}
	snapshots = append(snapshots, snap)

	data, err := json.MarshalIndent(snapshots, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal analytics: %w", err)
	}
	data = append(data, '\n')

	path := LogPath(claudeDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write analytics: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename analytics: %w", err)
	}
	return nil
}

// Filter returns snapshots matching the given filter options.
func Filter(snapshots []Snapshot, opts FilterOpts) []Snapshot {
	var result []Snapshot
	cutoff := time.Time{}
	if opts.Since > 0 {
		cutoff = time.Now().Add(-opts.Since)
	}

	for _, s := range snapshots {
		if !cutoff.IsZero() && s.Timestamp.Before(cutoff) {
			continue
		}
		if opts.Project != "" && s.Project != opts.Project {
			continue
		}
		result = append(result, s)
	}
	return result
}

// Aggregate computes summary statistics from a set of snapshots.
func Aggregate(snapshots []Snapshot) *Summary {
	s := &Summary{
		ModelMix: make(map[string]int),
	}
	if len(snapshots) == 0 {
		return s
	}

	s.Sessions = len(snapshots)
	totalSignalPct := 0.0
	totalCompactions := 0
	gradeSum := 0.0

	for _, snap := range snapshots {
		s.TotalCost += snap.CostTotal
		s.TotalTurns += snap.Turns
		s.TokensSaved += snap.CleanupTokensSaved
		s.CleanupCount += snap.CleanupCount
		totalSignalPct += snap.SignalPct
		totalCompactions += snap.Compactions

		if snap.ModelPrimary != "" {
			s.ModelMix[snap.ModelPrimary]++
		}
		for model, count := range snap.Models {
			s.ModelMix[model] += count
		}

		gradeSum += gradeToNum(snap.SignalGrade)
	}

	s.AvgCost = s.TotalCost / float64(s.Sessions)
	s.AvgSignalPct = totalSignalPct / float64(s.Sessions)
	s.AvgCompactions = float64(totalCompactions) / float64(s.Sessions)
	s.AvgGrade = numToGrade(gradeSum / float64(s.Sessions))

	return s
}

func gradeToNum(grade string) float64 {
	switch grade {
	case "A":
		return 5
	case "B":
		return 4
	case "C":
		return 3
	case "D":
		return 2
	case "F":
		return 1
	default:
		return 0
	}
}

func numToGrade(n float64) string {
	switch {
	case n >= 4.5:
		return "A"
	case n >= 3.5:
		return "B"
	case n >= 2.5:
		return "C"
	case n >= 1.5:
		return "D"
	default:
		return "F"
	}
}
