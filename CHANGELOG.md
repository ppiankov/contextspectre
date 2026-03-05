# Changelog

All notable changes to this project will be documented in this file.

## [0.33.2] - 2026-03-05

### Fixed

- Watch mode Ctrl+C: goroutine-based dual-signal handler for reliable exit
- Messages panel: cap ghost files and archaeology lines to prevent header overflow

## [0.33.1] - 2026-03-05

### Fixed

- Watch mode sleep/wake resilience: wall-clock gap detection and ticker drain

## [0.33.0] - 2026-03-05

### Added

- Project reasoning graph command (`graph`) with structural edges
- TUI Vector Control panel with clean/split/export actions
- Onboarding polish: `status` alias for `summary`, `list` alias for `sessions`
- Sessions default cap: 20 most recent, `--all` and `--limit` flags
- Actionable error message for `clean` with no operation flags

## [0.32.0] - 2026-03-05

### Added

- TUI Vector Control panel (v key from sessions view)

## [0.31.0] - 2026-03-05

### Fixed

- Lint issues from codex batch (errcheck, staticcheck)

## [0.30.0] - 2026-03-05

### Added

- Computation methodology documentation (`docs/methodology.md`)

## [0.29.0] - 2026-03-04

### Added

- Watch mode safety: tier gating for live cleanup operations

## [0.28.0] - 2026-03-04

### Added

- Vector gauge: three-state health indicator (stable/degrading/unstable/emergency)
- `stats --health` flag for vector gauge display

### Fixed

- Branch drill-in navigation in TUI

## [0.27.0] - 2026-03-04

### Added

- Decision economics: Cost Per Decision (CPD), Turns To Convergence (TTC), Context Drift Rate (CDR)
- Three-metric reasoning health model in stats, summary, and TUI

## [0.26.0] - 2026-03-04

### Added

- Expert hygiene mode (`config set expert-mode true`): auto-clean tiers 1-3 on context pressure
- Smart watch mode: mtime-based polling with 5s check / 30s cooldown
- Session analytics log: snapshots, filtering, aggregation via `analytics` command

## [0.25.0] - 2026-03-04

### Added

- Smart watch mode with mtime-based polling
- Session analytics log with snapshot recording

## [0.24.0] - 2026-03-04

### Added

- Expert hygiene mode for automatic cleanup of safe tiers (1-3)
- Status line telemetry command with mtime-based caching

## [0.23.0] - 2026-03-04

### Added

- Watch mode documentation and tokenomics concept

## [0.22.0] - 2026-03-04

### Added

- `clean --active --all` for batch cleanup of all active sessions
- `clean --active --all --watch` for continuous cleanup loop

## [0.21.0] - 2026-03-04

### Fixed

- Right-aligned numeric columns in TUI (compaction, context %)
- Branch column auto-hide when no sessions have branches

## [0.20.0] - 2026-03-04

### Added

- Tabbed detail view with four panels: Overview, Messages, Cleanup, Ghost
- Panel switching with Tab and 1-4 keys

## [0.19.2] - 2026-03-04

### Fixed

- Inflated savings calculation: image byte correction via backup scan

## [0.19.1] - 2026-03-04

### Fixed

- Per-model cost breakdown in CalculateCost

## [0.19.0] - 2026-03-04

### Added

- Savings attribution with lifetime tracking and projected gains
- `savings` command for cleanup economics

## [0.18.0] - 2026-03-04

### Added

- Cost alert thresholds with `config set cost-alert` command
- `config` command (set/get/list) for persistent configuration
- Cost velocity ($/hour) in stats output

## [0.17.0] - 2026-03-04

### Added

- `gc` command for stale session garbage collection

## [0.16.0] - 2026-03-04

### Added

- `watch` command for real-time context stats tail
- Color-coded context bar with compaction detection
- Terminal bell alerts at configurable thresholds

## [0.15.0] - 2026-03-04

### Added

- `search` command for cross-session content search
- Search across user text, tool use, and tool results

## [0.14.0] - 2026-03-04

### Added

- Federated project aliasing (`project alias/list/remove`)
- Cross-path session grouping via `--project` flag

## [0.13.0] - 2026-03-04

### Added

- Architectural invariants documentation
- Phase 5 roadmap planning
- Extended glossary with 20+ new terms

## [0.12.0] - 2026-03-04

### Added

- Phase 4 concepts to terminology glossary

## [0.11.0] - 2026-03-04

### Added

- `continuity` command for cross-session re-explanation tax measurement
- Continuity index scoring (0-100)

## [0.10.0] - 2026-03-04

### Added

- Ghost context detection for stale compaction summaries
- Ghost panel in TUI detail view

## [0.9.0] - 2026-03-04

### Added

- `vector` command for project north star extraction
- Vector JSON output types

## [0.8.0] - 2026-03-04

### Added

- `unite` command for merging and deduplicating context files

## [0.7.0] - 2026-03-03

### Added

- `distill` command for cross-session topic extraction to portable markdown

## [0.6.0] - 2026-03-03

### Added

- README before/after showcase with screenshots
- Per-model cost breakdown (Opus/Sonnet/Haiku)

## [0.5.2] - 2026-03-03

### Fixed

- JSONL line buffer increased from 1MB to 10MB for large sessions

## [0.5.1] - 2026-03-03

### Added

- `amputate` command for surgical session unblocking (context deadlock recovery)

## [0.5.0] - 2026-03-03

### Added

- Session identity with slug and short UUID
- Responsive TUI columns with 3 width breakpoints

## [0.4.8] - 2026-03-03

### Fixed

- Image replacement uses text description instead of PNG placeholder

## [0.4.7] - 2026-03-03

### Added

- Branch export to markdown with optional wipe
- `export` command with branches sub-view

## [0.4.6] - 2026-03-03

### Added

- Commit points with canonical state extraction
- Reasoning phase markers (exploratory/decision/operational)

## [0.4.5] - 2026-03-03

### Added

- Keep markers with sidecar file persistence
- `mark` command for entry annotation

## [0.4.4] - 2026-03-03

### Added

- Vector health score with signal-to-noise ratio (A-F grades)
- Noise classification: progress, stale reads, tangents, sidechains

## [0.4.3] - 2026-03-03

### Added

- `split` command for tangent extraction to markdown

## [0.4.2] - 2026-03-03

### Added

- CWD-based session targeting (`--cwd` flag)
- `quick-clean --cwd` for directory-scoped cleanup

## [0.4.1] - 2026-03-03

### Added

- Scope drift detection per epoch
- Dollar-cost quantification of drift

## [0.4.0] - 2026-03-03

### Added

- Compaction epoch timeline view
- Compaction archaeology: files touched, tools used, compression ratio

## [0.3.1] - 2026-03-03

### Added

- TUI title bar, session grouping, and search filter
- Vim-style navigation (G, gg, Ctrl+d/u/f/b, H/M/L)
- In-panel search (/) with match cycling (n/N)
- Help overlay (?) with per-panel keybinding reference

## [0.3.0] - 2026-03-02

### Added

- `quick-clean` command for one-step session cleanup
- `--project` flag for scoped operations
- `active` command for listing active sessions
- `summary` command for one-screen session overview

## [0.2.0] - 2026-03-02

### Added

- Post-compaction visual distinction: amber context bar for compacted sessions
- Ghost bar showing pre-compaction context level
- Compaction count indicator in session list (e.g., "3x")
- "Compacted from X%" label on context meter
- Turns estimate labeled "since last compaction" after compaction events
- Compaction detection in lightweight scanner (ScanLight)

## [0.1.0] - 2026-03-02

### Added

- Interactive TUI with session browser, message viewer, and context meter
- Compaction distance estimation (turns until next automatic compaction)
- Selective message deletion with impact prediction
- Image replacement (base64 → 1x1 transparent PNG placeholder)
- Progress message removal
- ParentUuid chain repair on deletion
- Mandatory backup before any modification
- Active session protection (read-only for recently modified files)
- CLI commands: `sessions`, `stats`, `clean`, `version`
- Streaming JSONL parser with 1MB buffer for large session files
- Token estimation (text, images, tool use)
- Compaction event detection from usage metadata
