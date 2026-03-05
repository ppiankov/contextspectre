package commands

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var usageCWD bool

var usageCmd = &cobra.Command{
	Use:   "usage [session-id-or-path]",
	Short: "Show weekly usage telemetry and cooldown reset",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUsage,
}

// WeeklyUsageSummary is the aggregate weekly usage snapshot.
type WeeklyUsageSummary struct {
	WeekStart           time.Time     `json:"week_start"`
	WeekEnd             time.Time     `json:"week_end"`
	WeeklyLimit         float64       `json:"weekly_limit,omitempty"`
	TotalCost           float64       `json:"total_cost"`
	Remaining           float64       `json:"remaining,omitempty"`
	ResetIn             time.Duration `json:"reset_in"`
	ResetInHuman        string        `json:"reset_in_human"`
	BurnRatePerHour     float64       `json:"burn_rate_per_hour"`
	CurrentSessionID    string        `json:"current_session_id,omitempty"`
	CurrentSessionCost  float64       `json:"current_session_cost,omitempty"`
	CurrentSessionShare float64       `json:"current_session_share,omitempty"`
	SessionCount        int           `json:"session_count"`
}

func runUsage(cmd *cobra.Command, args []string) error {
	currentSessionID := ""
	if len(args) > 0 || usageCWD {
		path, err := resolveSessionArg(args, usageCWD)
		if err != nil {
			return err
		}
		currentSessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}

	summary, err := computeWeeklyUsageSummary(resolveClaudeDir(), currentSessionID, time.Now())
	if err != nil {
		return err
	}

	if isJSON() {
		return printJSON(summary)
	}

	line := fmt.Sprintf("Week usage: %s", analyzer.FormatCost(summary.TotalCost))
	if summary.WeeklyLimit > 0 {
		line += fmt.Sprintf(" / %s | Remaining: %s",
			analyzer.FormatCost(summary.WeeklyLimit),
			analyzer.FormatCost(summary.Remaining))
	}
	line += fmt.Sprintf(" | Reset: %s", summary.ResetInHuman)
	fmt.Println(line)
	fmt.Printf("Burn rate: %s/hour\n", analyzer.FormatCost(summary.BurnRatePerHour))
	if summary.CurrentSessionID != "" {
		fmt.Printf("Current session share: %.1f%% (%s)\n",
			summary.CurrentSessionShare,
			analyzer.FormatCost(summary.CurrentSessionCost))
	}

	return nil
}

func computeWeeklyUsageSummary(claudeDir, currentSessionID string, now time.Time) (*WeeklyUsageSummary, error) {
	weekStart, weekEnd := billingWeekWindow(now, loadBillingWeekStart())

	d := &session.Discoverer{ClaudeDir: claudeDir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	total := 0.0
	currentCost := 0.0
	currentMatched := false
	fallbackSessionID := ""
	fallbackSessionCost := 0.0
	weekSessionCount := 0

	for _, s := range sessions {
		if s.ContextStats == nil || s.Modified.IsZero() {
			continue
		}
		if s.Modified.Before(weekStart) || !s.Modified.Before(weekEnd) {
			continue
		}

		cost := s.ContextStats.EstimatedCost
		total += cost
		weekSessionCount++

		if fallbackSessionID == "" {
			fallbackSessionID = s.SessionID
			fallbackSessionCost = cost
		}
		if currentSessionID != "" && s.SessionID == currentSessionID {
			currentCost += cost
			currentMatched = true
		}
	}

	if currentSessionID == "" || !currentMatched {
		currentSessionID = fallbackSessionID
		currentCost = fallbackSessionCost
	}

	limit := loadWeeklyBudgetLimit()
	remaining := 0.0
	if limit > 0 {
		remaining = limit - total
	}

	resetIn := weekEnd.Sub(now)
	if resetIn < 0 {
		resetIn = 0
	}

	burnRate := 0.0
	elapsedHours := now.Sub(weekStart).Hours()
	if elapsedHours > 0 {
		burnRate = total / elapsedHours
	}

	share := 0.0
	if total > 0 {
		share = currentCost / total * 100
	}

	return &WeeklyUsageSummary{
		WeekStart:           weekStart,
		WeekEnd:             weekEnd,
		WeeklyLimit:         limit,
		TotalCost:           total,
		Remaining:           remaining,
		ResetIn:             resetIn,
		ResetInHuman:        formatResetDuration(resetIn),
		BurnRatePerHour:     burnRate,
		CurrentSessionID:    currentSessionID,
		CurrentSessionCost:  currentCost,
		CurrentSessionShare: share,
		SessionCount:        weekSessionCount,
	}, nil
}

func init() {
	usageCmd.Flags().BoolVar(&usageCWD, "cwd", false, "Use most recent session for current directory as current session")
	rootCmd.AddCommand(usageCmd)
}
