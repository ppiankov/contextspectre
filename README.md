# ContextSpectre

[![CI](https://github.com/ppiankov/contextspectre/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/contextspectre/actions/workflows/ci.yml)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Reasoning hygiene layer for Claude Code. Not a cleanup utility — a tool you open at every decision boundary, not just when context is full. See what fills your context, what it costs, cut what no longer matters, and carry forward what does.

## 100% drift epoch removed

A real session: 625 lines, 2.0MB, Signal F (255K noise tokens, $19.80). Epoch 1 was 100% out-of-scope work — a tangent into another repository that consumed 189K tokens and $11.69, displacing the original project context. After cleanup: 232 lines, 1.1MB, Signal A (0 noise, $6.12).

![After cleanup](assets/stats-after.png)

## The problem

Claude Code conversations grow until automatic compaction triggers at ~165K tokens. Compaction summarizes and discards older context — you lose specificity, decisions blur, and reasoning drifts. After 10+ compactions, Claude is working from a summary of a summary of a summary. The CLI shows a context meter (`ctx:41%`), but it's a single number with no history, no breakdown, and no way to control what stays.

The deeper problem: not all context ages equally. LLM sessions have three reasoning phases: **exploratory** (temporary, unstable), **decision** (commit point), and **operational** (forward-only execution). Claude Code mixes them all permanently. Once you've decided, the scaffolding that got you there becomes noise — or worse, it pulls future reasoning off-vector. That's not token waste. That's **reasoning contamination**.

The hidden problem: a single long session can cost hundreds of dollars. Most of that is cache reads — re-processing the same context every turn. A debugging detour that gets compacted away still cost real money. Without visibility into where tokens and dollars go, optimization is impossible.

And it's not limited to one session. Long projects span dozens of sessions across branches, refactors, and debugging threads. Users re-explain the same architecture, re-read the same files, re-state the same constraints in every new session. This **re-explanation tax** is invisible and cumulative. Valuable reasoning is scattered across session files with no way to find it, extract it, or carry it forward.

## What it is

ContextSpectre reads Claude Code's local JSONL session files — from both Claude Code CLI and Claude for Mac — and gives you visibility and control over what fills your context window:

**Shipped (v0.3.x):**

- **Context meter** — current token usage, percentage of window, color-coded distance to compaction
- **Compaction history** — compaction count, token drops, growth rate, post-compaction visual distinction
- **Turn estimate** — approximately how many turns remain before the next compaction
- **Session browser** — all projects grouped by name, with search/filter (`/`), context bars, and active session detection
- **Message browser** — every message with estimated token cost, type, and content preview
- **Selective deletion** — remove exploratory branches, progress noise, stale file reads, and oversized images
- **Impact prediction** — before you delete, see the new context percentage and turns gained
- **Live session cleanup** — safely clean active sessions between Claude's turns with mtime-based race detection
- **Batch cleanup** — `clean --all` runs 9 operations in one pass; `quick-clean` finds and cleans the most recent session automatically
- **Chain repair** — parentUuid links are automatically repaired when messages are removed
- **Session cost attribution** — actual dollar cost per session and per compaction epoch. Uses `message.usage` data with model pricing — no estimation, no heuristics
- **Mandatory backup** — every edit creates a `.bak` first, restorable with one key

**Planned:** Compaction archaeology, ghost context detection, epoch timeline, predictive cleanup, scope drift detection with split surgery, reasoning phase markers, conversation branch navigation, separation and amputation surgery, cross-session context distillation. See [Roadmap](#roadmap) for details.

## What it is NOT

- Not a conversation analyzer. It does not interpret semantics or judge your prompts.
- Not a Claude Code plugin. It reads local files independently — no API, no integration required.
- Not a general JSONL editor. It understands Claude Code's specific schema and nothing else.
- Not a monitoring daemon. It is a point-in-time tool you run when you need visibility.
- Not multi-vendor. It works with Claude Code's local session format (CLI and Mac). ChatGPT is server-side — there is nothing to edit.
- Not an AI summarizer. It extracts existing content. It does not generate new summaries.
- Not a cost optimizer. It exposes the hidden economics of reasoning. You decide what to do about it.

## Philosophy

*Principiis obsta* — resist the beginnings.

**Keep conclusions, remove scaffolding.** Exploratory reasoning is valuable while exploring. After a decision is made, it becomes dead weight that biases future responses. ContextSpectre lets you collapse exploration into decisions — that's not history editing, it's reasoning hygiene.

**Mirrors, not oracles.** The tool presents evidence and lets you decide. It does not auto-trim, does not guess what matters, and does not modify files without your explicit confirmation and a backup.

**Context distillation over context deletion.** The goal is not to make sessions smaller. It's to increase the signal-to-noise ratio of what Claude sees. Progress messages, stale file reads, failed retries, and decorative separators are pure noise. Decisions, constraints, and working code are pure signal.

**Expose the hidden economics of reasoning.** Tokens are abstract. Percentages are abstract. Dollars are visceral. "$32 for that debugging detour that got compacted away" changes behavior faster than "82% context usage" ever will.

**Structural detection over semantic guessing.** Every analysis uses observable facts — token counts, file paths, compaction boundaries, parentUuid chains, usage fields. No ML, no heuristics that guess meaning, no probabilistic classification. When the tool doesn't know, it says so.

**The historian, not the operator.** ContextSpectre does not run your sessions or tell you what to do next. It records what happened, shows what it cost, and lets you decide what to carry forward. The operator explores and decides. The historian preserves the decisions and discards the scaffolding.

## Installation

```bash
# Homebrew
brew install ppiankov/tap/contextspectre

# From source
git clone https://github.com/ppiankov/contextspectre.git
cd contextspectre && make build
```

## Quick start

```bash
# Launch the TUI (default — browse all sessions)
contextspectre

# Quick-clean the most recent session (one command, no session ID needed)
contextspectre quick-clean

# Live cleanup on an active session (safe between Claude's turns)
contextspectre quick-clean --live

# Show context stats
contextspectre stats <session-id>

# Run all cleanup operations
contextspectre clean <session-id> --all

# JSON output for scripting
contextspectre sessions --format json
```

## TUI

Running `contextspectre` without arguments opens the interactive TUI.

**Session browser** — sessions grouped by project with message count, file size, context bars, compaction count, and last modified time. Press `/` to search by project name, branch, or session ID. Active sessions (modified <60s ago) are highlighted and read-only.

**Message viewer** — every message in the session with type, estimated tokens, timestamp, and preview. The context meter at the top shows current usage, compaction history, and estimated turns remaining. Stale reads, failed retries, sidechains, and tangents are labeled.

**Selection and deletion** — Space to select individual messages, `x` to select all progress, `h` snapshots, `r` stale reads, `c` sidechains, `g` tangents, `i` to replace images, `s` to strip separators, `t` to truncate large outputs, `a` to clean all. Selected messages show live impact prediction: token savings, new context percentage, and turns gained. `d` to delete with confirmation, `u` to undo from backup.

## CLI commands

| Command | Description |
|---------|-------------|
| `contextspectre` | Launch interactive TUI |
| `contextspectre sessions` | List all sessions with context stats |
| `contextspectre stats <id>` | Print detailed context analysis |
| `contextspectre clean <id> --all` | Run all 9 cleanup operations |
| `contextspectre clean <id> --live` | Safe cleanup for active sessions (Tier 1-3) |
| `contextspectre clean <id> --live --aggressive` | Live cleanup including images/separators/truncation (Tier 1-5) |
| `contextspectre clean --auto` | Find and clean the most recent session |
| `contextspectre quick-clean` | Discover most recent session, clean it |
| `contextspectre quick-clean --project <name>` | Scoped to a specific project |
| `contextspectre quick-clean --live` | Live cleanup on most recent session |
| `contextspectre fix <id>` | Diagnose and repair session issues |
| `contextspectre doctor` | Health check across all sessions |
| `contextspectre relocate --scan` | Find orphaned sessions after project moves |
| `contextspectre relocate --from <old> --to <new>` | Migrate sessions to new project path |
| `contextspectre version` | Print version, commit, and build date |

**Global flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--claude-dir` | `~/.claude` | Override Claude Code data directory |
| `--format` | `text` | Output format (`text` or `json`) |
| `--verbose` | `false` | Enable verbose logging |

## Cleanup operations

| Tier | Operation | Flag | Safe for live? |
|------|-----------|------|----------------|
| 1 | Remove progress messages | `--progress` | Yes |
| 2 | Remove file-history-snapshots | `--snapshots` | Yes |
| 3 | Remove stale duplicate file reads | `--dedup-reads` | No |
| 4 | Replace base64 images with 1x1 placeholders | `--images` | Aggressive |
| 4 | Strip decorative separator lines | `--separators` | Aggressive |
| 5 | Truncate large Bash outputs (keep first/last N lines) | `--truncate-output` | Aggressive |
| 6 | Remove failed tool retries | `--failed-retries` | No |
| 6 | Remove sidechain entries | `--sidechains` | No |
| 7 | Remove cross-repo tangent sequences | `--tangents` | No |

`--all` runs all 9. `--live` runs Tier 1-2. `--live --aggressive` runs Tier 1-5.

## How it works

**Session discovery.** Scans `~/.claude/projects/` for JSONL session files. Both Claude Code CLI and Claude for Mac store sessions in the same directory with the same schema. Reads `sessions-index.json` when available, falls back to glob.

**Token estimation.** Text: `len / 4`. Images: `len(base64) / 750`. Tool use: `(name + input) / 4`. These are estimates — actual tokenization varies, but they track closely enough for relative comparison.

**Compaction detection.** Monitors `message.usage` fields on assistant messages. A drop of >50K tokens between consecutive assistant messages indicates compaction. The period between two compactions is a **compaction epoch** — the fundamental unit of reasoning history.

**Compaction distance.** `(165,000 - current_tokens) / avg_tokens_per_turn` = estimated turns remaining.

**Cost attribution.** Every assistant message carries `usage` fields: `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`. Combined with model pricing, this produces exact dollar cost per turn, per epoch, per session. No estimation — the data is in the JSONL.

**Chain repair.** When deleting message D, all entries where `parentUuid == D.uuid` get `parentUuid = D.parentUuid`. Walks up deletion chains to find the nearest surviving ancestor.

**Image replacement.** Replaces base64 image data with a 1x1 transparent PNG (68 bytes). The `{type: "image"}` structure is preserved — Claude sees `[image]` but the context cost drops from megabytes to bytes.

**Live cleanup.** Uses mtime-based race detection: check mtime before each sub-operation, abort and restore from backup if Claude wrote to the file during cleanup. Requires 2s idle period before starting. Safe because Claude Code re-reads the JSONL between turns.

## Safety

- **Mandatory backup.** Every modification creates a `.bak` file first. Refuses to proceed if `.bak` already exists (preventing accidental double-edits).
- **Active session protection.** Sessions modified less than 60 seconds ago are read-only in the TUI.
- **Race detection.** Live cleanup aborts and restores the original file if the session is modified during cleanup.
- **Undo.** Restore from backup with `u` key in the TUI.
- **Chain validation.** Verifies parentUuid integrity after edits.
- **No network.** ContextSpectre never phones home, never sends data anywhere.

## Architecture

```
contextspectre/
├── cmd/contextspectre/main.go      # Entry point (LDFLAGS version injection)
├── internal/
│   ├── commands/                   # Cobra CLI: sessions, stats, clean, quick-clean, fix, doctor, relocate
│   ├── jsonl/                      # JSONL parser, types, writer (streaming, 1MB buffer)
│   ├── session/                    # Session discovery, relocation, path encoding
│   ├── analyzer/                   # Context stats, compaction detection, token estimation,
│   │   │                           #   deletion impact, duplicate reads, failed retries, tangents
│   ├── editor/                     # Deletion, image replacement, separator stripping,
│   │   │                           #   output truncation, CleanAll orchestrator, CleanLive orchestrator
│   ├── safecopy/                   # .bak create/restore/clean
│   ├── tui/                        # Bubbletea TUI (sessions, messages, confirm views)
│   └── logging/                    # slog initialization
├── testdata/                       # Session fixtures for testing
├── Makefile
└── go.mod
```

Key design decisions:

- **Streaming parser.** JSONL files can be hundreds of megabytes. The parser uses `bufio.Scanner` with a 1MB buffer, not `ioutil.ReadAll`.
- **Atomic writes.** File modifications write to a temp file then rename — no partial writes on crash.
- **No Claude API dependency.** Works entirely on local files. No network, no tokens consumed.
- **Bubbletea TUI.** Keyboard-driven, no mouse required. Lipgloss for styling.
- **Read-only by default.** The TUI shows data. Modifications require explicit selection and confirmation.
- **Tiered operations.** Cleanup operations are classified by safety level. Live mode only runs the safest tiers.
- **Structural, not semantic.** All analysis uses observable facts: token counts, file paths, UUID chains, usage fields. No ML, no probabilistic guessing.

## See also: CLI status line

Claude Code CLI supports a custom status line hook that shows context usage in real time:

```
Opus 4.6 | ctx:41% [########------------] | $11.13 | +1874/-2
```

This gives you live awareness while working. ContextSpectre complements it — the status line tells you *how full* you are; ContextSpectre tells you *what's filling it*, *what it costs*, and lets you act on it.

## Workflow patterns

For large projects, separating exploratory reasoning from structured execution reduces context drift. One effective pattern:

- **Explore and design** in a conversational interface (Claude for Mac, or a fresh CLI session).
- **Execute structured work** in Claude Code CLI with focused, scoped prompts.
- **Use ContextSpectre at decision boundaries** — collapse exploration into decisions before continuing. The scaffolding that got you to the decision becomes noise once you've committed.

ContextSpectre does not require this workflow — it works with any Claude Code session. But it complements structured development particularly well.

## Roadmap

**Phase 1: Entropy control** (complete)
Control noise within a session. Remove progress messages, stale reads, failed retries, oversized images, decorative separators, and cross-repo tangents. Live cleanup between turns. Batch operations.

**Phase 2: Reasoning economics** (next)
Expose the hidden costs of reasoning. Session cost attribution from actual usage data. Compaction epoch timeline — git log for reasoning with cost, turns, and topic per epoch. Compaction archaeology — forensic view of what 165K tokens compressed to 250 characters. Predictive cleanup with turn-gain estimates.

**Phase 3: Reasoning navigation**
Turn the flat message list into a navigable structure. Scope drift detection — track when tool calls leave the session's project directory, flag tangent sequences, quantify re-explanation tax in dollars, and offer split surgery to extract tangents into portable markdown before compaction erases the original context. Segment sessions into conversation branches by compaction boundaries and time gaps. Reasoning phase markers (exploratory/decision/operational). Keep markers and commit points for human-driven intent labeling. Stale branch detection. Vector health score showing signal/noise ratio.

**Phase 4: Selective continuity**
Extract healthy conversation branches into portable markdown context files. Separation surgery: mark branches worth continuing, export them, optionally prune from the source session. Amputation surgery for content-filter false positives. Start a new Claude Code session with `"read docs/branch-export.md"` — full context, zero compactions.

**Phase 5: Context distillation**
Synthesize across all sessions for a project. Unite multiple branch exports into a single context file with deduplication, conflict detection, and token budgeting. Vector snapshot — extract only canonical decisions and constraints as a project north star. Cross-session continuity score to measure re-explanation tax. Ghost context detection to flag stale compaction summaries.

## Known limitations

- **Token estimates are approximate.** The 4 chars/token heuristic is close but not exact. Actual BPE tokenization varies by content.
- **Cost estimates use published pricing.** Dollar figures are calculated from API pricing tables, not from actual Anthropic invoices. They are close but not authoritative.
- **Compaction threshold is empirical.** The ~165K trigger point is observed behavior, not documented by Anthropic. It may change.
- **No real-time updates.** ContextSpectre reads the file once on open. It does not watch for changes during a live session.
- **Claude Code format only.** If Claude Code changes its JSONL schema, ContextSpectre needs updating. Works with both CLI and Mac desktop sessions.
- **Large files are slow to parse.** Sessions over 100MB take a few seconds to load. The parser is streaming but analysis is in-memory.
- **Branch detection is structural, not semantic.** Branches are identified by compaction boundaries and time gaps, not by understanding what was discussed.

## License

MIT License — see [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Issues and pull requests welcome.

Built by [Obsta Labs](https://obstalabs.dev). Not affiliated with or endorsed by Anthropic.
