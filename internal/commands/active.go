package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	activeSince string
	activeQuiet bool
)

var activeCmd = &cobra.Command{
	Use:   "active",
	Short: "Show currently active sessions",
	Long:  "Lists sessions modified within the activity window (default 10 minutes) with key health metrics.",
	RunE:  runActive,
}

func init() {
	activeCmd.Flags().StringVar(&activeSince, "since", "10m", "Activity window duration (e.g. 10m, 1h, 30s)")
	activeCmd.Flags().BoolVar(&activeQuiet, "quiet", false, "One-line summary output")
	rootCmd.AddCommand(activeCmd)
}

func runActive(_ *cobra.Command, _ []string) error {
	sinceDuration, err := time.ParseDuration(activeSince)
	if err != nil {
		return fmt.Errorf("invalid --since value %q: %w", activeSince, err)
	}

	d := &session.Discoverer{ClaudeDir: resolveClaudeDir()}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return fmt.Errorf("discover sessions: %w", err)
	}

	// Filter to active sessions within the window.
	cutoff := time.Now().Add(-sinceDuration)
	var active []session.Info
	for _, s := range sessions {
		if s.Modified.After(cutoff) {
			active = append(active, s)
		}
	}

	// Classify healthy vs needs-cleaning.
	healthy, needsCleaning := 0, 0
	for _, s := range active {
		grade := sessionGrade(s)
		if grade == "A" || grade == "B" {
			healthy++
		} else {
			needsCleaning++
		}
	}

	// JSON output.
	if isJSON() {
		return printJSON(buildActiveOutput(active, healthy, needsCleaning))
	}

	// Quiet output.
	if activeQuiet {
		if len(active) == 0 {
			fmt.Println("No active sessions")
		} else {
			fmt.Printf("%d active: %d healthy, %d needs cleaning\n",
				len(active), healthy, needsCleaning)
		}
		return exitCodeFromHealth(needsCleaning)
	}

	// Text output.
	if len(active) == 0 {
		fmt.Println("No active sessions")
		return nil
	}

	fmt.Printf("Active sessions (%d):\n", len(active))

	// Find max project name length for alignment.
	maxProj := 0
	for _, s := range active {
		if len(s.ProjectName) > maxProj {
			maxProj = len(s.ProjectName)
		}
	}
	if maxProj > 20 {
		maxProj = 20
	}

	for _, s := range active {
		proj := s.ProjectName
		if len(proj) > maxProj {
			proj = proj[:maxProj]
		}

		ctxPct := 0.0
		ctxTokens := 0
		sigPct := 0
		cost := 0.0
		model := ""
		if s.ContextStats != nil {
			ctxPct = s.ContextStats.ContextPct
			ctxTokens = s.ContextStats.ContextTokens
			sigPct = s.ContextStats.SignalPercent
			cost = s.ContextStats.EstimatedCost
			model = s.ContextStats.Model
		}

		grade := analyzer.GradeFromSignalPercent(sigPct)
		cleanable := estimateCleanable(ctxTokens, sigPct)
		costStr := analyzer.FormatCost(cost)
		cleanStr := formatTokens(cleanable)
		modStr := timeAgo(s.Modified)

		modelShort := ""
		if model != "" {
			pricing := analyzer.PricingForModel(model)
			modelShort = pricing.Name
		}

		line := fmt.Sprintf("  %-*s  ctx:%4.0f%%  sig:%s  %8s  clean:%-6s  %s",
			maxProj, proj, ctxPct, grade, costStr, cleanStr, modStr)
		if modelShort != "" {
			line += fmt.Sprintf("  (%s)", modelShort)
		}
		fmt.Println(line)
	}

	return exitCodeFromHealth(needsCleaning)
}

func sessionGrade(s session.Info) string {
	sigPct := 0
	if s.ContextStats != nil {
		sigPct = s.ContextStats.SignalPercent
	}
	return analyzer.GradeFromSignalPercent(sigPct)
}

func estimateCleanable(contextTokens, signalPercent int) int {
	if contextTokens == 0 || signalPercent >= 100 {
		return 0
	}
	return contextTokens * (100 - signalPercent) / 100
}

func buildActiveOutput(active []session.Info, healthy, needsCleaning int) ActiveOutput {
	out := ActiveOutput{
		Total:         len(active),
		Healthy:       healthy,
		NeedsCleaning: needsCleaning,
	}
	for _, s := range active {
		ctxPct := 0.0
		ctxTokens := 0
		sigPct := 0
		cost := 0.0
		model := ""
		if s.ContextStats != nil {
			ctxPct = s.ContextStats.ContextPct
			ctxTokens = s.ContextStats.ContextTokens
			sigPct = s.ContextStats.SignalPercent
			cost = s.ContextStats.EstimatedCost
			model = s.ContextStats.Model
		}

		out.Active = append(out.Active, ActiveSessionJSON{
			ID:              s.SessionID,
			Slug:            s.Slug,
			Project:         s.ProjectName,
			ContextPercent:  ctxPct,
			SignalGrade:     analyzer.GradeFromSignalPercent(sigPct),
			SignalPercent:   sigPct,
			EstimatedCost:   cost,
			CleanableTokens: estimateCleanable(ctxTokens, sigPct),
			Model:           model,
			LastModified:    s.Modified.Format(time.RFC3339),
			SecondsAgo:      int(time.Since(s.Modified).Seconds()),
		})
	}
	if out.Active == nil {
		out.Active = []ActiveSessionJSON{}
	}
	return out
}

// exitCodeFromHealth returns a SilentErr if any sessions need cleaning.
func exitCodeFromHealth(needsCleaning int) error {
	if needsCleaning > 0 {
		os.Exit(1)
	}
	return nil
}
