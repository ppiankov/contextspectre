package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check tool health and environment",
	RunE:  runDoctor,
}

// DoctorOutput is the JSON output for the doctor command.
type DoctorOutput struct {
	Version           string                 `json:"version"`
	Platform          string                 `json:"platform"`
	ClaudeDir         DoctorCheck            `json:"claude_dir"`
	Sessions          DoctorCheck            `json:"sessions"`
	SidechainHealth   DoctorCheck            `json:"sidechains"`
	SidechainEntries  int                    `json:"sidechain_entries,omitempty"`
	SidechainSessions int                    `json:"sidechain_sessions,omitempty"`
	EntropyHealth     DoctorCheck            `json:"entropy"`
	EntropySessions   []DoctorEntropySession `json:"entropy_sessions,omitempty"`
	CadenceHealth     DoctorCheck            `json:"cleanup_cadence"`
	CadenceSessions   []DoctorCadenceSession `json:"cleanup_priority,omitempty"`
	Companions        []CompanionCheck       `json:"companions"`
}

// DoctorCheck holds a single health check result.
type DoctorCheck struct {
	Status  string `json:"status"` // "ok", "warn", "error"
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// CompanionCheck holds info about a companion tool.
type CompanionCheck struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
}

// DoctorEntropySession holds one ranked session for entropy urgency.
type DoctorEntropySession struct {
	SessionID    string  `json:"session_id"`
	Slug         string  `json:"slug,omitempty"`
	Project      string  `json:"project,omitempty"`
	EntropyScore float64 `json:"entropy_score"`
	EntropyLevel string  `json:"entropy_level"`
	SignalGrade  string  `json:"signal_grade,omitempty"`
	Compactions  int     `json:"compactions,omitempty"`
}

// DoctorCadenceSession is one cleanup-priority row for doctor output.
type DoctorCadenceSession struct {
	SessionID         string  `json:"session_id"`
	Slug              string  `json:"slug,omitempty"`
	Project           string  `json:"project,omitempty"`
	CadenceScore      float64 `json:"cadence_score"`
	CleanupStatus     string  `json:"cleanup_status"`
	Reason            string  `json:"reason"`
	RecommendedAction string  `json:"recommended_action"`
	PotentialSaveCost float64 `json:"potential_save_cost"`
	PotentialSaveTok  int     `json:"potential_save_tokens"`
	TurnsToCompaction int     `json:"turns_until_compaction"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	out := DoctorOutput{
		Version:  version,
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}

	dir := resolveClaudeDir()

	// Check claude directory
	if fi, err := os.Stat(dir); err != nil {
		out.ClaudeDir = DoctorCheck{Status: "error", Message: "claude directory not found", Detail: dir}
	} else if !fi.IsDir() {
		out.ClaudeDir = DoctorCheck{Status: "error", Message: "claude path is not a directory", Detail: dir}
	} else {
		out.ClaudeDir = DoctorCheck{Status: "ok", Message: "claude directory accessible", Detail: dir}
	}

	// Check sessions
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		out.Sessions = DoctorCheck{Status: "error", Message: fmt.Sprintf("list sessions: %v", err)}
	} else if len(sessions) == 0 {
		out.Sessions = DoctorCheck{Status: "warn", Message: "no sessions found"}
	} else {
		out.Sessions = DoctorCheck{
			Status:  "ok",
			Message: fmt.Sprintf("%d sessions found", len(sessions)),
		}

		sidechainEntries := 0
		sidechainSessions := 0
		var entropySessions []DoctorEntropySession
		var cadenceSessions []DoctorCadenceSession
		for _, si := range sessions {
			entries, err := jsonl.Parse(si.FullPath)
			if err != nil {
				continue
			}
			stats := analyzer.Analyze(entries)
			if stats.SidechainCount > 0 {
				sidechainEntries += stats.SidechainCount
				sidechainSessions++
			}

			dupResult := analyzer.FindDuplicateReads(entries)
			retryResult := analyzer.FindFailedRetries(entries)
			tangentResult := analyzer.FindTangents(entries)
			rec := analyzer.Recommend(stats, dupResult, retryResult, tangentResult)
			drift := analyzer.AnalyzeScopeDrift(entries, stats.Compactions, "")
			health := analyzer.ComputeHealth(stats, rec)

			signalGrade := "A"
			signalRatio := analyzer.SignalRatioForGrade(signalGrade)
			if health != nil {
				signalGrade = health.Grade
				signalRatio = analyzer.SignalRatioForGrade(health.Grade)
			}

			entropy := analyzer.CalculateEntropy(analyzer.EntropyInput{
				SignalRatio:     signalRatio,
				CurrentTokens:   stats.CurrentContextTokens,
				DriftRatio:      drift.OverallDrift,
				OrphanTokens:    stats.SidechainTokens,
				TotalTokens:     stats.CurrentContextTokens,
				CompactionCount: stats.CompactionCount,
			})
			entropySessions = append(entropySessions, DoctorEntropySession{
				SessionID:    si.SessionID,
				Slug:         si.Slug,
				Project:      si.ProjectName,
				EntropyScore: entropy.Score,
				EntropyLevel: string(entropy.Level),
				SignalGrade:  signalGrade,
				Compactions:  stats.CompactionCount,
			})

			cadence := analyzer.AssessCleanupCadence(stats, rec)
			if cadence != nil {
				cadenceSessions = append(cadenceSessions, DoctorCadenceSession{
					SessionID:         si.SessionID,
					Slug:              si.Slug,
					Project:           si.ProjectName,
					CadenceScore:      cadence.Score,
					CleanupStatus:     string(cadence.Status),
					Reason:            cadence.Reason,
					RecommendedAction: cadence.RecommendedAction,
					PotentialSaveCost: cadence.ProjectedSaveCost,
					PotentialSaveTok:  cadence.ProjectedSaveTokens,
					TurnsToCompaction: cadence.TurnsUntilCompaction,
				})
			}
		}
		sort.Slice(entropySessions, func(i, j int) bool {
			return entropySessions[i].EntropyScore > entropySessions[j].EntropyScore
		})
		sort.Slice(cadenceSessions, func(i, j int) bool {
			return cadenceSessions[i].CadenceScore > cadenceSessions[j].CadenceScore
		})
		out.SidechainEntries = sidechainEntries
		out.SidechainSessions = sidechainSessions
		out.EntropySessions = entropySessions
		out.CadenceSessions = cadenceSessions
		if sidechainEntries == 0 {
			out.SidechainHealth = DoctorCheck{
				Status:  "ok",
				Message: "no sidechains detected",
			}
		} else {
			out.SidechainHealth = DoctorCheck{
				Status:  "warn",
				Message: fmt.Sprintf("%d sidechains across %d sessions", sidechainEntries, sidechainSessions),
			}
		}

		if len(entropySessions) == 0 {
			out.EntropyHealth = DoctorCheck{
				Status:  "warn",
				Message: "no sessions could be analyzed for entropy",
			}
		} else {
			top := entropySessions[0]
			status := "ok"
			if top.EntropyScore > 50 {
				status = "warn"
			}
			out.EntropyHealth = DoctorCheck{
				Status:  status,
				Message: fmt.Sprintf("sessions ranked by entropy (highest: %.1f %s)", top.EntropyScore, top.EntropyLevel),
			}
		}

		if len(cadenceSessions) == 0 {
			out.CadenceHealth = DoctorCheck{
				Status:  "warn",
				Message: "no sessions could be assessed for cleanup cadence",
			}
		} else {
			top := cadenceSessions[0]
			status := "ok"
			if top.CadenceScore >= 40 {
				status = "warn"
			}
			out.CadenceHealth = DoctorCheck{
				Status:  status,
				Message: fmt.Sprintf("cleanup priority ranked by cadence (highest: %.1f %s)", top.CadenceScore, top.CleanupStatus),
			}
		}
	}

	// Check companion tools
	companions := []string{"ancc", "chainwatch"}
	for _, name := range companions {
		cc := CompanionCheck{Name: name}
		if path, err := exec.LookPath(name); err == nil {
			cc.Available = true
			cc.Path = path
		}
		out.Companions = append(out.Companions, cc)
	}

	if isJSON() {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Text output
	fmt.Printf("contextspectre doctor (%s)\n\n", out.Platform)

	printCheck("Claude directory", out.ClaudeDir)
	printCheck("Sessions", out.Sessions)
	if out.SidechainHealth.Message != "" {
		printCheck("Sidechains", out.SidechainHealth)
	}
	if out.EntropyHealth.Message != "" {
		printCheck("Entropy", out.EntropyHealth)
	}
	if out.CadenceHealth.Message != "" {
		printCheck("Cleanup cadence", out.CadenceHealth)
	}

	fmt.Println()
	if len(out.EntropySessions) > 0 {
		fmt.Println("Session entropy (highest first):")
		limit := 10
		if len(out.EntropySessions) < limit {
			limit = len(out.EntropySessions)
		}
		for i := 0; i < limit; i++ {
			es := out.EntropySessions[i]
			label := es.Slug
			if label == "" {
				label = shortSessionID(es.SessionID)
			}
			fmt.Printf("  %d. %-24s %8s %5.1f  (%s)\n",
				i+1, label, es.EntropyLevel, es.EntropyScore, es.Project)
		}
		fmt.Println()
	}
	if len(out.CadenceSessions) > 0 {
		fmt.Println("Cleanup priority (highest first):")
		limit := 10
		if len(out.CadenceSessions) < limit {
			limit = len(out.CadenceSessions)
		}
		for i := 0; i < limit; i++ {
			cs := out.CadenceSessions[i]
			label := cs.Slug
			if label == "" {
				label = shortSessionID(cs.SessionID)
			}
			fmt.Printf("  Priority %d: %s (score %.1f, %s, save %s) -> %s\n",
				i+1,
				label,
				cs.CadenceScore,
				cs.CleanupStatus,
				analyzer.FormatCost(cs.PotentialSaveCost),
				cs.RecommendedAction)
		}
		fmt.Println()
	}

	fmt.Println("Companion tools:")
	for _, c := range out.Companions {
		if c.Available {
			fmt.Printf("  %s: found at %s\n", c.Name, c.Path)
		} else {
			fmt.Printf("  %s: not found\n", c.Name)
		}
	}
	return nil
}

func shortSessionID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func printCheck(label string, check DoctorCheck) {
	icon := "?"
	switch check.Status {
	case "ok":
		icon = "ok"
	case "warn":
		icon = "!!"
	case "error":
		icon = "XX"
	}
	msg := check.Message
	if check.Detail != "" {
		msg += " (" + check.Detail + ")"
	}
	fmt.Printf("  [%s] %s: %s\n", icon, label, msg)
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
