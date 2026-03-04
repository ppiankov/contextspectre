package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	gcProject  string
	gcApply    bool
	gcArchive  bool
	gcMaxAge   int
	gcMaxTurns int
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Identify and clean up stale sessions",
	RunE:  runGC,
}

// gcCandidate is a session identified for garbage collection.
type gcCandidate struct {
	Info    session.Info
	AgeDays int
	Reason  string
}

func runGC(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}

	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if gcProject != "" {
		sessions = resolveProjectSessions(sessions, gcProject, dir)
	}

	// Find candidates
	now := time.Now()
	var candidates []gcCandidate
	var totalBytes int64

	for _, s := range sessions {
		age := int(now.Sub(s.Modified).Hours() / 24)
		if age < gcMaxAge {
			continue
		}

		// Skip if too many messages (not a throwaway)
		if s.MessageCount > gcMaxTurns && gcMaxTurns > 0 {
			continue
		}

		// Skip if has compactions (meaningful work happened)
		if s.ContextStats != nil && s.ContextStats.CompactionCount > 0 {
			continue
		}

		reason := fmt.Sprintf("%dd old, %d msgs", age, s.MessageCount)
		candidates = append(candidates, gcCandidate{
			Info:    s,
			AgeDays: age,
			Reason:  reason,
		})
		totalBytes += int64(s.FileSizeMB * 1024 * 1024)
	}

	if isJSON() {
		return printJSON(buildGCJSON(candidates, len(sessions), totalBytes))
	}

	if len(candidates) == 0 {
		fmt.Printf("No stale sessions found (%d sessions scanned, threshold: %dd old, ≤%d msgs, no compactions).\n",
			len(sessions), gcMaxAge, gcMaxTurns)
		return nil
	}

	fmt.Printf("Found %d stale sessions (%.1f MB) — threshold: %dd old, ≤%d msgs, no compactions\n\n",
		len(candidates), float64(totalBytes)/1024/1024, gcMaxAge, gcMaxTurns)

	for _, c := range candidates {
		slug := c.Info.DisplayName()
		fmt.Printf("  %-30s %-8s  %4d msgs  %.1f MB  %s\n",
			slug, c.Info.ShortID(), c.Info.MessageCount, c.Info.FileSizeMB, c.Reason)
	}
	fmt.Println()

	if !gcApply {
		fmt.Println("Dry run — use --apply to delete, or --apply --archive to move to archive.")
		return nil
	}

	// Apply
	action := "Deleted"
	archiveDir := ""
	if gcArchive {
		archiveDir = filepath.Join(dir, "archive")
		action = "Archived"
	}

	removed := 0
	for _, c := range candidates {
		if gcArchive {
			if err := archiveSession(c.Info.FullPath, archiveDir); err != nil {
				fmt.Fprintf(os.Stderr, "  Error archiving %s: %v\n", c.Info.ShortID(), err)
				continue
			}
		} else {
			if err := os.Remove(c.Info.FullPath); err != nil {
				fmt.Fprintf(os.Stderr, "  Error removing %s: %v\n", c.Info.ShortID(), err)
				continue
			}
		}
		removed++
	}

	fmt.Printf("%s %d sessions (%.1f MB freed).\n", action, removed, float64(totalBytes)/1024/1024)
	return nil
}

func archiveSession(srcPath, archiveDir string) error {
	// Preserve project subdirectory structure
	parts := strings.Split(srcPath, string(filepath.Separator))
	var projectDir string
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			projectDir = parts[i+1]
			break
		}
	}

	destDir := filepath.Join(archiveDir, projectDir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	destPath := filepath.Join(destDir, filepath.Base(srcPath))
	return os.Rename(srcPath, destPath)
}

// GCCandidateJSON is a single GC candidate for JSON output.
type GCCandidateJSON struct {
	SessionID string  `json:"session_id"`
	Slug      string  `json:"slug,omitempty"`
	Project   string  `json:"project"`
	AgeDays   int     `json:"age_days"`
	Messages  int     `json:"messages"`
	SizeMB    float64 `json:"size_mb"`
	Reason    string  `json:"reason"`
}

// GCOutputJSON is the JSON output for the gc command.
type GCOutputJSON struct {
	Candidates      []GCCandidateJSON `json:"candidates"`
	TotalScanned    int               `json:"total_scanned"`
	TotalCandidates int               `json:"total_candidates"`
	TotalBytes      int64             `json:"total_bytes"`
}

func buildGCJSON(candidates []gcCandidate, totalScanned int, totalBytes int64) *GCOutputJSON {
	out := &GCOutputJSON{
		TotalScanned:    totalScanned,
		TotalCandidates: len(candidates),
		TotalBytes:      totalBytes,
	}
	for _, c := range candidates {
		out.Candidates = append(out.Candidates, GCCandidateJSON{
			SessionID: c.Info.SessionID,
			Slug:      c.Info.Slug,
			Project:   c.Info.ProjectName,
			AgeDays:   c.AgeDays,
			Messages:  c.Info.MessageCount,
			SizeMB:    c.Info.FileSizeMB,
			Reason:    c.Reason,
		})
	}
	if out.Candidates == nil {
		out.Candidates = []GCCandidateJSON{}
	}
	return out
}

func init() {
	gcCmd.Flags().StringVar(&gcProject, "project", "", "Filter by project name or alias")
	gcCmd.Flags().BoolVar(&gcApply, "apply", false, "Actually delete/archive stale sessions (default: dry run)")
	gcCmd.Flags().BoolVar(&gcArchive, "archive", false, "Move to ~/.claude/archive/ instead of deleting (requires --apply)")
	gcCmd.Flags().IntVar(&gcMaxAge, "age", 30, "Minimum age in days to be considered stale")
	gcCmd.Flags().IntVar(&gcMaxTurns, "turns", 10, "Maximum turns for a session to be considered stale")
	rootCmd.AddCommand(gcCmd)
}
