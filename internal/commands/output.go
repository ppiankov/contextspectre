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
}

// SessionsOutput is the JSON output for the sessions command.
type SessionsOutput struct {
	Sessions []SessionJSON `json:"sessions"`
	Total    int           `json:"total"`
}

// StatsOutput is the JSON output for the stats command.
type StatsOutput struct {
	SessionID   string           `json:"session_id"`
	Project     string           `json:"project,omitempty"`
	Context     ContextJSON      `json:"context"`
	Cost        *CostJSON        `json:"cost,omitempty"`
	EpochCosts  []EpochCostJSON  `json:"epoch_costs,omitempty"`
	Archaeology *ArchaeologyJSON `json:"archaeology,omitempty"`
	Compactions CompactionsJSON  `json:"compactions"`
	Messages    MessagesJSON     `json:"messages"`
	Images      ImagesJSON       `json:"images"`
	GrowthRate  GrowthRateJSON   `json:"growth_rate"`
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
	Model            string  `json:"model,omitempty"`
	TotalCost        float64 `json:"total_cost"`
	CostPerTurn      float64 `json:"cost_per_turn"`
	InputCost        float64 `json:"input_cost"`
	OutputCost       float64 `json:"output_cost"`
	CacheWriteCost   float64 `json:"cache_write_cost"`
	CacheReadCost    float64 `json:"cache_read_cost"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	TurnCount        int     `json:"turn_count"`
}

// EpochCostJSON holds cost for a single compaction epoch.
type EpochCostJSON struct {
	EpochIndex int     `json:"epoch_index"`
	TurnCount  int     `json:"turn_count"`
	PeakTokens int     `json:"peak_tokens"`
	TotalCost  float64 `json:"total_cost"`
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

// CleanOutput is the JSON output for the clean command.
type CleanOutput struct {
	SessionID  string           `json:"session_id"`
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

// buildStatsOutput converts analyzer stats to JSON output.
func buildStatsOutput(sessionID string, stats *analyzer.ContextStats) *StatsOutput {
	out := &StatsOutput{
		SessionID: sessionID,
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

	// Cost attribution
	if stats.Cost != nil && stats.Cost.TurnCount > 0 {
		out.Cost = &CostJSON{
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
		for _, ec := range stats.EpochCosts {
			out.EpochCosts = append(out.EpochCosts, EpochCostJSON{
				EpochIndex: ec.EpochIndex,
				TurnCount:  ec.TurnCount,
				PeakTokens: ec.PeakTokens,
				TotalCost:  ec.Cost.TotalCost,
			})
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
