package commands

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/savings"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	cleanImages        bool
	cleanProgress      bool
	cleanSeparators    bool
	cleanSnapshots     bool
	cleanDedupReads    bool
	cleanTruncate      bool
	cleanOutThreshold  int
	cleanOutKeepLines  int
	cleanFailedRetries bool
	cleanSidechains    bool
	cleanTangents      bool
	cleanAll           bool
	cleanLive          bool
	cleanAggressive    bool
	cleanAuto          bool
	cleanActiveFlag    bool
	cleanActiveSince   string
	cleanWatch         bool
	cleanWatchInterval int
)

var cleanCmd = &cobra.Command{
	Use:   "clean [session-id-or-path]",
	Short: "Clean a session (replace images, remove progress)",
	Long: `Clean a conversation session by replacing base64 images with tiny
placeholders or removing progress messages. Always creates a backup first.

Use --auto to automatically find and clean the most recent session:
  contextspectre clean --auto`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClean,
}

func runClean(cmd *cobra.Command, args []string) error {
	if !cleanImages && !cleanProgress && !cleanSeparators && !cleanSnapshots && !cleanDedupReads && !cleanTruncate && !cleanFailedRetries && !cleanSidechains && !cleanTangents && !cleanAll && !cleanLive && !cleanAuto && !cleanActiveFlag {
		return fmt.Errorf("specify at least one clean operation flag")
	}

	if cleanAggressive && !cleanLive {
		return fmt.Errorf("--aggressive can only be used with --live")
	}

	if cleanWatch && (!cleanActiveFlag || !cleanAll) {
		return fmt.Errorf("--watch requires --active --all")
	}

	if cleanActiveFlag {
		if !cleanAll {
			return fmt.Errorf("--active requires --all (to prevent accidental targeted cleanup)")
		}
		if len(args) > 0 {
			return fmt.Errorf("--active does not accept a session argument")
		}
		if cleanWatch {
			return runCleanActiveWatch()
		}
		return runCleanActive()
	}

	if cleanAuto && len(args) > 0 {
		return fmt.Errorf("--auto does not accept a session argument (it finds the most recent session)")
	}
	if !cleanAuto && !cleanActiveFlag && len(args) == 0 {
		return fmt.Errorf("session argument required (or use --auto to find the most recent session)")
	}

	// --auto: find the most recent session and run --all
	if cleanAuto {
		return runCleanAuto()
	}

	path := resolveSessionPath(args[0])
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", path)
	}

	if cleanLive {
		if cleanAll || cleanImages || cleanProgress || cleanSeparators || cleanSnapshots ||
			cleanDedupReads || cleanTruncate || cleanFailedRetries || cleanSidechains || cleanTangents {
			return fmt.Errorf("--live cannot be combined with --all or individual operation flags")
		}
		if !isJSON() {
			printSessionIdentity(path)
		}
		return runCleanLive(path)
	}

	if !isJSON() {
		printSessionIdentity(path)
	}

	if cleanAll {
		result, err := editor.CleanAll(path)
		if err != nil {
			return fmt.Errorf("clean all: %w", err)
		}
		if isJSON() {
			return printJSON(cleanAllToJSON(path, result))
		}
		fmt.Printf("Cleaned: %d prog, %d snap, %d chain, %d tangent, %d retry, %d stale, %d img, %d sep, %d trunc\n",
			result.ProgressRemoved, result.SnapshotsRemoved, result.SidechainsRemoved,
			result.TangentsRemoved, result.FailedRetries, result.StaleReadsRemoved,
			result.ImagesReplaced, result.SeparatorsStripped, result.OutputsTruncated)
		fmt.Printf("Total saved: ~%d tokens, %s\n",
			result.TotalTokensSaved, formatBytes(result.BytesBefore-result.BytesAfter))
		printSavingsLine(recordCleanupSavings(path, result.TotalTokensSaved))
		slog.Info("Clean all complete", "tokens", result.TotalTokensSaved)
		return nil
	}

	if cleanImages {
		result, err := editor.ReplaceImages(path)
		if err != nil {
			return fmt.Errorf("replace images: %w", err)
		}
		if result.ImagesReplaced > 0 {
			fmt.Printf("Replaced %d images, saved %s\n",
				result.ImagesReplaced,
				formatBytes(result.BytesSaved))
			slog.Info("Images replaced", "count", result.ImagesReplaced, "saved", result.BytesSaved)
		} else {
			fmt.Println("No images to replace.")
		}
	}

	if cleanProgress {
		result, err := editor.RemoveProgress(path)
		if err != nil {
			return fmt.Errorf("remove progress: %w", err)
		}
		if result.EntriesRemoved > 0 {
			fmt.Printf("Removed %d progress messages\n", result.EntriesRemoved)
			slog.Info("Progress removed", "count", result.EntriesRemoved)
		} else {
			fmt.Println("No progress messages to remove.")
		}
	}

	if cleanSeparators {
		result, err := editor.StripSeparators(path)
		if err != nil {
			return fmt.Errorf("strip separators: %w", err)
		}
		if result.LinesStripped > 0 {
			fmt.Printf("Stripped %d separator lines from %d messages, saved ~%d tokens\n",
				result.LinesStripped, result.MessagesModified, result.CharsSaved/4)
			slog.Info("Separators stripped", "lines", result.LinesStripped, "messages", result.MessagesModified)
		} else {
			fmt.Println("No decorative separators found.")
		}
	}

	if cleanSnapshots {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for snapshots: %w", err)
		}
		toDelete := make(map[int]bool)
		for i, e := range entries {
			if e.Type == jsonl.TypeFileHistorySnapshot {
				toDelete[i] = true
			}
		}
		if len(toDelete) == 0 {
			fmt.Println("No file-history-snapshot entries found.")
		} else {
			result, err := editor.Delete(path, toDelete)
			if err != nil {
				return fmt.Errorf("remove snapshots: %w", err)
			}
			fmt.Printf("Removed %d snapshot entries, saved %s\n",
				result.EntriesRemoved,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Snapshots removed", "count", result.EntriesRemoved)
		}
	}

	if cleanDedupReads {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for dedup: %w", err)
		}
		dupResult := analyzer.FindDuplicateReads(entries)
		if len(dupResult.Groups) == 0 {
			fmt.Println("No duplicate file reads found.")
		} else {
			result, err := editor.DeduplicateReads(path, dupResult)
			if err != nil {
				return fmt.Errorf("dedup reads: %w", err)
			}
			fmt.Printf("Removed %d stale file reads across %d files, saved %s\n",
				result.StaleReadsRemoved, dupResult.UniqueFiles,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Dedup reads", "stale", result.StaleReadsRemoved, "files", dupResult.UniqueFiles)
		}
	}

	if cleanTruncate {
		result, err := editor.TruncateOutputs(path, cleanOutThreshold, cleanOutKeepLines)
		if err != nil {
			return fmt.Errorf("truncate outputs: %w", err)
		}
		if result.OutputsTruncated > 0 {
			fmt.Printf("Truncated %d outputs, saved ~%d tokens (kept first/last %d lines)\n",
				result.OutputsTruncated, result.TokensSaved, cleanOutKeepLines)
			slog.Info("Outputs truncated", "count", result.OutputsTruncated, "tokens", result.TokensSaved)
		} else {
			fmt.Println("No large outputs to truncate.")
		}
	}

	if cleanFailedRetries {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for retries: %w", err)
		}
		retryResult := analyzer.FindFailedRetries(entries)
		if len(retryResult.Sequences) == 0 {
			fmt.Println("No failed retries found.")
		} else {
			result, err := editor.RemoveFailedRetries(path, retryResult)
			if err != nil {
				return fmt.Errorf("remove retries: %w", err)
			}
			fmt.Printf("Removed %d failed attempts, saved %s\n",
				result.FailedRemoved,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Failed retries removed", "count", result.FailedRemoved)
		}
	}

	if cleanSidechains {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for sidechains: %w", err)
		}
		toDelete := make(map[int]bool)
		for i, e := range entries {
			if e.IsSidechain {
				toDelete[i] = true
			}
		}
		if len(toDelete) == 0 {
			fmt.Println("No sidechain entries found.")
		} else {
			result, err := editor.Delete(path, toDelete)
			if err != nil {
				return fmt.Errorf("remove sidechains: %w", err)
			}
			fmt.Printf("Removed %d sidechain entries, saved %s\n",
				result.EntriesRemoved,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Sidechains removed", "count", result.EntriesRemoved)
		}
	}

	if cleanTangents {
		entries, err := jsonl.Parse(path)
		if err != nil {
			return fmt.Errorf("parse for tangents: %w", err)
		}
		tangentResult := analyzer.FindTangents(entries)
		if len(tangentResult.Groups) == 0 {
			fmt.Println("No cross-repo tangents found.")
		} else {
			toDelete := tangentResult.AllTangentIndices()
			result, err := editor.Delete(path, toDelete)
			if err != nil {
				return fmt.Errorf("remove tangents: %w", err)
			}
			fmt.Printf("Removed %d tangent entries across %d groups referencing %d external repos, saved %s\n",
				result.EntriesRemoved, len(tangentResult.Groups), tangentResult.ExternalDirs,
				formatBytes(result.BytesBefore-result.BytesAfter))
			slog.Info("Tangents removed", "entries", result.EntriesRemoved, "groups", len(tangentResult.Groups))
		}
	}

	return nil
}

func runCleanAuto() error {
	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("discover sessions: %w", err)
	}
	if len(sessions) == 0 {
		if isJSON() {
			return printJSON(map[string]string{"status": "no_sessions"})
		}
		fmt.Println("No sessions found.")
		return nil
	}

	// Most recent session (already sorted by mtime desc)
	target := sessions[0]
	path := target.FullPath
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session file not found: %s", path)
	}

	if !isJSON() {
		printSessionIdentity(path)
	}

	result, err := editor.CleanAll(path)
	if err != nil {
		return fmt.Errorf("clean auto: %w", err)
	}

	if isJSON() {
		out := cleanAllToJSON(path, result)
		out.Mode = "auto"
		return printJSON(out)
	}

	totalOps := result.ProgressRemoved + result.SnapshotsRemoved + result.SidechainsRemoved +
		result.TangentsRemoved + result.FailedRetries + result.StaleReadsRemoved +
		result.ImagesReplaced + result.SeparatorsStripped + result.OutputsTruncated
	if totalOps == 0 {
		fmt.Printf("Session %s (%s): nothing to clean\n", target.SessionID, target.ProjectName)
		return nil
	}

	fmt.Printf("Auto-cleaned session %s (%s): %d entries removed, ~%d tokens saved, %s\n",
		target.SessionID, target.ProjectName,
		result.ProgressRemoved+result.SnapshotsRemoved+result.SidechainsRemoved+
			result.TangentsRemoved+result.FailedRetries+result.StaleReadsRemoved,
		result.TotalTokensSaved,
		formatBytes(result.BytesBefore-result.BytesAfter))
	printSavingsLine(recordCleanupSavings(path, result.TotalTokensSaved))
	slog.Info("Clean auto complete", "session", target.SessionID, "project", target.ProjectName, "tokens", result.TotalTokensSaved)
	return nil
}

func runCleanActive() error {
	sinceDuration, err := time.ParseDuration(cleanActiveSince)
	if err != nil {
		return fmt.Errorf("invalid --since value %q: %w", cleanActiveSince, err)
	}

	active, err := discoverActiveSessions(sinceDuration)
	if err != nil {
		return err
	}

	if len(active) == 0 {
		if isJSON() {
			return printJSON(CleanActiveOutput{Sessions: []CleanActiveSessionJSON{}})
		}
		fmt.Println("No active sessions to clean")
		return nil
	}

	if !isJSON() {
		fmt.Printf("Cleaning %d active sessions...\n", len(active))
	}

	results, totalTokens, cleaned := cleanActiveSessions(active)

	if isJSON() {
		return printJSON(CleanActiveOutput{
			Sessions:    results,
			Total:       len(active),
			Cleaned:     cleaned,
			TotalTokens: totalTokens,
		})
	}

	if cleaned > 0 {
		fmt.Printf("Total: %s tokens saved across %d sessions\n",
			formatTokens(totalTokens), cleaned)
	}

	// Record analytics snapshots for cleaned sessions.
	for _, s := range active {
		recordAnalyticsSnapshot(s.FullPath)
	}

	return nil
}

func runCleanActiveWatch() error {
	sinceDuration, err := time.ParseDuration(cleanActiveSince)
	if err != nil {
		return fmt.Errorf("invalid --since value %q: %w", cleanActiveSince, err)
	}

	// If --interval is explicitly set, use fixed-interval mode.
	// Otherwise, use smart mtime-based polling.
	if cleanWatchInterval > 0 {
		return runFixedIntervalWatch(sinceDuration)
	}
	return runSmartWatch(sinceDuration)
}

// runFixedIntervalWatch uses a fixed ticker interval (legacy behavior).
func runFixedIntervalWatch(sinceDuration time.Duration) error {
	interval := time.Duration(cleanWatchInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	startTime := time.Now()
	fmt.Printf("Watching active sessions (interval: %ds, Ctrl+C to quit)\n", cleanWatchInterval)

	cumulativeTokens := 0
	cumulativeSessions := 0
	cycles := 0

	// Run first cycle immediately.
	ct, cs := runCleanActiveCycleDiscover(sinceDuration)
	cumulativeTokens += ct
	if cs > 0 {
		cumulativeSessions += cs
	}
	cycles++
	printCumulative(cumulativeTokens, cycles, startTime)

	for {
		select {
		case <-ticker.C:
			ct, cs := runCleanActiveCycleDiscover(sinceDuration)
			cumulativeTokens += ct
			if cs > 0 {
				cumulativeSessions += cs
			}
			cycles++
			printCumulative(cumulativeTokens, cycles, startTime)
		case <-sigCh:
			elapsed := time.Since(startTime)
			fmt.Printf("\nStopped. %d cycles, %s tokens saved across %d sessions in %s.\n",
				cycles, formatTokens(cumulativeTokens), cumulativeSessions,
				formatDuration(elapsed))
			recordWatchSnapshots(sinceDuration)
			return nil
		}
	}
}

const smartPollInterval = 5 * time.Second
const sessionCooldown = 30 * time.Second

// runSmartWatch polls session mtimes every 5s and only cleans changed sessions.
func runSmartWatch(sinceDuration time.Duration) error {
	ticker := time.NewTicker(smartPollInterval)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	startTime := time.Now()
	fmt.Printf("Watching active sessions (smart mode, Ctrl+C to quit)\n")

	lastMtime := make(map[string]time.Time) // path → last known mtime
	lastClean := make(map[string]time.Time) // path → last cleanup time
	cumulativeTokens := 0
	cumulativeSessions := 0
	cycles := 0

	// Run first cycle immediately on all active sessions.
	ct, cs := runCleanActiveCycleDiscover(sinceDuration)
	cumulativeTokens += ct
	if cs > 0 {
		cumulativeSessions += cs
	}
	cycles++
	// Seed mtime map from current active sessions.
	seedMtimeMap(sinceDuration, lastMtime, lastClean)
	printCumulative(cumulativeTokens, cycles, startTime)

	for {
		select {
		case <-ticker.C:
			changed := findChangedSessions(sinceDuration, lastMtime, lastClean)
			if len(changed) == 0 {
				continue
			}

			ts := time.Now().Format("15:04:05")
			fmt.Printf("[%s] Cleaning %d changed sessions...\n", ts, len(changed))
			_, totalTokens, cleaned := cleanActiveSessions(changed)

			// Update maps.
			now := time.Now()
			for _, s := range changed {
				if fi, err := os.Stat(s.FullPath); err == nil {
					lastMtime[s.FullPath] = fi.ModTime()
				}
				lastClean[s.FullPath] = now
			}

			cumulativeTokens += totalTokens
			if cleaned > 0 {
				cumulativeSessions += cleaned
				fmt.Printf("[%s] %s tokens saved across %d sessions\n",
					ts, formatTokens(totalTokens), cleaned)
			} else {
				fmt.Printf("[%s] All clean\n", ts)
			}
			cycles++
			printCumulative(cumulativeTokens, cycles, startTime)
		case <-sigCh:
			elapsed := time.Since(startTime)
			fmt.Printf("\nStopped. %d cycles, %s tokens saved across %d sessions in %s.\n",
				cycles, formatTokens(cumulativeTokens), cumulativeSessions,
				formatDuration(elapsed))
			recordWatchSnapshots(sinceDuration)
			return nil
		}
	}
}

// seedMtimeMap populates the mtime tracking map from current active sessions.
func seedMtimeMap(sinceDuration time.Duration, lastMtime, lastClean map[string]time.Time) {
	active, err := discoverActiveSessions(sinceDuration)
	if err != nil {
		return
	}
	now := time.Now()
	for _, s := range active {
		lastMtime[s.FullPath] = s.Modified
		lastClean[s.FullPath] = now
	}
}

// findChangedSessions returns active sessions whose mtime changed since last check
// and whose cooldown period has expired.
func findChangedSessions(sinceDuration time.Duration, lastMtime, lastClean map[string]time.Time) []session.Info {
	active, err := discoverActiveSessions(sinceDuration)
	if err != nil {
		slog.Warn("Discovery failed", "error", err)
		return nil
	}

	now := time.Now()
	var changed []session.Info
	for _, s := range active {
		prevMtime, seen := lastMtime[s.FullPath]
		if !seen || s.Modified.After(prevMtime) {
			// Check cooldown.
			if lc, ok := lastClean[s.FullPath]; ok && now.Sub(lc) < sessionCooldown {
				continue
			}
			changed = append(changed, s)
		}
	}
	return changed
}

// printCumulative prints the inline cumulative stats if there are any savings.
func printCumulative(totalTokens, cycles int, startTime time.Time) {
	if totalTokens <= 0 {
		return
	}
	elapsed := time.Since(startTime)
	fmt.Printf("           Cumulative: %s tokens saved (%d cycles, %s)\n",
		formatTokens(totalTokens), cycles, formatDuration(elapsed))
}

// recordWatchSnapshots records analytics snapshots for all active sessions on watch exit.
func recordWatchSnapshots(sinceDuration time.Duration) {
	active, err := discoverActiveSessions(sinceDuration)
	if err != nil {
		return
	}
	for _, s := range active {
		recordAnalyticsSnapshot(s.FullPath)
	}
}

// runCleanActiveCycleDiscover discovers active sessions and cleans them.
// Returns tokens saved and sessions cleaned.
func runCleanActiveCycleDiscover(sinceDuration time.Duration) (int, int) {
	active, err := discoverActiveSessions(sinceDuration)
	if err != nil {
		slog.Warn("Discovery failed", "error", err)
		return 0, 0
	}

	ts := time.Now().Format("15:04:05")

	if len(active) == 0 {
		fmt.Printf("[%s] No active sessions\n", ts)
		return 0, 0
	}

	fmt.Printf("[%s] Cleaning %d active sessions...\n", ts, len(active))
	_, totalTokens, cleaned := cleanActiveSessions(active)

	if cleaned == 0 {
		fmt.Printf("[%s] All clean\n", ts)
	} else {
		fmt.Printf("[%s] %s tokens saved across %d sessions\n",
			ts, formatTokens(totalTokens), cleaned)
	}

	return totalTokens, cleaned
}

// discoverActiveSessions returns sessions modified within the given duration.
func discoverActiveSessions(sinceDuration time.Duration) ([]session.Info, error) {
	d := &session.Discoverer{ClaudeDir: resolveClaudeDir()}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return nil, fmt.Errorf("discover sessions: %w", err)
	}

	cutoff := time.Now().Add(-sinceDuration)
	var active []session.Info
	for _, s := range sessions {
		if s.Modified.After(cutoff) {
			active = append(active, s)
		}
	}
	return active, nil
}

// cleanActiveSessions cleans a set of sessions, printing per-session output.
// Returns JSON results, total tokens saved, and count of sessions that had work.
func cleanActiveSessions(active []session.Info) ([]CleanActiveSessionJSON, int, int) {
	var results []CleanActiveSessionJSON
	totalTokens := 0
	cleaned := 0

	for _, s := range active {
		path := s.FullPath
		result, err := editor.CleanAll(path)
		if err != nil {
			slog.Warn("Failed to clean session", "session", s.SessionID, "error", err)
			continue
		}

		totalOps := result.ProgressRemoved + result.SnapshotsRemoved + result.SidechainsRemoved +
			result.TangentsRemoved + result.FailedRetries + result.StaleReadsRemoved +
			result.ImagesReplaced + result.SeparatorsStripped + result.OutputsTruncated

		shortID := s.SessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		proj := s.ProjectName
		savingsEvent := recordCleanupSavings(path, result.TotalTokensSaved)

		sessionResult := CleanActiveSessionJSON{
			ID:      s.SessionID,
			Slug:    s.Slug,
			Project: proj,
		}

		if totalOps == 0 {
			if !isJSON() {
				fmt.Printf("  %s (%s): clean\n", proj, shortID)
			}
		} else {
			cleaned++
			totalTokens += result.TotalTokensSaved

			sessionResult.ProgressRemoved = result.ProgressRemoved
			sessionResult.SnapshotsRemoved = result.SnapshotsRemoved
			sessionResult.StaleReadsRemoved = result.StaleReadsRemoved
			sessionResult.TokensSaved = result.TotalTokensSaved
			sessionResult.BytesSaved = result.BytesBefore - result.BytesAfter

			if savingsEvent != nil {
				sessionResult.AvoidedCost = savingsEvent.AvoidedCost
			}

			if !isJSON() {
				parts := []string{}
				if result.ProgressRemoved > 0 {
					parts = append(parts, fmt.Sprintf("%d prog", result.ProgressRemoved))
				}
				if result.SnapshotsRemoved > 0 {
					parts = append(parts, fmt.Sprintf("%d snap", result.SnapshotsRemoved))
				}
				if result.SidechainsRemoved > 0 {
					parts = append(parts, fmt.Sprintf("%d chain", result.SidechainsRemoved))
				}
				if result.TangentsRemoved > 0 {
					parts = append(parts, fmt.Sprintf("%d tangent", result.TangentsRemoved))
				}
				if result.FailedRetries > 0 {
					parts = append(parts, fmt.Sprintf("%d retry", result.FailedRetries))
				}
				if result.StaleReadsRemoved > 0 {
					parts = append(parts, fmt.Sprintf("%d stale", result.StaleReadsRemoved))
				}
				if result.ImagesReplaced > 0 {
					parts = append(parts, fmt.Sprintf("%d img", result.ImagesReplaced))
				}
				if result.SeparatorsStripped > 0 {
					parts = append(parts, fmt.Sprintf("%d sep", result.SeparatorsStripped))
				}
				if result.OutputsTruncated > 0 {
					parts = append(parts, fmt.Sprintf("%d trunc", result.OutputsTruncated))
				}
				costStr := ""
				if savingsEvent != nil {
					costStr = fmt.Sprintf(" (~%s)", analyzer.FormatCost(savingsEvent.AvoidedCost))
				}
				fmt.Printf("  %s (%s): %s → %s tokens saved%s\n",
					proj, shortID, strings.Join(parts, ", "),
					formatTokens(result.TotalTokensSaved), costStr)
			}
		}

		results = append(results, sessionResult)
	}

	return results, totalTokens, cleaned
}

func runCleanLive(path string) error {
	opts := editor.CleanLiveOpts{
		Aggressive: cleanAggressive,
	}
	result, err := editor.CleanLive(path, opts)
	if err != nil {
		if errors.Is(err, editor.ErrRaceDetected) {
			return fmt.Errorf("aborted: Claude Code wrote to session during cleanup (file restored from backup)")
		}
		if errors.Is(err, editor.ErrSessionNotIdle) {
			return fmt.Errorf("session is actively being written to — wait a few seconds and retry")
		}
		return fmt.Errorf("clean live: %w", err)
	}

	if isJSON() {
		return printJSON(cleanLiveToJSON(path, result))
	}

	fmt.Printf("Live cleaned: %d prog, %d snap",
		result.ProgressRemoved, result.SnapshotsRemoved)
	if cleanAggressive {
		fmt.Printf(", %d img, %d sep, %d trunc",
			result.ImagesReplaced, result.SeparatorsStripped, result.OutputsTruncated)
	}
	fmt.Println()
	fmt.Printf("Total saved: ~%d tokens, %s\n",
		result.TotalTokensSaved, formatBytes(result.BytesBefore-result.BytesAfter))
	printSavingsLine(recordCleanupSavings(path, result.TotalTokensSaved))
	slog.Info("Clean live complete", "tokens", result.TotalTokensSaved, "aggressive", opts.Aggressive)
	return nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/1024/1024)
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// printSessionIdentity prints a one-line identity summary before destructive operations.
func printSessionIdentity(path string) {
	fi, err := os.Stat(path)
	if err != nil {
		return
	}

	slug := "—"
	msgs := 0
	if stats, err := jsonl.ScanLight(path); err == nil {
		if stats.Slug != "" {
			slug = stats.Slug
		}
		msgs = stats.LineCount
	}

	base := filepath.Base(path)
	sessionID := strings.TrimSuffix(base, ".jsonl")
	shortID := sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	// Derive project name from parent directory
	project := session.ProjectNameFromDir(filepath.Dir(path))

	size := float64(fi.Size()) / 1024 / 1024
	mod := timeAgo(fi.ModTime())

	fmt.Printf("Cleaning: %s (%s) | %s | %d msgs | %.1f MB | modified %s\n",
		slug, shortID, project, msgs, size, mod)
}

// recordCleanupSavings computes and optionally records compounding savings from a cleanup.
// Returns the savings event for display purposes.
//
// tokensSaved is the raw byte-diff/4 from the editor. This function corrects it
// for image inflation (base64 bytes counted at /4 instead of /750) using the
// backup file, and uses assistant turn count + compaction threshold for turns remaining.
func recordCleanupSavings(path string, tokensSaved int) *savings.Event {
	if tokensSaved <= 0 {
		return nil
	}

	postStats, err := jsonl.ScanLight(path)
	if err != nil {
		return nil
	}

	// Correct tokensSaved for image inflation using backup file.
	// The editor's (BytesBefore-BytesAfter)/4 counts base64 image data as text tokens.
	// Images should use bytes/750 (same as Recommend() in recommend.go), not bytes/4.
	bakPath := path + ".bak"
	if preStats, bakErr := jsonl.ScanLight(bakPath); bakErr == nil {
		imageBytesRemoved := preStats.ImageBytesEstimate - postStats.ImageBytesEstimate
		if imageBytesRemoved > 0 {
			totalBytesRemoved := preStats.FileSizeBytes - postStats.FileSizeBytes
			nonImageBytesRemoved := totalBytesRemoved - imageBytesRemoved
			if nonImageBytesRemoved < 0 {
				nonImageBytesRemoved = 0
			}
			// Text noise: bytes/4. Images: bytes/750 (conservative, matches analyzer).
			tokensSaved = int(nonImageBytesRemoved)/4 + int(imageBytesRemoved)/750
		}
	}

	if tokensSaved <= 0 {
		return nil
	}

	// Compute turns remaining using assistant turn count (not total line count)
	// and cap at compaction threshold (not full context window).
	currentTokens := 0
	if postStats.LastUsage != nil {
		currentTokens = postStats.LastUsage.TotalContextTokens()
	}

	// Use epoch assistant count (turns since last compaction) for accurate growth rate.
	// Fall back to lifetime AssistantCount for sessions that haven't compacted.
	assistantTurns := postStats.EpochAssistantCount
	if assistantTurns == 0 {
		assistantTurns = postStats.AssistantCount
	}

	turnsRemaining := 0
	if assistantTurns > 0 && currentTokens > 0 {
		avgPerTurn := currentTokens / assistantTurns
		if avgPerTurn > 0 {
			remaining := analyzer.CompactionThreshold - currentTokens
			if remaining > 0 {
				turnsRemaining = remaining / avgPerTurn
			}
		}
	}

	if turnsRemaining <= 0 {
		return nil
	}

	// Compute avoided cost
	pricing := analyzer.PricingForModel(postStats.Model)
	avoidedTokens := tokensSaved * turnsRemaining
	avoidedCost := float64(avoidedTokens) / 1_000_000 * pricing.CacheReadPerMillion

	// Extract session identity
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	event := &savings.Event{
		SessionID:      sessionID,
		Slug:           postStats.Slug,
		Timestamp:      time.Now(),
		TokensRemoved:  tokensSaved,
		TurnsRemaining: turnsRemaining,
		Model:          postStats.Model,
		AvoidedTokens:  avoidedTokens,
		AvoidedCost:    avoidedCost,
	}

	// Record to savings log
	dir := resolveClaudeDir()
	if err := savings.Append(dir, *event); err != nil {
		slog.Warn("Failed to record savings event", "error", err)
	}

	return event
}

// printSavingsLine prints the savings line after a cleanup operation.
func printSavingsLine(event *savings.Event) {
	if event == nil {
		return
	}
	fmt.Printf("This cleanup avoids ~%s cache-read tokens (~%s) assuming ~%d turns remaining.\n",
		formatTokens(event.AvoidedTokens),
		analyzer.FormatCost(event.AvoidedCost),
		event.TurnsRemaining)
}

func init() {
	cleanCmd.Flags().BoolVar(&cleanImages, "images", false, "Replace base64 images with placeholders")
	cleanCmd.Flags().BoolVar(&cleanProgress, "progress", false, "Remove all progress messages")
	cleanCmd.Flags().BoolVar(&cleanSeparators, "separators", false, "Strip decorative separator lines")
	cleanCmd.Flags().BoolVar(&cleanSnapshots, "snapshots", false, "Remove all file-history-snapshot entries")
	cleanCmd.Flags().BoolVar(&cleanDedupReads, "dedup-reads", false, "Remove stale duplicate file reads")
	cleanCmd.Flags().BoolVar(&cleanTruncate, "truncate-output", false, "Truncate large Bash outputs")
	cleanCmd.Flags().IntVar(&cleanOutThreshold, "output-threshold", 4096, "Byte threshold for output truncation")
	cleanCmd.Flags().IntVar(&cleanOutKeepLines, "keep-lines", 10, "Lines to keep at start and end")
	cleanCmd.Flags().BoolVar(&cleanFailedRetries, "failed-retries", false, "Remove failed tool attempts that were retried")
	cleanCmd.Flags().BoolVar(&cleanSidechains, "sidechains", false, "Remove all sidechain entries")
	cleanCmd.Flags().BoolVar(&cleanTangents, "tangents", false, "Remove cross-repo tangent sequences")
	cleanCmd.Flags().BoolVar(&cleanAll, "all", false, "Run all cleanup operations")
	cleanCmd.Flags().BoolVar(&cleanLive, "live", false, "Safe cleanup for active sessions (Tier 1-3)")
	cleanCmd.Flags().BoolVar(&cleanAggressive, "aggressive", false, "Include Tier 4-5 operations (use with --live)")
	cleanCmd.Flags().BoolVar(&cleanAuto, "auto", false, "Find and clean the most recent session (no session arg needed)")
	cleanCmd.Flags().BoolVar(&cleanActiveFlag, "active", false, "Clean all active sessions (requires --all)")
	cleanCmd.Flags().StringVar(&cleanActiveSince, "since", "10m", "Activity window for --active (e.g. 10m, 1h)")
	cleanCmd.Flags().BoolVar(&cleanWatch, "watch", false, "Continuous cleanup loop (requires --active --all)")
	cleanCmd.Flags().IntVar(&cleanWatchInterval, "interval", 0, "Watch interval in seconds (0=smart mtime-based)")
	rootCmd.AddCommand(cleanCmd)
}
