package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
)

// outputFormat is a persistent flag for JSON output.
var outputFormat string

// SessionJSON is the JSON output for a single session.
type SessionJSON struct {
	ID             string    `json:"id"`
	Slug           string    `json:"slug,omitempty"`
	Project        string    `json:"project"`
	Branch         string    `json:"branch,omitempty"`
	Messages       int       `json:"messages"`
	Tokens         int       `json:"tokens"`
	ContextPercent float64   `json:"context_percent"`
	Compactions    int       `json:"compactions"`
	FileSizeBytes  int64     `json:"file_size_bytes"`
	LastModified   time.Time `json:"last_modified"`
	Active         bool      `json:"active"`
	Images         int       `json:"images"`
	EstimatedCost  float64   `json:"estimated_cost,omitempty"`
	Model          string    `json:"model,omitempty"`
	SignalPercent  *int      `json:"signal_percent,omitempty"`
	EntropyScore   *float64  `json:"entropy_score,omitempty"`
	EntropyLevel   string    `json:"entropy_level,omitempty"`
	CleanupStatus  string    `json:"cleanup_status,omitempty"`
	CadenceScore   *float64  `json:"cadence_score,omitempty"`
	ClientType     string    `json:"client_type,omitempty"`
}

// SessionsOutput is the JSON output for the sessions command.
type SessionsOutput struct {
	Sessions []SessionJSON `json:"sessions"`
	Total    int           `json:"total"`
}

// StatsOutput is the JSON output for the stats command.
type StatsOutput struct {
	SessionID          string                    `json:"session_id"`
	Project            string                    `json:"project,omitempty"`
	ClientType         string                    `json:"client_type,omitempty"`
	Context            ContextJSON               `json:"context"`
	Health             *HealthScoreJSON          `json:"health,omitempty"`
	Entropy            *EntropyJSON              `json:"entropy,omitempty"`
	Cadence            *CadenceJSON              `json:"cadence,omitempty"`
	BudgetProtection   *BudgetProtectionJSON     `json:"budget_protection,omitempty"`
	Cost               *CostJSON                 `json:"cost,omitempty"`
	CostAlertThreshold float64                   `json:"cost_alert_threshold,omitempty"`
	CostAlertTriggered bool                      `json:"cost_alert_triggered,omitempty"`
	DecisionEconomics  *DecisionEconomicsJSON    `json:"decision_economics,omitempty"`
	VectorGauge        *VectorGaugeJSON          `json:"vector_gauge,omitempty"`
	EpochCosts         []EpochCostJSON           `json:"epoch_costs,omitempty"`
	Archaeology        *ArchaeologyJSON          `json:"archaeology,omitempty"`
	Compactions        CompactionsJSON           `json:"compactions"`
	Messages           MessagesJSON              `json:"messages"`
	Images             ImagesJSON                `json:"images"`
	GrowthRate         GrowthRateJSON            `json:"growth_rate"`
	Recommendation     *RecommendationJSON       `json:"recommendation,omitempty"`
	Sidechains         *SidechainStatsJSON       `json:"sidechains,omitempty"`
	EpochTimeline      []EpochTimelineJSON       `json:"epoch_timeline,omitempty"`
	ScopeDrift         *ScopeDriftJSON           `json:"scope_drift,omitempty"`
	GhostContext       *GhostReportJSON          `json:"ghost_context,omitempty"`
	InputPurity        *analyzer.InputPurity     `json:"input_purity,omitempty"`
	Integrity          *analyzer.IntegrityReport `json:"integrity,omitempty"`
}

// DecisionEconomicsJSON holds CPD/TTC/CDR for JSON output.
type DecisionEconomicsJSON struct {
	CPD             float64                 `json:"cpd"`
	TTC             int                     `json:"ttc"`
	CDR             float64                 `json:"cdr"`
	TotalDecisions  int                     `json:"total_decisions"`
	DecisionDensity float64                 `json:"decision_density"`
	PerEpoch        []EpochDecisionEconJSON `json:"per_epoch,omitempty"`
}

// EpochDecisionEconJSON holds per-epoch decision economics.
type EpochDecisionEconJSON struct {
	Epoch     int     `json:"epoch"`
	CPD       float64 `json:"cpd"`
	TTC       int     `json:"ttc"`
	CDR       float64 `json:"cdr"`
	Decisions int     `json:"decisions"`
	Density   float64 `json:"density"`
}

// VectorGaugeJSON holds session health vector for JSON output.
type VectorGaugeJSON struct {
	State          string  `json:"state"`
	Action         string  `json:"action"`
	Score          int     `json:"score"`
	ContextPct     float64 `json:"context_percent"`
	CPD            float64 `json:"cpd,omitempty"`
	TTC            int     `json:"ttc,omitempty"`
	CDR            float64 `json:"cdr,omitempty"`
	PostCompaction bool    `json:"post_compaction,omitempty"`
}

// ArchaeologyJSON holds compaction archaeology for JSON output.
type ArchaeologyJSON struct {
	Events []CompactionArchJSON `json:"events"`
}

// CompactionArchJSON is a single compaction's archaeology data.
type CompactionArchJSON struct {
	CompactionIndex int              `json:"compaction_index"`
	LineIndex       int              `json:"line_index"`
	Before          EpochSummaryJSON `json:"before"`
	After           CompSummaryJSON  `json:"after"`
}

// EpochSummaryJSON holds pre-compaction epoch metadata.
type EpochSummaryJSON struct {
	TurnCount       int            `json:"turn_count"`
	TokensPeak      int            `json:"tokens_peak"`
	FilesReferenced []string       `json:"files_referenced"`
	ToolCallCounts  map[string]int `json:"tool_call_counts"`
	UserQuestions   []string       `json:"user_questions,omitempty"`
	DecisionHints   []string       `json:"decision_hints,omitempty"`
}

// CompSummaryJSON holds post-compaction summary.
type CompSummaryJSON struct {
	SummaryText      string  `json:"summary_text"`
	SummaryCharCount int     `json:"summary_char_count"`
	CompressionRatio float64 `json:"compression_ratio"`
}

// CostJSON holds session cost attribution.
type CostJSON struct {
	Model            string          `json:"model,omitempty"`
	TotalCost        float64         `json:"total_cost"`
	CostPerTurn      float64         `json:"cost_per_turn"`
	CostPerHour      float64         `json:"cost_per_hour,omitempty"`
	InputCost        float64         `json:"input_cost"`
	OutputCost       float64         `json:"output_cost"`
	CacheWriteCost   float64         `json:"cache_write_cost"`
	CacheReadCost    float64         `json:"cache_read_cost"`
	InputTokens      int             `json:"input_tokens"`
	OutputTokens     int             `json:"output_tokens"`
	CacheWriteTokens int             `json:"cache_write_tokens"`
	CacheReadTokens  int             `json:"cache_read_tokens"`
	TurnCount        int             `json:"turn_count"`
	PerModel         []ModelCostJSON `json:"per_model,omitempty"`
}

// ModelCostJSON holds cost for a single model.
type ModelCostJSON struct {
	Model     string  `json:"model"`
	Name      string  `json:"name,omitempty"`
	TurnCount int     `json:"turn_count"`
	TotalCost float64 `json:"total_cost"`
	Percent   float64 `json:"percent"`
}

// EpochCostJSON holds cost for a single compaction epoch.
type EpochCostJSON struct {
	EpochIndex int     `json:"epoch_index"`
	TurnCount  int     `json:"turn_count"`
	PeakTokens int     `json:"peak_tokens"`
	TotalCost  float64 `json:"total_cost"`
}

// EpochTimelineJSON is a unified epoch view for JSON output.
type EpochTimelineJSON struct {
	Index         int     `json:"index"`
	TurnCount     int     `json:"turn_count"`
	PeakTokens    int     `json:"peak_tokens"`
	Cost          float64 `json:"cost"`
	Topic         string  `json:"topic"`
	SurvivedChars int     `json:"survived_chars"`
	IsActive      bool    `json:"is_active,omitempty"`
}

// ContextJSON holds context usage info.
type ContextJSON struct {
	Tokens         int     `json:"tokens"`
	Percent        float64 `json:"percent"`
	Window         int     `json:"window"`
	TurnsRemaining int     `json:"turns_remaining"`
}

// CompactionsJSON holds compaction info.
type CompactionsJSON struct {
	Count  int              `json:"count"`
	Events []CompactionJSON `json:"events,omitempty"`
}

// CompactionJSON is a single compaction event.
type CompactionJSON struct {
	LineIndex  int `json:"line_index"`
	FromTokens int `json:"from_tokens"`
	ToTokens   int `json:"to_tokens"`
	TokensDrop int `json:"tokens_drop"`
}

// MessagesJSON holds message type counts.
type MessagesJSON struct {
	Total     int `json:"total"`
	User      int `json:"user"`
	Assistant int `json:"assistant"`
	Progress  int `json:"progress"`
	Snapshots int `json:"snapshots"`
	System    int `json:"system"`
}

// ImagesJSON holds image stats.
type ImagesJSON struct {
	Count           int   `json:"count"`
	BytesTotal      int64 `json:"bytes_total"`
	EstimatedTokens int   `json:"estimated_tokens"`
}

// GrowthRateJSON holds token growth rate info.
type GrowthRateJSON struct {
	TokensPerTurn       float64 `json:"tokens_per_turn"`
	SinceLastCompaction bool    `json:"since_last_compaction"`
}

// HealthScoreJSON holds signal/noise health metrics for JSON output.
type HealthScoreJSON struct {
	SignalTokens    int     `json:"signal_tokens"`
	NoiseTokens     int     `json:"noise_tokens"`
	TotalTokens     int     `json:"total_tokens"`
	SignalPercent   float64 `json:"signal_percent"`
	NoisePercent    float64 `json:"noise_percent"`
	Grade           string  `json:"grade"`
	BiggestOffender string  `json:"biggest_offender,omitempty"`
	OffenderTokens  int     `json:"offender_tokens,omitempty"`
}

// EntropyJSON holds session entropy score and per-axis breakdown.
type EntropyJSON struct {
	Score              float64 `json:"score"`
	Level              string  `json:"level"`
	Noise              float64 `json:"noise"`
	CompactionPressure float64 `json:"compaction_pressure"`
	Drift              float64 `json:"drift"`
	Orphans            float64 `json:"orphans"`
	CompressionLoss    float64 `json:"compression_loss"`
}

// CadenceJSON holds cleanup cadence urgency and savings projection.
type CadenceJSON struct {
	Score                float64 `json:"score"`
	Status               string  `json:"status"`
	Reason               string  `json:"reason"`
	RecommendedAction    string  `json:"recommended_action"`
	NoiseTokens          int     `json:"noise_tokens"`
	NoiseRatio           float64 `json:"noise_ratio"`
	TurnsUntilCompaction int     `json:"turns_until_compaction"`
	TurnsSinceCleanup    int     `json:"turns_since_cleanup"`
	ProjectedSaveTokens  int     `json:"projected_save_tokens"`
	ProjectedSaveCost    float64 `json:"projected_save_cost"`
	PerTurnSaveCost      float64 `json:"per_turn_save_cost"`
}

// BudgetProtectionJSON holds combined risk assessment and ranked actions.
type BudgetProtectionJSON struct {
	RiskLevel                 string             `json:"risk_level"`
	WeeklyLimit               float64            `json:"weekly_limit"`
	WeeklySpent               float64            `json:"weekly_spent"`
	WeeklyRemaining           float64            `json:"weekly_remaining"`
	WeeklyRemainingPercent    float64            `json:"weekly_remaining_percent"`
	TurnsUntilCompaction      int                `json:"turns_until_compaction"`
	NoiseTokens               int                `json:"noise_tokens"`
	NoiseRatio                float64            `json:"noise_ratio"`
	EstimatedCostToCompaction float64            `json:"estimated_cost_to_compaction"`
	ExpectedTurnsGained       int                `json:"expected_turns_gained"`
	ExpectedDelayMinutes      int                `json:"expected_delay_minutes"`
	RecommendedAction         string             `json:"recommended_action"`
	RecommendedSavings        float64            `json:"recommended_savings"`
	OffloadHint               bool               `json:"offload_hint"`
	Actions                   []BudgetActionJSON `json:"actions,omitempty"`
}

// BudgetActionJSON is one ranked budget action.
type BudgetActionJSON struct {
	Action           string  `json:"action"`
	EstimatedSavings float64 `json:"estimated_savings"`
}

// RecommendationJSON holds cleanup recommendations for JSON output.
type RecommendationJSON struct {
	Items               []CleanupItemJSON `json:"items"`
	TotalTokens         int               `json:"total_tokens"`
	TotalTurnsGained    int               `json:"total_turns_gained"`
	CurrentPercent      float64           `json:"current_percent"`
	ProjectedPercent    float64           `json:"projected_percent"`
	ProjectedSavedCost  float64           `json:"projected_saved_cost,omitempty"`
	ProjectedSavedToken int               `json:"projected_saved_tokens,omitempty"`
}

// CleanupItemJSON is a single cleanup recommendation item.
type CleanupItemJSON struct {
	Category    string `json:"category"`
	Label       string `json:"label"`
	Count       int    `json:"count"`
	TokensSaved int    `json:"tokens_saved"`
	TurnsGained int    `json:"turns_gained"`
}

// SidechainStatsJSON holds structural sidechain stats.
type SidechainStatsJSON struct {
	Count      int `json:"count"`
	GroupCount int `json:"group_count"`
	Tokens     int `json:"tokens"`
}

// ScopeDriftJSON holds scope drift analysis for JSON output.
type ScopeDriftJSON struct {
	SessionProject string           `json:"session_project"`
	EpochScopes    []EpochScopeJSON `json:"epoch_scopes"`
	TangentSeqs    []TangentSeqJSON `json:"tangent_sequences,omitempty"`
	TotalInScope   int              `json:"total_in_scope"`
	TotalOutScope  int              `json:"total_out_scope"`
	OverallDrift   float64          `json:"overall_drift"`
}

// EpochScopeJSON holds per-epoch scope distribution.
type EpochScopeJSON struct {
	EpochIndex     int            `json:"epoch_index"`
	InScope        int            `json:"in_scope"`
	OutScope       int            `json:"out_scope"`
	OutScopeByRepo map[string]int `json:"out_scope_by_repo,omitempty"`
	DriftRatio     float64        `json:"drift_ratio"`
	DriftCost      float64        `json:"drift_cost"`
}

// TangentSeqJSON holds a contiguous out-of-scope tangent sequence.
type TangentSeqJSON struct {
	StartIdx           int      `json:"start_idx"`
	EndIdx             int      `json:"end_idx"`
	TargetRepo         string   `json:"target_repo"`
	TokenCost          int      `json:"token_cost"`
	DollarCost         float64  `json:"dollar_cost"`
	ReExplanationFiles []string `json:"re_explanation_files,omitempty"`
}

// GhostFileJSON is a single ghost context file for JSON output.
type GhostFileJSON struct {
	Path            string `json:"path"`
	CompactionIndex int    `json:"compaction_index"`
	EpochModified   int    `json:"epoch_modified"`
}

// GhostReportJSON holds ghost context detection results for JSON output.
type GhostReportJSON struct {
	Files       []GhostFileJSON `json:"files"`
	TotalGhosts int             `json:"total_ghosts"`
}

// CleanOutput is the JSON output for the clean command.
type CleanOutput struct {
	SessionID  string           `json:"session_id"`
	Slug       string           `json:"slug,omitempty"`
	Mode       string           `json:"mode,omitempty"`
	Operations []CleanOpJSON    `json:"operations"`
	Summary    CleanSummaryJSON `json:"summary"`
}

// CleanOpJSON describes a single cleanup operation.
type CleanOpJSON struct {
	Type            string `json:"type"`
	EntriesAffected int    `json:"entries_affected"`
	TokensSaved     int    `json:"tokens_saved,omitempty"`
	BytesSaved      int64  `json:"bytes_saved,omitempty"`
}

// CleanSummaryJSON holds the combined cleanup results.
type CleanSummaryJSON struct {
	EntriesRemoved  int   `json:"entries_removed"`
	EntriesModified int   `json:"entries_modified"`
	TokensSaved     int   `json:"tokens_saved"`
	BytesSaved      int64 `json:"bytes_saved"`
}

// ActiveSessionJSON is the JSON output for a single active session.
type ActiveSessionJSON struct {
	ID              string  `json:"id"`
	Slug            string  `json:"slug,omitempty"`
	Project         string  `json:"project"`
	ContextPercent  float64 `json:"context_percent"`
	SignalGrade     string  `json:"signal_grade"`
	SignalPercent   int     `json:"signal_percent"`
	EstimatedCost   float64 `json:"estimated_cost"`
	CleanableTokens int     `json:"cleanable_tokens"`
	Model           string  `json:"model,omitempty"`
	LastModified    string  `json:"last_modified"`
	SecondsAgo      int     `json:"seconds_ago"`
}

// ActiveOutput is the JSON output for the active command.
type ActiveOutput struct {
	Active        []ActiveSessionJSON `json:"active"`
	Total         int                 `json:"total"`
	Healthy       int                 `json:"healthy"`
	NeedsCleaning int                 `json:"needs_cleaning"`
}

// CleanActiveSessionJSON is a single session result from clean --active.
type CleanActiveSessionJSON struct {
	ID                string  `json:"id"`
	Slug              string  `json:"slug,omitempty"`
	Project           string  `json:"project"`
	ProgressRemoved   int     `json:"progress_removed"`
	SnapshotsRemoved  int     `json:"snapshots_removed"`
	StaleReadsRemoved int     `json:"stale_reads_removed"`
	TokensSaved       int     `json:"tokens_saved"`
	BytesSaved        int64   `json:"bytes_saved"`
	AvoidedCost       float64 `json:"avoided_cost,omitempty"`
}

// CleanActiveOutput is the JSON output for clean --active.
type CleanActiveOutput struct {
	Sessions    []CleanActiveSessionJSON `json:"sessions"`
	Total       int                      `json:"total"`
	Cleaned     int                      `json:"cleaned"`
	TotalTokens int                      `json:"total_tokens"`
}

// isJSON returns true if the output format is JSON.
func isJSON() bool {
	return outputFormat == "json"
}

// printJSON marshals v to JSON and prints it.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// statsOutputOpt holds optional parameters for buildStatsOutput.
type statsOutputOpt struct {
	duration           time.Duration
	costAlertThreshold float64
	decisionEconomics  *analyzer.DecisionEconomics
	vectorGauge        *analyzer.VectorGauge
	entropy            *analyzer.EntropyScore
	cadence            *analyzer.CadenceAssessment
	budget             *analyzer.BudgetAssessment
}

// buildStatsOutput converts analyzer stats to JSON output.
func buildStatsOutput(sessionID string, stats *analyzer.ContextStats, rec *analyzer.CleanupRecommendation, drift *analyzer.ScopeDrift, opts ...statsOutputOpt) *StatsOutput {
	var opt statsOutputOpt
	if len(opts) > 0 {
		opt = opts[0]
	}

	out := &StatsOutput{
		SessionID:  sessionID,
		ClientType: stats.ClientType,
		Context: ContextJSON{
			Tokens:         stats.CurrentContextTokens,
			Percent:        stats.UsagePercent,
			Window:         analyzer.ContextWindowSize,
			TurnsRemaining: stats.EstimatedTurnsLeft,
		},
		Compactions: CompactionsJSON{
			Count: stats.CompactionCount,
		},
		Messages: MessagesJSON{
			Total: stats.TotalLines,
		},
		Images: ImagesJSON{
			Count:      stats.ImageCount,
			BytesTotal: stats.ImageBytesTotal,
		},
		GrowthRate: GrowthRateJSON{
			TokensPerTurn:       stats.TokenGrowthRate,
			SinceLastCompaction: stats.CompactionCount > 0,
		},
	}

	// Health score
	health := analyzer.ComputeHealth(stats, rec)
	if health != nil && health.TotalTokens > 0 {
		out.Health = &HealthScoreJSON{
			SignalTokens:    health.SignalTokens,
			NoiseTokens:     health.NoiseTokens,
			TotalTokens:     health.TotalTokens,
			SignalPercent:   health.SignalPercent,
			NoisePercent:    health.NoisePercent,
			Grade:           health.Grade,
			BiggestOffender: health.BiggestOffender,
			OffenderTokens:  health.OffenderTokens,
		}
	}
	if opt.entropy != nil {
		out.Entropy = &EntropyJSON{
			Score:              opt.entropy.Score,
			Level:              string(opt.entropy.Level),
			Noise:              opt.entropy.Breakdown.Noise,
			CompactionPressure: opt.entropy.Breakdown.CompactionPressure,
			Drift:              opt.entropy.Breakdown.Drift,
			Orphans:            opt.entropy.Breakdown.Orphans,
			CompressionLoss:    opt.entropy.Breakdown.CompressionLoss,
		}
	}
	if opt.cadence != nil {
		out.Cadence = &CadenceJSON{
			Score:                opt.cadence.Score,
			Status:               string(opt.cadence.Status),
			Reason:               opt.cadence.Reason,
			RecommendedAction:    opt.cadence.RecommendedAction,
			NoiseTokens:          opt.cadence.NoiseTokens,
			NoiseRatio:           opt.cadence.NoiseRatio,
			TurnsUntilCompaction: opt.cadence.TurnsUntilCompaction,
			TurnsSinceCleanup:    opt.cadence.TurnsSinceCleanup,
			ProjectedSaveTokens:  opt.cadence.ProjectedSaveTokens,
			ProjectedSaveCost:    opt.cadence.ProjectedSaveCost,
			PerTurnSaveCost:      opt.cadence.PerTurnSaveCost,
		}
	}
	if opt.budget != nil {
		bj := &BudgetProtectionJSON{
			RiskLevel:                 string(opt.budget.RiskLevel),
			WeeklyLimit:               opt.budget.WeeklyLimit,
			WeeklySpent:               opt.budget.WeeklySpent,
			WeeklyRemaining:           opt.budget.WeeklyRemaining,
			WeeklyRemainingPercent:    opt.budget.WeeklyRemainingPercent,
			TurnsUntilCompaction:      opt.budget.TurnsUntilCompaction,
			NoiseTokens:               opt.budget.NoiseTokens,
			NoiseRatio:                opt.budget.NoiseRatio,
			EstimatedCostToCompaction: opt.budget.EstimatedCostToCompaction,
			ExpectedTurnsGained:       opt.budget.ExpectedTurnsGained,
			ExpectedDelayMinutes:      opt.budget.ExpectedDelayMinutes,
			RecommendedAction:         opt.budget.RecommendedAction,
			RecommendedSavings:        opt.budget.RecommendedSavings,
			OffloadHint:               opt.budget.OffloadHint,
		}
		for _, a := range opt.budget.Actions {
			bj.Actions = append(bj.Actions, BudgetActionJSON{
				Action:           a.Action,
				EstimatedSavings: a.EstimatedSavings,
			})
		}
		out.BudgetProtection = bj
	}

	// Fill message counts
	for typ, count := range stats.MessageCounts {
		switch typ {
		case "user":
			out.Messages.User = count
		case "assistant":
			out.Messages.Assistant = count
		case "progress":
			out.Messages.Progress = count
		case "file-history-snapshot":
			out.Messages.Snapshots = count
		case "system":
			out.Messages.System = count
		}
	}

	// Fill compaction events
	for _, c := range stats.Compactions {
		out.Compactions.Events = append(out.Compactions.Events, CompactionJSON{
			LineIndex:  c.LineIndex,
			FromTokens: c.BeforeTokens,
			ToTokens:   c.AfterTokens,
			TokensDrop: c.TokensDrop,
		})
	}

	// Estimate image tokens
	if stats.ImageBytesTotal > 0 {
		out.Images.EstimatedTokens = int(stats.ImageBytesTotal / 750)
	}
	if stats.SidechainCount > 0 {
		out.Sidechains = &SidechainStatsJSON{
			Count:      stats.SidechainCount,
			GroupCount: stats.SidechainGroups,
			Tokens:     stats.SidechainTokens,
		}
	}

	// Cost attribution
	if stats.Cost != nil && stats.Cost.TurnCount > 0 {
		costJSON := &CostJSON{
			Model:            stats.Cost.Model,
			TotalCost:        stats.Cost.TotalCost,
			CostPerTurn:      stats.Cost.CostPerTurn,
			InputCost:        stats.Cost.InputCost,
			OutputCost:       stats.Cost.OutputCost,
			CacheWriteCost:   stats.Cost.CacheWriteCost,
			CacheReadCost:    stats.Cost.CacheReadCost,
			InputTokens:      stats.Cost.InputTokens,
			OutputTokens:     stats.Cost.OutputTokens,
			CacheWriteTokens: stats.Cost.CacheWriteTokens,
			CacheReadTokens:  stats.Cost.CacheReadTokens,
			TurnCount:        stats.Cost.TurnCount,
		}
		if opt.duration > 0 && stats.Cost.TotalCost > 0 {
			costJSON.CostPerHour = stats.Cost.TotalCost / opt.duration.Hours()
		}

		// Per-model breakdown
		if len(stats.Cost.PerModel) > 1 {
			for model, pm := range stats.Cost.PerModel {
				pct := analyzer.CostPercent(pm.TotalCost, stats.Cost.TotalCost)
				name := analyzer.PricingForModel(model).Name
				costJSON.PerModel = append(costJSON.PerModel, ModelCostJSON{
					Model:     model,
					Name:      name,
					TurnCount: pm.TurnCount,
					TotalCost: pm.TotalCost,
					Percent:   pct,
				})
			}
		}
		out.Cost = costJSON

		// Cost alert
		if opt.costAlertThreshold > 0 {
			out.CostAlertThreshold = opt.costAlertThreshold
			out.CostAlertTriggered = stats.Cost.TotalCost >= opt.costAlertThreshold
		}

		for _, ec := range stats.EpochCosts {
			out.EpochCosts = append(out.EpochCosts, EpochCostJSON{
				EpochIndex: ec.EpochIndex,
				TurnCount:  ec.TurnCount,
				PeakTokens: ec.PeakTokens,
				TotalCost:  ec.Cost.TotalCost,
			})
		}
	}

	// Decision economics
	if opt.decisionEconomics != nil && opt.decisionEconomics.HasDecisions {
		de := opt.decisionEconomics
		dej := &DecisionEconomicsJSON{
			CPD:             de.CPD,
			TTC:             de.TTC,
			CDR:             de.CDR,
			TotalDecisions:  de.TotalDecisions,
			DecisionDensity: de.DecisionDensity,
		}
		for _, epe := range de.PerEpoch {
			dej.PerEpoch = append(dej.PerEpoch, EpochDecisionEconJSON{
				Epoch:     epe.EpochIndex,
				CPD:       epe.CPD,
				TTC:       epe.TTC,
				CDR:       epe.CDR,
				Decisions: epe.Decisions,
				Density:   epe.Density,
			})
		}
		out.DecisionEconomics = dej
	}

	// Vector gauge
	if opt.vectorGauge != nil {
		g := opt.vectorGauge
		out.VectorGauge = &VectorGaugeJSON{
			State:          string(g.State),
			Action:         string(g.Action),
			Score:          g.Score,
			ContextPct:     g.ContextPct,
			CPD:            g.CPD,
			TTC:            g.TTC,
			CDR:            g.CDR,
			PostCompaction: g.PostCompaction,
		}
	}

	// Compaction archaeology
	if stats.Archaeology != nil && len(stats.Archaeology.Events) > 0 {
		arch := &ArchaeologyJSON{}
		for _, ev := range stats.Archaeology.Events {
			files := ev.Before.FilesReferenced
			if files == nil {
				files = []string{}
			}
			tools := ev.Before.ToolCallCounts
			if tools == nil {
				tools = map[string]int{}
			}
			ae := CompactionArchJSON{
				CompactionIndex: ev.CompactionIndex,
				LineIndex:       ev.LineIndex,
				Before: EpochSummaryJSON{
					TurnCount:       ev.Before.TurnCount,
					TokensPeak:      ev.Before.TokensPeak,
					FilesReferenced: files,
					ToolCallCounts:  tools,
					UserQuestions:   ev.Before.UserQuestions,
					DecisionHints:   ev.Before.DecisionHints,
				},
				After: CompSummaryJSON{
					SummaryText:      ev.After.SummaryText,
					SummaryCharCount: ev.After.SummaryCharCount,
					CompressionRatio: ev.After.CompressionRatio,
				},
			}
			arch.Events = append(arch.Events, ae)
		}
		out.Archaeology = arch
	}

	// Cleanup recommendations
	if rec != nil && len(rec.Items) > 0 {
		rj := &RecommendationJSON{
			TotalTokens:      rec.TotalTokens,
			TotalTurnsGained: rec.TotalTurnsGained,
			CurrentPercent:   rec.CurrentPercent,
			ProjectedPercent: rec.ProjectedPercent,
		}
		for _, item := range rec.Items {
			rj.Items = append(rj.Items, CleanupItemJSON{
				Category:    item.Category,
				Label:       item.Label,
				Count:       item.Count,
				TokensSaved: item.TokensSaved,
				TurnsGained: item.TurnsGained,
			})
		}
		// Projected savings if cleaned now
		if rec.TotalTokens > 0 && stats.EstimatedTurnsLeft > 0 {
			pricing := analyzer.PricingForModel(stats.Model)
			avoidedTokens := rec.TotalTokens * stats.EstimatedTurnsLeft
			avoidedCost := float64(avoidedTokens) / 1_000_000 * pricing.CacheReadPerMillion
			rj.ProjectedSavedToken = avoidedTokens
			rj.ProjectedSavedCost = avoidedCost
		}
		out.Recommendation = rj
	}

	// Epoch timeline
	if len(stats.EpochCosts) > 1 && stats.Archaeology != nil {
		epochs := analyzer.BuildEpochs(stats.EpochCosts, stats.Archaeology, "")
		for _, ep := range epochs {
			out.EpochTimeline = append(out.EpochTimeline, EpochTimelineJSON{
				Index:         ep.Index,
				TurnCount:     ep.TurnCount,
				PeakTokens:    ep.PeakTokens,
				Cost:          ep.Cost,
				Topic:         ep.Topic,
				SurvivedChars: ep.SurvivedChars,
				IsActive:      ep.IsActive,
			})
		}
	}

	// Scope drift
	if drift != nil && drift.TotalOutScope > 0 {
		dj := &ScopeDriftJSON{
			SessionProject: drift.SessionProject,
			TotalInScope:   drift.TotalInScope,
			TotalOutScope:  drift.TotalOutScope,
			OverallDrift:   drift.OverallDrift,
		}
		for _, es := range drift.EpochScopes {
			repos := es.OutScopeByRepo
			if repos == nil {
				repos = map[string]int{}
			}
			dj.EpochScopes = append(dj.EpochScopes, EpochScopeJSON{
				EpochIndex:     es.EpochIndex,
				InScope:        es.InScope,
				OutScope:       es.OutScope,
				OutScopeByRepo: repos,
				DriftRatio:     es.DriftRatio,
				DriftCost:      es.DriftCost,
			})
		}
		for _, ts := range drift.TangentSeqs {
			dj.TangentSeqs = append(dj.TangentSeqs, TangentSeqJSON{
				StartIdx:           ts.StartIdx,
				EndIdx:             ts.EndIdx,
				TargetRepo:         ts.TargetRepo,
				TokenCost:          ts.TokenCost,
				DollarCost:         ts.DollarCost,
				ReExplanationFiles: ts.ReExplanationFiles,
			})
		}
		out.ScopeDrift = dj
	}

	// Ghost context
	if stats.GhostReport != nil && stats.GhostReport.TotalGhosts > 0 {
		gj := &GhostReportJSON{
			TotalGhosts: stats.GhostReport.TotalGhosts,
		}
		for _, g := range stats.GhostReport.Files {
			gj.Files = append(gj.Files, GhostFileJSON{
				Path:            g.Path,
				CompactionIndex: g.CompactionIndex,
				EpochModified:   g.EpochModified,
			})
		}
		out.GhostContext = gj
	}

	if stats.InputPurity != nil && stats.InputPurity.TotalResultTokens > 0 {
		out.InputPurity = stats.InputPurity
	}

	if stats.Integrity != nil && !stats.Integrity.Healthy {
		out.Integrity = stats.Integrity
	}

	return out
}

// cleanAllToJSON converts a CleanAllResult to JSON output.
func cleanAllToJSON(path string, r *editor.CleanAllResult) *CleanOutput {
	out := &CleanOutput{
		SessionID: filepath.Base(path),
	}

	addOp := func(typ string, count int) {
		if count > 0 {
			out.Operations = append(out.Operations, CleanOpJSON{
				Type:            typ,
				EntriesAffected: count,
			})
		}
	}

	addOp("progress_removal", r.ProgressRemoved)
	addOp("snapshot_removal", r.SnapshotsRemoved)
	addOp("sidechain_removal", r.SidechainsRemoved)
	addOp("tangent_removal", r.TangentsRemoved)
	addOp("failed_retry_removal", r.FailedRetries)
	addOp("stale_read_removal", r.StaleReadsRemoved)
	addOp("image_replacement", r.ImagesReplaced)
	addOp("separator_stripping", r.SeparatorsStripped)
	addOp("output_truncation", r.OutputsTruncated)

	totalEntries := r.ProgressRemoved + r.SnapshotsRemoved + r.SidechainsRemoved +
		r.TangentsRemoved + r.FailedRetries + r.StaleReadsRemoved
	totalModified := r.ImagesReplaced + r.SeparatorsStripped + r.OutputsTruncated

	out.Summary = CleanSummaryJSON{
		EntriesRemoved:  totalEntries,
		EntriesModified: totalModified,
		TokensSaved:     r.TotalTokensSaved,
		BytesSaved:      r.BytesBefore - r.BytesAfter,
	}

	return out
}

// cleanLiveToJSON converts a CleanLiveResult to JSON output.
func cleanLiveToJSON(path string, r *editor.CleanLiveResult) *CleanOutput {
	mode := "live"
	if r.ImagesReplaced > 0 || r.SeparatorsStripped > 0 || r.OutputsTruncated > 0 {
		mode = "live-aggressive"
	}

	out := &CleanOutput{
		SessionID: filepath.Base(path),
		Mode:      mode,
	}

	addOp := func(typ string, count int) {
		if count > 0 {
			out.Operations = append(out.Operations, CleanOpJSON{
				Type:            typ,
				EntriesAffected: count,
			})
		}
	}

	addOp("progress_removal", r.ProgressRemoved)
	addOp("snapshot_removal", r.SnapshotsRemoved)
	addOp("image_replacement", r.ImagesReplaced)
	addOp("separator_stripping", r.SeparatorsStripped)
	addOp("output_truncation", r.OutputsTruncated)

	totalEntries := r.ProgressRemoved + r.SnapshotsRemoved
	totalModified := r.ImagesReplaced + r.SeparatorsStripped + r.OutputsTruncated

	out.Summary = CleanSummaryJSON{
		EntriesRemoved:  totalEntries,
		EntriesModified: totalModified,
		TokensSaved:     r.TotalTokensSaved,
		BytesSaved:      r.BytesBefore - r.BytesAfter,
	}

	return out
}

// MarkOutput is the JSON output for a single mark operation.
type MarkOutput struct {
	UUID   string `json:"uuid"`
	Marker string `json:"marker,omitempty"`
	Phase  string `json:"phase,omitempty"`
	Label  string `json:"label,omitempty"`
	Action string `json:"action"`
}

// MarkListOutput is the JSON output for listing markers.
type MarkListOutput struct {
	Markers   map[string]string           `json:"markers"`
	Phases    map[string]string           `json:"phases,omitempty"`
	Bookmarks map[string]MarkBookmarkJSON `json:"bookmarks,omitempty"`
	Total     int                         `json:"total"`
}

// MarkBookmarkJSON is a bookmark entry for mark list JSON output.
type MarkBookmarkJSON struct {
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
}

// MarksOutputJSON is the JSON output for contextspectre marks.
type MarksOutputJSON struct {
	SessionID string          `json:"session_id"`
	Marks     []MarkEntryJSON `json:"marks"`
	Total     int             `json:"total"`
}

// MarkEntryJSON is a single mark/bookmark/commit-point row.
type MarkEntryJSON struct {
	UUID       string `json:"uuid"`
	Type       string `json:"type"`
	Label      string `json:"label,omitempty"`
	EntryIndex int    `json:"entry_index,omitempty"`
	LineNumber int    `json:"line_number,omitempty"`
	Epoch      int    `json:"epoch"`
}

// CollapseOutput is the JSON output for the collapse command.
type CollapseOutput struct {
	SessionID       string `json:"session_id"`
	CommitPointUUID string `json:"commit_point_uuid"`
	EntriesRemoved  int    `json:"entries_removed"`
	ChainRepairs    int    `json:"chain_repairs"`
	BytesSaved      int64  `json:"bytes_saved"`
	DryRun          bool   `json:"dry_run,omitempty"`
}

// SplitOutput is the JSON output for the split command.
type SplitOutput struct {
	SessionID        string          `json:"session_id"`
	From             int             `json:"from"`
	To               int             `json:"to"`
	EntriesExtracted int             `json:"entries_extracted"`
	TargetRepo       string          `json:"target_repo,omitempty"`
	TokenCost        int             `json:"token_cost"`
	DollarCost       float64         `json:"dollar_cost"`
	ReExplFiles      []string        `json:"re_explanation_files,omitempty"`
	OutputPath       string          `json:"output_path"`
	Cleaned          bool            `json:"cleaned"`
	CleanResult      *SplitCleanJSON `json:"clean_result,omitempty"`
}

// SplitCleanJSON holds the result of the --clean operation.
type SplitCleanJSON struct {
	EntriesRemoved int `json:"entries_removed"`
	ChainRepairs   int `json:"chain_repairs"`
}

// AmputateOutput is the JSON output for the amputate command.
type AmputateOutput struct {
	SessionID      string `json:"session_id"`
	Slug           string `json:"slug,omitempty"`
	From           int    `json:"from"`
	To             int    `json:"to"`
	EntriesRemoved int    `json:"entries_removed"`
	TokensSaved    int    `json:"tokens_saved"`
	ChainRepairs   int    `json:"chain_repairs"`
	DryRun         bool   `json:"dry_run"`
}

// DistillTopicJSON is a single topic in the distill topic list.
type DistillTopicJSON struct {
	Index       int     `json:"index"`
	SessionID   string  `json:"session_id"`
	SessionSlug string  `json:"session_slug,omitempty"`
	Summary     string  `json:"summary"`
	TimeStart   string  `json:"time_start,omitempty"`
	TimeEnd     string  `json:"time_end,omitempty"`
	UserTurns   int     `json:"user_turns"`
	EntryCount  int     `json:"entry_count"`
	FileCount   int     `json:"file_count"`
	TokenCost   int     `json:"token_cost"`
	DollarCost  float64 `json:"dollar_cost"`
}

// DistillTopicListJSON is the JSON output for --dry-run.
type DistillTopicListJSON struct {
	ProjectName string             `json:"project_name"`
	Sessions    int                `json:"sessions"`
	Topics      []DistillTopicJSON `json:"topics"`
	Total       int                `json:"total"`
	TotalTokens int                `json:"total_tokens"`
	TotalCost   float64            `json:"total_cost"`
}

// DistillOutputJSON is the JSON output for a completed distill.
type DistillOutputJSON struct {
	ProjectName     string             `json:"project_name"`
	TopicsIncluded  int                `json:"topics_included"`
	SessionsSpanned int                `json:"sessions_spanned"`
	TotalTokens     int                `json:"total_tokens"`
	TotalCost       float64            `json:"total_cost"`
	OutputPath      string             `json:"output_path"`
	FullContent     bool               `json:"full_content"`
	Topics          []DistillTopicJSON `json:"topics"`
}

// UniteSectionJSON is a single section in the unite section list.
type UniteSectionJSON struct {
	Index         int    `json:"index"`
	SourceFile    string `json:"source_file"`
	Heading       string `json:"heading"`
	Summary       string `json:"summary"`
	TokenEstimate int    `json:"token_estimate"`
	IsDuplicate   bool   `json:"is_duplicate,omitempty"`
	DuplicateOf   int    `json:"duplicate_of,omitempty"`
}

// UniteListJSON is the JSON output for --dry-run.
type UniteListJSON struct {
	Sections    []UniteSectionJSON `json:"sections"`
	Total       int                `json:"total"`
	TotalTokens int                `json:"total_tokens"`
	SourceFiles int                `json:"source_files"`
	Duplicates  int                `json:"duplicates"`
}

// UniteOutputJSON is the JSON output for a completed unite.
type UniteOutputJSON struct {
	SectionsIncluded int                `json:"sections_included"`
	SourceFiles      int                `json:"source_files"`
	TotalTokens      int                `json:"total_tokens"`
	OutputPath       string             `json:"output_path"`
	Sections         []UniteSectionJSON `json:"sections"`
}

// VectorItemJSON is a single vector item for JSON output.
type VectorItemJSON struct {
	Text       string `json:"text"`
	Source     string `json:"source"`
	SourceType string `json:"source_type"`
	Epoch      int    `json:"epoch"`
}

// VectorOutputJSON is the JSON output for the vector command.
type VectorOutputJSON struct {
	ProjectName     string           `json:"project_name"`
	SnapshotDate    string           `json:"snapshot_date"`
	SessionsScanned int              `json:"sessions_scanned"`
	Decisions       []VectorItemJSON `json:"decisions"`
	Constraints     []VectorItemJSON `json:"constraints"`
	Questions       []VectorItemJSON `json:"questions"`
	Files           []string         `json:"files"`
}

// ContinuityOutputJSON is the JSON output for the continuity command.
type ContinuityOutputJSON struct {
	ProjectName      string                  `json:"project_name"`
	SessionsScanned  int                     `json:"sessions_scanned"`
	RepeatedFiles    []RepeatedFileJSON      `json:"repeated_files"`
	RepeatedTexts    []RepeatedTextJSON      `json:"repeated_texts"`
	RepeatTopics     []RepeatTopicJSON       `json:"repeat_topics,omitempty"`
	Suggestions      []ContinuitySuggestJSON `json:"suggestions,omitempty"`
	TotalFileReads   int                     `json:"total_file_reads"`
	UniqueFileReads  int                     `json:"unique_file_reads"`
	TotalTextBlocks  int                     `json:"total_text_blocks"`
	UniqueTextBlocks int                     `json:"unique_text_blocks"`
	ContinuityIndex  float64                 `json:"continuity_index"`
	TotalFileTokens  int                     `json:"total_repeated_file_tokens"`
	TotalTextTokens  int                     `json:"total_repeated_text_tokens"`
	TotalTaxTokens   int                     `json:"total_tax_tokens"`
	TotalFileCost    float64                 `json:"total_repeated_file_cost"`
	TotalTextCost    float64                 `json:"total_repeated_text_cost"`
	TotalTaxCost     float64                 `json:"total_tax_cost"`
}

// RepeatedFileJSON is a single repeated file for JSON output.
type RepeatedFileJSON struct {
	Path            string   `json:"path"`
	SessionCount    int      `json:"session_count"`
	ReadCount       int      `json:"read_count"`
	RedundantReads  int      `json:"redundant_reads"`
	Sessions        []string `json:"sessions"`
	EstimatedTokens int      `json:"estimated_tokens"`
	EstimatedCost   float64  `json:"estimated_cost"`
}

// RepeatedTextJSON is a single repeated text block for JSON output.
type RepeatedTextJSON struct {
	Text            string   `json:"text"`
	CharCount       int      `json:"char_count"`
	SessionCount    int      `json:"session_count"`
	ReadCount       int      `json:"read_count"`
	RedundantReads  int      `json:"redundant_reads"`
	Sessions        []string `json:"sessions"`
	EstimatedTokens int      `json:"estimated_tokens"`
	EstimatedCost   float64  `json:"estimated_cost"`
}

// RepeatTopicJSON is a repeated file cluster for continuity output.
type RepeatTopicJSON struct {
	Files           []string `json:"files"`
	SessionCount    int      `json:"session_count"`
	EstimatedTokens int      `json:"estimated_tokens"`
	EstimatedCost   float64  `json:"estimated_cost"`
}

// ContinuitySuggestJSON is a continuity suggestion row.
type ContinuitySuggestJSON struct {
	Path            string  `json:"path"`
	SessionCount    int     `json:"session_count"`
	EstimatedTokens int     `json:"estimated_tokens"`
	EstimatedCost   float64 `json:"estimated_cost"`
	Reason          string  `json:"reason"`
}

// SidechainOutputJSON is the JSON output for sidechains and repair dry-runs.
type SidechainOutputJSON struct {
	TotalEntries    int                  `json:"total_entries"`
	TotalTokens     int                  `json:"total_tokens"`
	GroupCount      int                  `json:"group_count"`
	RepairableCount int                  `json:"repairable_count"`
	PruneOnlyCount  int                  `json:"prune_only_count"`
	Entries         []SidechainEntryJSON `json:"entries"`
}

// SidechainEntryJSON is a single sidechain entry.
type SidechainEntryJSON struct {
	EntryIndex      int      `json:"entry_index"`
	LineNumber      int      `json:"line_number"`
	UUID            string   `json:"uuid,omitempty"`
	ParentUUID      string   `json:"parent_uuid,omitempty"`
	ToolUseID       string   `json:"tool_use_id,omitempty"`
	TokenCost       int      `json:"token_cost"`
	Preview         string   `json:"preview,omitempty"`
	Reasons         []string `json:"reasons"`
	Classification  string   `json:"classification"`
	ReconnectParent string   `json:"reconnect_parent,omitempty"`
}

// ProjectAliasJSON is a single project alias for JSON output.
type ProjectAliasJSON struct {
	Name     string   `json:"name"`
	Paths    []string `json:"paths"`
	Sessions int      `json:"sessions"`
}

// ProjectListOutput is the JSON output for the project list command.
type ProjectListOutput struct {
	Aliases []ProjectAliasJSON `json:"aliases"`
}

// SearchHitJSON is a single search match for JSON output.
type SearchHitJSON struct {
	SessionID  string `json:"session_id"`
	Slug       string `json:"slug,omitempty"`
	Project    string `json:"project"`
	EntryIndex int    `json:"entry_index"`
	Timestamp  string `json:"timestamp"`
	Role       string `json:"role"`
	Snippet    string `json:"snippet"`
}

// SearchOutputJSON is the JSON output for the search command.
type SearchOutputJSON struct {
	Query    string          `json:"query"`
	Total    int             `json:"total_hits"`
	Sessions int             `json:"sessions_searched"`
	Matches  int             `json:"sessions_with_matches"`
	Hits     []SearchHitJSON `json:"hits"`
}

// ExportOutput is the JSON output for the export command.
type ExportOutput struct {
	SessionID        string          `json:"session_id"`
	BranchesExported int             `json:"branches_exported"`
	EntriesExtracted int             `json:"entries_extracted"`
	TokenCost        int             `json:"token_cost"`
	DollarCost       float64         `json:"dollar_cost"`
	OutputPath       string          `json:"output_path"`
	Wiped            bool            `json:"wiped,omitempty"`
	WipeResult       *SplitCleanJSON `json:"wipe_result,omitempty"`
}

// buildDistillTopicListJSON converts a TopicSet to a dry-run JSON output.
func buildDistillTopicListJSON(ts *analyzer.TopicSet) *DistillTopicListJSON {
	out := &DistillTopicListJSON{
		ProjectName: ts.ProjectName,
		Sessions:    len(ts.Sessions),
		Total:       len(ts.Topics),
		TotalTokens: ts.TotalTokens,
		TotalCost:   ts.TotalCost,
	}
	for i, t := range ts.Topics {
		tj := DistillTopicJSON{
			Index:       i,
			SessionID:   t.SessionID,
			SessionSlug: t.SessionSlug,
			Summary:     t.Branch.Summary,
			UserTurns:   t.Branch.UserTurns,
			EntryCount:  t.Branch.EntryCount,
			FileCount:   t.Branch.FileCount,
			TokenCost:   t.Branch.TokenCost,
			DollarCost:  t.CostDollars,
		}
		if !t.Branch.TimeStart.IsZero() {
			tj.TimeStart = t.Branch.TimeStart.Format(time.RFC3339)
		}
		if !t.Branch.TimeEnd.IsZero() {
			tj.TimeEnd = t.Branch.TimeEnd.Format(time.RFC3339)
		}
		out.Topics = append(out.Topics, tj)
	}
	if out.Topics == nil {
		out.Topics = []DistillTopicJSON{}
	}
	return out
}

// buildDistillOutputJSON converts a completed distill result to JSON.
func buildDistillOutputJSON(ts *analyzer.TopicSet, selectedIndices []int, result *editor.DistillResult) *DistillOutputJSON {
	out := &DistillOutputJSON{
		ProjectName:     ts.ProjectName,
		TopicsIncluded:  result.TopicsIncluded,
		SessionsSpanned: result.SessionsSpanned,
		TotalTokens:     result.TotalTokens,
		TotalCost:       result.TotalCost,
		OutputPath:      result.OutputPath,
		FullContent:     result.TopicsIncluded > 0 && len(selectedIndices) > 0,
	}
	for _, idx := range selectedIndices {
		t := ts.Topics[idx]
		tj := DistillTopicJSON{
			Index:       idx,
			SessionID:   t.SessionID,
			SessionSlug: t.SessionSlug,
			Summary:     t.Branch.Summary,
			UserTurns:   t.Branch.UserTurns,
			EntryCount:  t.Branch.EntryCount,
			FileCount:   t.Branch.FileCount,
			TokenCost:   t.Branch.TokenCost,
			DollarCost:  t.CostDollars,
		}
		if !t.Branch.TimeStart.IsZero() {
			tj.TimeStart = t.Branch.TimeStart.Format(time.RFC3339)
		}
		if !t.Branch.TimeEnd.IsZero() {
			tj.TimeEnd = t.Branch.TimeEnd.Format(time.RFC3339)
		}
		out.Topics = append(out.Topics, tj)
	}
	if out.Topics == nil {
		out.Topics = []DistillTopicJSON{}
	}
	return out
}
