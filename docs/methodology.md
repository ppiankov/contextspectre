# Computation Methodology

This document specifies how ContextSpectre computes user-facing metrics.
Formulas here are derived from code paths in `internal/analyzer/`, `internal/jsonl/`, and command integrations.

## Notation

- `chars` = character count (UTF-8 bytes in current implementation)
- `tok` = estimated tokens
- `ctx` = current context tokens from usage fields
- `T_compact` = compaction threshold (`165000`)
- `W_ctx` = context window (`200000`)
- `r_in`, `r_out`, `r_cw`, `r_cr` = per-million token rates for input/output/cache-write/cache-read

Source: `internal/analyzer/analyzer.go` (`CompactionThreshold`, `ContextWindowSize`), `internal/analyzer/cost.go` (`ModelPricing`).

## Token Estimation

### Text and generic payloads

- `tok_text = floor(chars / 4)`
- Constant: `CharsPerToken = 4`

Source: `internal/analyzer/estimator.go` (`CharsPerToken`, `EstimateTokens`, `estimateContentTokens`).

### Image blocks

- `tok_image = floor(base64_length / 750)`
- Constant: `Base64BytesPerToken = 750`

Source: `internal/analyzer/estimator.go` (`Base64BytesPerToken`, `EstimateImageTokens`, `estimateContentTokens`).

### Context size from usage fields

- `ctx = input_tokens + cache_creation_input_tokens + cache_read_input_tokens`

Source: `internal/jsonl/types.go` (`Usage.TotalContextTokens`).

### Session-level heuristics in analyzers

Several analyzers estimate tokens from raw entry size:

- `tok_entry ≈ RawSize / 4`

Examples: `internal/analyzer/analyzer.go`, `internal/analyzer/recommend.go`, `internal/analyzer/continuity.go`, `internal/analyzer/tangents.go`.

## Cost Attribution

### Per-turn/session cost

For each assistant message usage record:

- `cost_turn = input_tokens*r_in + output_tokens*r_out + cache_write_tokens*r_cw + cache_read_tokens*r_cr`

Session total:

- `cost_total = Σ(cost_turn)`
- `cost_per_turn = cost_total / turns` (when `turns > 0`)

Source: `internal/analyzer/cost.go` (`CalculateCost`).

### Per-model breakdown

- Costs are aggregated by `message.model`.
- Primary model is the highest-cost contributor.

Source: `internal/analyzer/cost.go` (`CalculateCost`, `PricingForModel`).

### Cost velocity

- `cost_per_hour = cost_total / session_duration_hours`

Source: `internal/commands/stats.go` (session cost section), `internal/commands/output.go` (`buildStatsOutput`).

### Cost alert

- Trigger condition: `cost_total >= cost_alert_threshold`.
- Threshold from config (`cost-alert`), `0` disables.

Source: `internal/commands/config.go` (`loadCostAlertThreshold`, `printCostAlert`).

## Compaction Detection

A compaction event is detected when consecutive assistant context tokens drop by more than `50000`:

- `if prev_ctx - ctx > 50000 => compaction event`

Event payload:

- before tokens, after tokens, dropped tokens, line index.

Source: `internal/analyzer/analyzer.go` (`CompactionDropThreshold`, `Analyze`).

## Turn Projection

### Main analyzer projection

Token growth rate:

- Post-compaction sessions: `growth = (ctx_current - ctx_post_compaction) / turns_since_compaction`
- No-compaction sessions: `growth = ctx_current / conversational_turns`

Turns remaining:

- `turns_remaining = floor((T_compact - ctx_current) / growth)` when `growth > 0`
- `0` if already over threshold, `-1` if unknown.

Source: `internal/analyzer/analyzer.go` (`Analyze`, `CompactionDistance`).

### Fast status-line projection

- `avg_tokens_per_turn = ctx_current / assistant_turns`
- `turns_remaining = floor((T_compact - ctx_current) / avg_tokens_per_turn)`

Source: `internal/commands/statusline.go` (`computeStatusLine`).

## Signal / Noise Classification

### Noise construction

`Recommend()` aggregates recoverable categories:

- Progress: `ProgressTokens` (`RawSize/4`)
- Snapshots: `SnapshotBytesTotal/4`
- Stale reads: duplicate read analyzer output
- Images: `ImageBytesTotal/750`
- Large outputs: large tool_result bytes (`/4`)
- Failed retries
- Sidechains
- Tangents

Source: `internal/analyzer/recommend.go`, supporting analyzers under `internal/analyzer/`.

### Signal and grade

From `ComputeHealth()`:

- `noise_tokens = rec.TotalTokens`
- `signal_tokens = max(total_tokens - noise_tokens, 0)`
- `signal_percent = signal_tokens / total_tokens * 100`

Grade thresholds in current code:

- `A`: `signal_percent > 90`
- `B`: `> 75`
- `C`: `> 60`
- `D`: `> 40`
- `F`: otherwise

Source: `internal/analyzer/health.go` (`ComputeHealth`, `gradeFromPercent`).

### Session entropy (composite decay)

`CalculateEntropy()`:

```text
entropy = (1 - signal_ratio) * 0.30
        + compaction_pressure * 0.25
        + drift_ratio * 0.20
        + orphan_ratio * 0.15
        + compression_loss * 0.10
score = clamp(entropy, 0..1) * 100
```

Where:

- `compaction_pressure = current_tokens / 165000`
- `orphan_ratio = orphan_tokens / total_tokens`
- `compression_loss = min(compaction_count,10)/10`

Source: `internal/analyzer/entropy.go` (`CalculateEntropy`).

### Cleanup cadence score

`AssessCleanupCadence()` weighted score:

- Noise axis: `clamp(noise_ratio/0.30) * 40`
- Compaction axis: `compactionProximityScore(turns_left) * 30`
- Growth axis: `clamp(token_growth_rate/2000) * 20`
- Since-cleanup axis: `clamp(turns_since_cleanup/50) * 10`

Status bands in current code:

- `score >= 70` => `overdue`
- `score >= 40` => `due`
- else `clean`

Plus explicit overrides:

- `noise_ratio > 30%` => overdue
- `noise_ratio > 15%` => due
- `turns_left < 10 and noise_tokens > 5000` => overdue

Source: `internal/analyzer/cadence.go` (`AssessCleanupCadence`).

## Scope Drift

Scope is computed from referenced file paths relative to session CWD.

Per epoch:

- `drift_ratio = out_scope / (in_scope + out_scope)`
- `drift_cost = sum(assistant turn costs for out-of-scope entries)`

Tangent sequences are contiguous out-of-scope blocks with additional token/cost attribution.

Source: `internal/analyzer/drift.go` (`AnalyzeScopeDrift`, `computeEpochScopes`, `detectTangentSequences`).

## Budget Protection

When `weekly_budget_limit` is configured, risk is computed from:

- compaction proximity (`turns_until_compaction`)
- noise ratio (`noise_tokens / current_tokens`)
- weekly budget remaining percent

Action ranking uses estimated savings for:

- clean noise now
- split tangent range
- start new session with branch export
- offload mechanical work to cheaper agents or CI

Source: `internal/analyzer/budget.go` (`AssessBudgetRisk`), `internal/commands/budget_helpers.go`.

## Continuity Tax (Cross-session)

`AnalyzeContinuity()` computes repeated work across sessions:

- Repeated files: redundant token estimate from repeated reads
- Repeated text blocks: normalized repeated prompts/explanations
- `continuity_index = average(unique/total over available file/text dimensions) * 100`

Cost conversion:

- File re-read cost uses cache-read pricing (`r_cr`)
- Repeated text cost uses input pricing (`r_in`)

Source: `internal/analyzer/continuity.go` (`AnalyzeContinuity`, `computeContinuityIndex`).

## Savings Attribution

Cleanup savings are recorded as conservative avoided cache-read cost:

- `avoided_tokens = tokens_removed * turns_remaining`
- `avoided_cost = cache_read_cost(model, avoided_tokens)`

Image correction in savings path:

- Non-image removed bytes use `/4`
- Image removed bytes use `/750`

Source: `internal/commands/clean.go` (`recordCleanupSavings`), `internal/analyzer/cost.go` (`CacheReadCostForTokens`).

## Known Limitations

- `chars/4` and `base64/750` are heuristics, not model tokenizer outputs.
- Fast status-line projection uses lightweight estimates and may differ from full `stats`.
- Continuity and savings are estimates from transcript structure and published pricing, not billing ledger data.
- Scope drift is structural path analysis; it does not infer semantic intent.

## Prior Work And External Validation

- Knuth, D.E. "Claude's Cycles." Stanford CS, Feb 28 2026: <https://www-cs-faculty.stanford.edu/~knuth/papers/claude-cycles.pdf>
- Claude Code session format documentation: add official link when published.

ContextSpectre metrics are empirically derived from observed session data and deterministic rules in code, not learned models.
