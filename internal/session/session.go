package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// Info holds metadata about a single conversation session.
type Info struct {
	SessionID    string
	Slug         string
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
	Zombie       bool
	ZombieReason string
}

// ShortID returns the first 8 characters of the session ID.
func (i Info) ShortID() string {
	if len(i.SessionID) >= 8 {
		return i.SessionID[:8]
	}
	return i.SessionID
}

// DisplayName returns the slug if available, otherwise the short ID.
func (i Info) DisplayName() string {
	if i.Slug != "" {
		return i.Slug
	}
	return i.ShortID()
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
	SignalPercent        int     // 0-100, estimated signal/noise ratio
	ClientType           string  // "cli", "desktop", or "unknown"
	EntropyScore         float64 // 0-100
	EntropyLevel         string  // LOW/MEDIUM/HIGH/CRITICAL
	CleanupStatus        string  // clean/due/overdue
	CleanupCadenceScore  float64 // 0-100
}

func quickStatsFromLight(stats *jsonl.LightStats) *QuickStats {
	clientType := "unknown"
	snapshotCount := stats.TypeCounts[jsonl.TypeFileHistorySnapshot]
	if snapshotCount > 0 {
		clientType = "cli"
	} else if stats.StartsWithQueueOp {
		clientType = "desktop"
	} else if stats.LineCount > 100 {
		clientType = "cli" // cleaned CLI session (snapshots removed)
	}
	return &QuickStats{
		ImageCount:           stats.ImageCount,
		CompactionCount:      stats.CompactionCount,
		LastCompactionBefore: stats.LastCompactionBefore,
		LastCompactionAfter:  stats.LastCompactionAfter,
		Model:                stats.Model,
		SignalPercent:        stats.SignalPercent,
		ClientType:           clientType,
	}
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

	// Cached stats (populated by contextspectre, skips ScanLight when mtime matches)
	CachedSlug          string  `json:"cachedSlug,omitempty"`
	CachedContextTokens int     `json:"cachedContextTokens,omitempty"`
	CachedContextPct    float64 `json:"cachedContextPct,omitempty"`
	CachedImageCount    int     `json:"cachedImageCount,omitempty"`
	CachedCompactions   int     `json:"cachedCompactions,omitempty"`
	CachedModel         string  `json:"cachedModel,omitempty"`
	CachedSignalPct     int     `json:"cachedSignalPct,omitempty"`
	CachedClientType    string  `json:"cachedClientType,omitempty"`
	CachedCost          float64 `json:"cachedCost,omitempty"`
	CachedIsZombie      bool    `json:"cachedIsZombie,omitempty"`
	CachedZombieReason  string  `json:"cachedZombieReason,omitempty"`
	CachedLineCount     int     `json:"cachedLineCount,omitempty"`
	CachedFileSizeMB    float64 `json:"cachedFileSizeMB,omitempty"`
}

// Discoverer finds sessions across Claude project directories.
type Discoverer struct {
	ClaudeDir string
}

// DefaultClaudeDir returns the default ~/.claude path.
// In WSL2, if ~/.claude/projects doesn't exist, falls back to the Windows
// user's .claude directory at /mnt/c/Users/<name>/.claude.
func DefaultClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	claudeDir := filepath.Join(home, ".claude")

	// Check if this directory has projects — if so, use it
	if hasProjects(claudeDir) {
		return claudeDir
	}

	// WSL2 fallback: check Windows .claude directory
	if runtime.GOOS == "linux" {
		if wslDir := detectWSL2ClaudeDir(); wslDir != "" {
			return wslDir
		}
	}

	// Return default even if empty (first-run case)
	return claudeDir
}

// hasProjects returns true if the claude dir has a non-empty projects subdirectory.
func hasProjects(claudeDir string) bool {
	entries, err := os.ReadDir(filepath.Join(claudeDir, "projects"))
	return err == nil && len(entries) > 0
}

// detectWSL2ClaudeDir looks for Claude Code sessions in the Windows filesystem
// mounted at /mnt/c/Users/*/. Returns the first .claude dir with projects.
func detectWSL2ClaudeDir() string {
	// Quick WSL2 check: /mnt/c must exist
	if _, err := os.Stat("/mnt/c"); err != nil {
		return ""
	}
	entries, err := os.ReadDir("/mnt/c/Users")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip system directories
		if name == "Public" || name == "Default" || name == "Default User" || name == "All Users" {
			continue
		}
		candidate := filepath.Join("/mnt/c/Users", name, ".claude")
		if hasProjects(candidate) {
			return candidate
		}
	}
	return ""
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

	projectName := ProjectNameFromDir(projectDir)

	var sessions []Info
	needsRewrite := false
	for i, e := range idx.Entries {
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

		// Check file existence and mtime
		fi, err := os.Stat(e.FullPath)
		if err != nil {
			continue // file gone
		}
		info.FileSizeMB = float64(fi.Size()) / 1024 / 1024

		// Cache hit: mtime matches and we have cached stats
		currentMtime := float64(fi.ModTime().UnixMilli())
		if currentMtime == e.FileMtime && e.CachedClientType != "" {
			info.Slug = e.CachedSlug
			info.MessageCount = e.CachedLineCount
			info.Zombie = e.CachedIsZombie
			info.ZombieReason = e.CachedZombieReason
			info.FileSizeMB = e.CachedFileSizeMB
			info.ContextStats = &QuickStats{
				ContextTokens:   e.CachedContextTokens,
				ContextPct:      e.CachedContextPct,
				ImageCount:      e.CachedImageCount,
				CompactionCount: e.CachedCompactions,
				Model:           e.CachedModel,
				SignalPercent:   e.CachedSignalPct,
				ClientType:      e.CachedClientType,
				EstimatedCost:   e.CachedCost,
			}
			applyEntropyQuickStats(info.ContextStats)
			sessions = append(sessions, info)
			continue
		}

		// Cache miss: ScanLight and update index entry
		if stats, err := jsonl.ScanLight(e.FullPath); err == nil {
			info.Slug = stats.Slug
			info.MessageCount = stats.LineCount
			info.ContextStats = quickStatsFromLight(stats)
			if stats.LastUsage != nil {
				info.ContextStats.ContextTokens = stats.LastUsage.TotalContextTokens()
				info.ContextStats.ContextPct = float64(stats.LastUsage.TotalContextTokens()) / 200000 * 100
				applyEntropyQuickStats(info.ContextStats)
			}
			info.ContextStats.EstimatedCost = analyzer.QuickCost(
				stats.TotalInputTokens, stats.TotalOutputTokens,
				stats.TotalCacheWriteTokens, stats.TotalCacheReadTokens,
				stats.Model,
			)
			zombie := analyzer.DetectZombie(stats)
			info.Zombie = zombie.IsZombie
			info.ZombieReason = zombie.Reason

			// Update cache fields in index entry
			idx.Entries[i].FileMtime = currentMtime
			idx.Entries[i].CachedSlug = info.Slug
			idx.Entries[i].CachedContextTokens = info.ContextStats.ContextTokens
			idx.Entries[i].CachedContextPct = info.ContextStats.ContextPct
			idx.Entries[i].CachedImageCount = info.ContextStats.ImageCount
			idx.Entries[i].CachedCompactions = info.ContextStats.CompactionCount
			idx.Entries[i].CachedModel = info.ContextStats.Model
			idx.Entries[i].CachedSignalPct = info.ContextStats.SignalPercent
			idx.Entries[i].CachedClientType = info.ContextStats.ClientType
			idx.Entries[i].CachedCost = info.ContextStats.EstimatedCost
			idx.Entries[i].CachedIsZombie = info.Zombie
			idx.Entries[i].CachedZombieReason = info.ZombieReason
			idx.Entries[i].CachedLineCount = stats.LineCount
			idx.Entries[i].CachedFileSizeMB = info.FileSizeMB
			needsRewrite = true
		}

		sessions = append(sessions, info)
	}

	// Persist updated cache
	if needsRewrite {
		_ = writeSessionsIndex(indexPath, &idx)
	}

	return sessions, nil
}

func (d *Discoverer) fromGlob(projectDir string) ([]Info, error) {
	pattern := filepath.Join(projectDir, "*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}

	projectName := ProjectNameFromDir(projectDir)

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
			info.Slug = stats.Slug
			info.MessageCount = stats.LineCount
			info.ContextStats = quickStatsFromLight(stats)
			if stats.LastUsage != nil {
				info.ContextStats.ContextTokens = stats.LastUsage.TotalContextTokens()
				info.ContextStats.ContextPct = float64(stats.LastUsage.TotalContextTokens()) / 200000 * 100
				applyEntropyQuickStats(info.ContextStats)
			}
			info.ContextStats.EstimatedCost = analyzer.QuickCost(
				stats.TotalInputTokens, stats.TotalOutputTokens,
				stats.TotalCacheWriteTokens, stats.TotalCacheReadTokens,
				stats.Model,
			)
			zombie := analyzer.DetectZombie(stats)
			info.Zombie = zombie.IsZombie
			info.ZombieReason = zombie.Reason
		}

		sessions = append(sessions, info)
	}
	return sessions, nil
}

// writeSessionsIndex atomically writes the sessions index with cached stats.
func writeSessionsIndex(indexPath string, idx *sessionsIndex) error {
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: temp + rename
	tmpPath := indexPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, indexPath)
}

// ProjectNameFromDir extracts a human-readable project name from a Claude project directory path.
func ProjectNameFromDir(dir string) string {
	name := filepath.Base(dir)
	// Project dirs are URL-encoded paths like "-Users-user-dev-myproject"
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

func applyEntropyQuickStats(stats *QuickStats) {
	if stats == nil || stats.ContextTokens <= 0 {
		return
	}
	grade := analyzer.GradeFromSignalPercent(stats.SignalPercent)
	ratio := analyzer.SignalRatioForGrade(grade)
	entropy := analyzer.CalculateEntropy(analyzer.EntropyInput{
		SignalRatio:     ratio,
		CurrentTokens:   stats.ContextTokens,
		TotalTokens:     stats.ContextTokens,
		CompactionCount: stats.CompactionCount,
	})
	stats.EntropyScore = entropy.Score
	stats.EntropyLevel = string(entropy.Level)

	// Approximate cadence for session list using quick stats only.
	noiseTokens := stats.ContextTokens * (100 - stats.SignalPercent) / 100
	quick := &analyzer.ContextStats{
		CurrentContextTokens: stats.ContextTokens,
		CompactionCount:      stats.CompactionCount,
		Model:                stats.Model,
		TokenGrowthRate:      1200, // list-level default growth estimate
	}
	remaining := analyzer.CompactionThreshold - stats.ContextTokens
	if remaining < 0 {
		quick.EstimatedTurnsLeft = 0
	} else {
		quick.EstimatedTurnsLeft = remaining / 1200
	}
	cadence := analyzer.AssessCleanupCadence(quick, &analyzer.CleanupRecommendation{
		TotalTokens: noiseTokens,
	})
	if cadence != nil {
		stats.CleanupStatus = string(cadence.Status)
		stats.CleanupCadenceScore = cadence.Score
	}
}
