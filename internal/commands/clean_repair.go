package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// repairAuditEntry is a single line in the repair audit log.
type repairAuditEntry struct {
	Timestamp   string `json:"ts"`
	Session     string `json:"session"`
	Tier        int    `json:"tier"`
	Action      string `json:"action"`
	Count       int    `json:"count"`
	TokensSaved int    `json:"tokens_saved,omitempty"`
}

var repairAuditPath = filepath.Join(os.TempDir(),
	fmt.Sprintf("contextspectre-repairs-%d.jsonl", os.Getpid()))

func logRepairAction(sessionID string, tier int, action string, count, tokensSaved int) {
	entry := repairAuditEntry{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Session:     sessionID,
		Tier:        tier,
		Action:      action,
		Count:       count,
		TokensSaved: tokensSaved,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(repairAuditPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(data, '\n'))
}

// runWatchAutoRepair applies tiered auto-repairs to a session that has been
// quiescent for at least sessionCooldown. Must only be called after the
// mtime cooldown has elapsed in the watch loop.
//
// Returns the count of repair actions applied.
func runWatchAutoRepair(path, sessionID string) int {
	// Non-blocking lock: if Claude Code holds the lock, skip this cycle.
	unlock, ok, err := jsonl.TryLockFile(path)
	if err != nil || !ok {
		return 0
	}
	defer unlock()

	// Re-check mtime under lock (race guard).
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if time.Since(fi.ModTime()) < sessionCooldown {
		return 0 // Claude wrote just before we got the lock
	}

	repaired := 0

	// --- Tier 1: always safe ---

	// 1a. Orphaned tool_use → rewire
	if result, err := editor.Rewire(path); err == nil && result.Injected > 0 {
		logRepairAction(sessionID, 1, "rewire", result.Injected, 0)
		repaired += result.Injected
	}

	// 1b. Coalesce adjacent same-role entries
	if cr, err := editor.Coalesce(path); err == nil && (cr.EntriesRemoved > 0 || cr.OrphansStripped > 0) {
		count := cr.EntriesRemoved + cr.OrphansStripped
		logRepairAction(sessionID, 1, "coalesce", count, 0)
		repaired += count
	}

	// --- Tier 2: low risk, with logging ---

	entries, err := jsonl.Parse(path)
	if err != nil {
		return repaired
	}

	diagnosis := analyzer.Diagnose(entries)
	var tier2Issues []analyzer.Issue
	for _, issue := range diagnosis.Issues {
		switch issue.Kind {
		case analyzer.IssueFilterBlock, analyzer.IssueOversizedImage, analyzer.IssueMediaTypeMismatch:
			tier2Issues = append(tier2Issues, issue)
		}
	}
	if len(tier2Issues) > 0 {
		tombstone := autoTombstone(path)
		if result, err := editor.Repair(path, tier2Issues, tombstone); err == nil {
			count := result.EntriesRemoved + result.EntriesTombstoned + result.ImagesReplaced
			if count > 0 {
				logRepairAction(sessionID, 2, "repair", count, 0)
				repaired += count
			}
		}
		// Re-parse after repair
		entries, err = jsonl.Parse(path)
		if err != nil {
			return repaired
		}
	}

	// Tier 2: sidechains
	sidechainReport := analyzer.DetectSidechains(entries)
	sidechainIdx := analyzer.SidechainIndexSet(sidechainReport)
	if len(sidechainIdx) > 0 {
		if dr, err := editor.Delete(path, sidechainIdx); err == nil && dr.EntriesRemoved > 0 {
			logRepairAction(sessionID, 2, "sidechains", dr.EntriesRemoved, 0)
			repaired += dr.EntriesRemoved
		}
	}

	// --- Tier 3: context >= 70% threshold ---

	stats, err := jsonl.ScanLight(path)
	if err != nil || stats.LastUsage == nil {
		return repaired
	}
	ctxTokens := stats.LastUsage.TotalContextTokens()
	ctxPct := float64(ctxTokens) / float64(analyzer.ContextWindowSize) * 100
	if ctxPct < 70 {
		return repaired
	}

	// Stale reads + failed retries via CleanLive Tier 3
	if result, err := editor.CleanLive(path, editor.CleanLiveOpts{
		Tier3: true,
	}); err == nil {
		count := result.StaleReadsRemoved + result.FailedRetries
		if count > 0 {
			logRepairAction(sessionID, 3, "stale+retry", count, result.TotalTokensSaved)
			repaired += count
		}
	}

	// Chain breaks
	entries, err = jsonl.Parse(path)
	if err != nil {
		return repaired
	}
	diagnosis = analyzer.Diagnose(entries)
	var chainIssues []analyzer.Issue
	for _, issue := range diagnosis.Issues {
		switch issue.Kind {
		case analyzer.IssueChainMissingParent, analyzer.IssueChainBadStart:
			chainIssues = append(chainIssues, issue)
		}
	}
	if len(chainIssues) > 0 {
		tombstone := autoTombstone(path)
		if result, err := editor.Repair(path, chainIssues, tombstone); err == nil {
			count := result.EntriesRemoved + result.ChainRepairs + result.ParentPatches
			if count > 0 {
				logRepairAction(sessionID, 3, "chain-repair", count, 0)
				repaired += count
			}
		}
	}

	return repaired
}
