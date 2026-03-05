package commands

import (
	"testing"
	"time"
)

func TestValidateBillingWeekStart(t *testing.T) {
	got, err := validateBillingWeekStart("Mon")
	if err != nil {
		t.Fatalf("validateBillingWeekStart returned error: %v", err)
	}
	if got != "monday" {
		t.Fatalf("weekday normalize = %q, want %q", got, "monday")
	}

	got, err = validateBillingWeekStart("2026-03-04")
	if err != nil {
		t.Fatalf("validateBillingWeekStart returned error: %v", err)
	}
	if got != "2026-03-04" {
		t.Fatalf("date normalize = %q, want %q", got, "2026-03-04")
	}

	if _, err := validateBillingWeekStart("not-a-day"); err == nil {
		t.Fatal("expected invalid billing-week-start error")
	}
}

func TestBillingWeekWindowWeekday(t *testing.T) {
	now := time.Date(2026, 3, 5, 16, 0, 0, 0, time.UTC) // Thursday
	start, end := billingWeekWindow(now, "monday")

	wantStart := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Fatalf("start = %s, want %s", start, wantStart)
	}
	wantEnd := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %s, want %s", end, wantEnd)
	}
}

func TestBillingWeekWindowDateAnchor(t *testing.T) {
	now := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	start, end := billingWeekWindow(now, "2026-03-04")

	wantStart := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Fatalf("start = %s, want %s", start, wantStart)
	}
	wantEnd := time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC)
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %s, want %s", end, wantEnd)
	}
}

func TestFormatResetDuration(t *testing.T) {
	if got := formatResetDuration(62*time.Hour + 15*time.Minute); got != "2d 14h" {
		t.Fatalf("formatResetDuration = %q, want %q", got, "2d 14h")
	}
	if got := formatResetDuration(45 * time.Minute); got != "45m" {
		t.Fatalf("formatResetDuration = %q, want %q", got, "45m")
	}
}
