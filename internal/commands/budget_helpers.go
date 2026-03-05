package commands

import (
	"time"

	"github.com/ppiankov/contextspectre/internal/project"
	"github.com/ppiankov/contextspectre/internal/savings"
	"github.com/ppiankov/contextspectre/internal/session"
)

func loadWeeklyBudgetLimit() float64 {
	dir := resolveClaudeDir()
	cfg, err := project.Load(dir)
	if err != nil {
		return 0
	}
	return cfg.WeeklyBudgetLimit
}

func computeWeeklySpend(claudeDir string) (float64, int, error) {
	d := &session.Discoverer{ClaudeDir: claudeDir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return 0, 0, err
	}

	now := time.Now()
	weekStart := startOfWeek(now)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	weeklySpent := 0.0
	highDailySessions := 0

	for _, s := range sessions {
		if s.ContextStats == nil {
			continue
		}
		cost := s.ContextStats.EstimatedCost
		if !s.Modified.IsZero() && (s.Modified.Equal(weekStart) || s.Modified.After(weekStart)) {
			weeklySpent += cost
		}
		if !s.Modified.IsZero() && (s.Modified.Equal(dayStart) || s.Modified.After(dayStart)) && cost > 5 {
			highDailySessions++
		}
	}

	return weeklySpent, highDailySessions, nil
}

func computeWeeklySavings(claudeDir string) float64 {
	events, err := savings.Load(claudeDir)
	if err != nil || len(events) == 0 {
		return 0
	}
	start := startOfWeek(time.Now())
	total := 0.0
	for _, ev := range events {
		if ev.Timestamp.Equal(start) || ev.Timestamp.After(start) {
			total += ev.AvoidedCost
		}
	}
	return total
}

func startOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	// Go week starts on Sunday(0). Shift to Monday-based week.
	if weekday == 0 {
		weekday = 7
	}
	delta := weekday - 1
	dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return dayStart.AddDate(0, 0, -delta)
}
