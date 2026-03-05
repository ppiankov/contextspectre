package commands

import (
	"fmt"
	"strings"
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
	if cfg.WeeklyLimit > 0 {
		return cfg.WeeklyLimit
	}
	return cfg.WeeklyBudgetLimit
}

func loadBillingWeekStart() string {
	dir := resolveClaudeDir()
	cfg, err := project.Load(dir)
	if err != nil {
		return "monday"
	}
	if strings.TrimSpace(cfg.BillingWeekStart) == "" {
		return "monday"
	}
	return cfg.BillingWeekStart
}

func computeWeeklySpend(claudeDir string) (float64, int, error) {
	d := &session.Discoverer{ClaudeDir: claudeDir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return 0, 0, err
	}

	now := time.Now()
	weekStart, weekEnd := billingWeekWindow(now, loadBillingWeekStart())
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	weeklySpent := 0.0
	highDailySessions := 0

	for _, s := range sessions {
		if s.ContextStats == nil {
			continue
		}
		cost := s.ContextStats.EstimatedCost
		if !s.Modified.IsZero() &&
			(s.Modified.Equal(weekStart) || s.Modified.After(weekStart)) &&
			s.Modified.Before(weekEnd) {
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
	start, end := billingWeekWindow(time.Now(), loadBillingWeekStart())
	total := 0.0
	for _, ev := range events {
		if (ev.Timestamp.Equal(start) || ev.Timestamp.After(start)) && ev.Timestamp.Before(end) {
			total += ev.AvoidedCost
		}
	}
	return total
}

func startOfWeek(t time.Time) time.Time {
	start, _ := billingWeekWindow(t, loadBillingWeekStart())
	return start
}

func billingWeekWindow(now time.Time, startSpec string) (time.Time, time.Time) {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startSpec = strings.TrimSpace(startSpec)
	if startSpec == "" {
		startSpec = "monday"
	}

	if anchor, err := time.ParseInLocation("2006-01-02", startSpec, now.Location()); err == nil {
		start := billingWindowFromAnchor(dayStart, anchor)
		return start, start.AddDate(0, 0, 7)
	}

	wd, ok := parseBillingWeekday(startSpec)
	if !ok {
		wd = time.Monday
	}
	delta := (int(dayStart.Weekday()) - int(wd) + 7) % 7
	start := dayStart.AddDate(0, 0, -delta)
	return start, start.AddDate(0, 0, 7)
}

func validateBillingWeekStart(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("billing-week-start cannot be empty")
	}
	if anchor, err := time.Parse("2006-01-02", value); err == nil {
		return anchor.Format("2006-01-02"), nil
	}
	if wd, ok := parseBillingWeekday(value); ok {
		return strings.ToLower(wd.String()), nil
	}
	return "", fmt.Errorf("invalid billing-week-start: %s (use weekday like monday or date YYYY-MM-DD)", value)
}

func parseBillingWeekday(value string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sun", "sunday":
		return time.Sunday, true
	case "mon", "monday":
		return time.Monday, true
	case "tue", "tues", "tuesday":
		return time.Tuesday, true
	case "wed", "wednesday":
		return time.Wednesday, true
	case "thu", "thurs", "thursday":
		return time.Thursday, true
	case "fri", "friday":
		return time.Friday, true
	case "sat", "saturday":
		return time.Saturday, true
	default:
		return time.Sunday, false
	}
}

func billingWindowFromAnchor(dayStart, anchor time.Time) time.Time {
	days := int(dayStart.Sub(anchor).Hours() / 24)
	weeks := floorDiv(days, 7)
	return anchor.AddDate(0, 0, weeks*7)
}

func floorDiv(a, b int) int {
	q := a / b
	r := a % b
	if r != 0 && ((r > 0) != (b > 0)) {
		q--
	}
	return q
}

func formatResetDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	hours := int(d.Hours())
	days := hours / 24
	remHours := hours % 24
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, remHours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	mins := int(d.Minutes())
	if mins <= 0 {
		return "0m"
	}
	return fmt.Sprintf("%dm", mins)
}
