package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// Info holds metadata about a single conversation session.
type Info struct {
	SessionID    string
	FullPath     string
	FirstPrompt  string
	MessageCount int
	Created      time.Time
	Modified     time.Time
	GitBranch    string
	ProjectPath  string
	ProjectName  string
	FileSizeMB   float64
	IsSidechain  bool
	ContextStats *QuickStats
}

// QuickStats holds lightweight context stats for the session browser.
type QuickStats struct {
	ContextTokens        int
	ContextPct           float64
	ImageCount           int
	CompactionCount      int
	LastCompactionBefore int
	LastCompactionAfter  int
	EstimatedCost        float64
	Model                string
	SignalPercent        int // 0-100, estimated signal/noise ratio
}

// IsActive returns true if the session was modified within the last 60 seconds.
func (i Info) IsActive() bool {
	return time.Since(i.Modified) < 60*time.Second
}

// sessionsIndex represents the sessions-index.json file.
type sessionsIndex struct {
	Version      int          `json:"version"`
	Entries      []indexEntry `json:"entries"`
	OriginalPath string       `json:"originalPath"`
}

type indexEntry struct {
	SessionID    string  `json:"sessionId"`
	FullPath     string  `json:"fullPath"`
	FileMtime    float64 `json:"fileMtime"` // unix ms
	FirstPrompt  string  `json:"firstPrompt"`
	MessageCount int     `json:"messageCount"`
	Created      string  `json:"created"`
	Modified     string  `json:"modified"`
	GitBranch    string  `json:"gitBranch"`
	ProjectPath  string  `json:"projectPath"`
	IsSidechain  bool    `json:"isSidechain"`
}

// Discoverer finds sessions across Claude project directories.
type Discoverer struct {
	ClaudeDir string
}

// DefaultClaudeDir returns the default ~/.claude path.
func DefaultClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// ListProjects returns all project directory paths under the claude dir.
func (d *Discoverer) ListProjects() ([]string, error) {
	projectsDir := filepath.Join(d.ClaudeDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("read projects dir: %w", err)
	}

	var projects []string
	for _, e := range entries {
		if e.IsDir() {
			projects = append(projects, filepath.Join(projectsDir, e.Name()))
		}
	}
	return projects, nil
}

// ListSessions returns sessions for a specific project directory.
func (d *Discoverer) ListSessions(projectDir string) ([]Info, error) {
	// Try sessions-index.json first
	indexPath := filepath.Join(projectDir, "sessions-index.json")
	if sessions, err := d.fromIndex(indexPath, projectDir); err == nil && len(sessions) > 0 {
		return sessions, nil
	}

	// Fallback: scan JSONL files
	return d.fromGlob(projectDir)
}

// ListAllSessions returns sessions across all projects, sorted by modification time.
func (d *Discoverer) ListAllSessions() ([]Info, error) {
	projects, err := d.ListProjects()
	if err != nil {
		return nil, err
	}

	var all []Info
	for _, p := range projects {
		sessions, err := d.ListSessions(p)
		if err != nil {
			continue
		}
		all = append(all, sessions...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Modified.After(all[j].Modified)
	})
	return all, nil
}

func (d *Discoverer) fromIndex(indexPath, projectDir string) ([]Info, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}

	var idx sessionsIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse sessions-index: %w", err)
	}

	projectName := projectNameFromDir(projectDir)

	var sessions []Info
	for _, e := range idx.Entries {
		created, _ := time.Parse(time.RFC3339Nano, e.Created)
		modified, _ := time.Parse(time.RFC3339Nano, e.Modified)

		info := Info{
			SessionID:    e.SessionID,
			FullPath:     e.FullPath,
			FirstPrompt:  e.FirstPrompt,
			MessageCount: e.MessageCount,
			Created:      created,
			Modified:     modified,
			GitBranch:    e.GitBranch,
			ProjectPath:  e.ProjectPath,
			ProjectName:  projectName,
			IsSidechain:  e.IsSidechain,
		}

		// Get file size
		if fi, err := os.Stat(e.FullPath); err == nil {
			info.FileSizeMB = float64(fi.Size()) / 1024 / 1024
		}

		// Quick context stats
		if stats, err := jsonl.ScanLight(e.FullPath); err == nil {
			info.ContextStats = &QuickStats{
				ImageCount:           stats.ImageCount,
				CompactionCount:      stats.CompactionCount,
				LastCompactionBefore: stats.LastCompactionBefore,
				LastCompactionAfter:  stats.LastCompactionAfter,
				Model:                stats.Model,
				SignalPercent:        stats.SignalPercent,
			}
			if stats.LastUsage != nil {
				info.ContextStats.ContextTokens = stats.LastUsage.TotalContextTokens()
				info.ContextStats.ContextPct = float64(stats.LastUsage.TotalContextTokens()) / 200000 * 100
			}
			info.ContextStats.EstimatedCost = analyzer.QuickCost(
				stats.TotalInputTokens, stats.TotalOutputTokens,
				stats.TotalCacheWriteTokens, stats.TotalCacheReadTokens,
				stats.Model,
			)
		}

		sessions = append(sessions, info)
	}
	return sessions, nil
}

func (d *Discoverer) fromGlob(projectDir string) ([]Info, error) {
	pattern := filepath.Join(projectDir, "*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}

	projectName := projectNameFromDir(projectDir)

	var sessions []Info
	for _, path := range matches {
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}

		base := filepath.Base(path)
		sessionID := strings.TrimSuffix(base, ".jsonl")

		info := Info{
			SessionID:   sessionID,
			FullPath:    path,
			ProjectName: projectName,
			Modified:    fi.ModTime(),
			FileSizeMB:  float64(fi.Size()) / 1024 / 1024,
		}

		if stats, err := jsonl.ScanLight(path); err == nil {
			info.MessageCount = stats.LineCount
			info.ContextStats = &QuickStats{
				ImageCount:           stats.ImageCount,
				CompactionCount:      stats.CompactionCount,
				LastCompactionBefore: stats.LastCompactionBefore,
				LastCompactionAfter:  stats.LastCompactionAfter,
				Model:                stats.Model,
				SignalPercent:        stats.SignalPercent,
			}
			if stats.LastUsage != nil {
				info.ContextStats.ContextTokens = stats.LastUsage.TotalContextTokens()
				info.ContextStats.ContextPct = float64(stats.LastUsage.TotalContextTokens()) / 200000 * 100
			}
			info.ContextStats.EstimatedCost = analyzer.QuickCost(
				stats.TotalInputTokens, stats.TotalOutputTokens,
				stats.TotalCacheWriteTokens, stats.TotalCacheReadTokens,
				stats.Model,
			)
		}

		sessions = append(sessions, info)
	}
	return sessions, nil
}

func projectNameFromDir(dir string) string {
	name := filepath.Base(dir)
	// Project dirs are URL-encoded paths like "-Users-user-dev-ppiankov-github"
	// Extract the last meaningful segment
	parts := strings.Split(name, "-")
	if len(parts) > 1 {
		// Find the last non-empty part that looks like a project name
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != "" {
				return parts[i]
			}
		}
	}
	return name
}
