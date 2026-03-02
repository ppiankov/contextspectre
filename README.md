# ContextSpectre

[![CI](https://github.com/ppiankov/contextspectre/actions/workflows/ci.yml/badge.svg)](https://github.com/ppiankov/contextspectre/actions/workflows/ci.yml)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Claude Code conversation context manager. Shows how close you are to compaction and lets you selectively trim what no longer matters.

## The problem

Claude Code conversations grow until automatic compaction triggers at ~165K tokens. Compaction summarizes and discards older context — you lose specificity, decisions blur, and reasoning drifts. The CLI shows a context meter (`ctx:41%`), but it's a single number with no history, no breakdown, and no way to control what stays. The Mac app shows nothing at all.

The deeper problem: not all context ages equally. You ask a sharp question to correct trajectory. You get alignment. You decide. Now that exploratory branch is complete — but it stays in context forever, subtly biasing future responses. That's not token waste. That's reasoning contamination.

## What it is

ContextSpectre reads Claude Code's local JSONL session files and shows you exactly what's inside:

- **Context meter** — current token usage, percentage of window, color-coded distance to compaction
- **Compaction history** — how many compactions occurred, token drops, growth rate
- **Turn estimate** — approximately how many turns remain before the next compaction
- **Message browser** — every message with estimated token cost, type, and preview
- **Selective deletion** — remove exploratory branches, progress noise, and stale images
- **Impact prediction** — before you delete, see the new context percentage and turns gained
- **Chain repair** — parentUuid links are automatically repaired when messages are removed
- **Mandatory backup** — every edit creates a `.bak` first, restorable with one key

## What it is NOT

- Not a conversation analyzer. It does not interpret semantics or judge your prompts.
- Not a Claude Code plugin. It reads local files independently — no API, no integration required.
- Not a general JSONL editor. It understands Claude Code's specific schema and nothing else.
- Not a monitoring daemon. It is a point-in-time tool you run when you need visibility.
- Not multi-vendor. It works with Claude Code's local session format. ChatGPT is server-side — there is nothing to edit.

## Philosophy

*Principiis obsta* — resist the beginnings.

LLM sessions have three states of reasoning: **exploratory** (temporary, unstable), **decision** (commit point), and **operational** (forward-only execution). Claude Code mixes them all permanently. Once you've decided, the scaffolding that got you there becomes noise — or worse, it pulls future reasoning off-vector.

ContextSpectre gives you the ability to collapse exploratory branches after a decision is made. Keep conclusions, remove scaffolding. That's not history editing — it's reasoning hygiene.

The tool presents evidence and lets you decide. It does not auto-trim, does not guess what matters, and does not modify files without your explicit confirmation and a backup.

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

# List sessions in terminal
contextspectre sessions

# Show context stats for a session
contextspectre stats <session-id>

# Batch clean: replace images and remove progress messages
contextspectre clean <session-id> --images --progress
```

## TUI

Running `contextspectre` without arguments opens the interactive TUI.

**Session browser** — lists all projects with message count, file size, context percentage, and last modified time. Active sessions (modified <60s ago) are marked and read-only.

**Message viewer** — shows every message in the session with type, estimated tokens, timestamp, and preview. The context meter at the top shows current usage, compaction count, and estimated turns remaining.

**Selection and deletion** — Space to select individual messages, `x` to select all progress messages, `i` to replace images with 1x1 placeholders. Selected messages show live impact prediction: token savings, new context percentage, and turns gained. `d` to delete with confirmation, `u` to undo from backup.

## CLI commands

| Command | Description |
|---------|-------------|
| `contextspectre` | Launch interactive TUI |
| `contextspectre sessions` | List all sessions with context stats |
| `contextspectre stats <id>` | Print detailed context analysis |
| `contextspectre clean <id> --images` | Replace base64 images with 1x1 placeholders |
| `contextspectre clean <id> --progress` | Remove all progress messages |
| `contextspectre version` | Print version, commit, and build date |

**Global flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--claude-dir` | `~/.claude` | Override Claude Code data directory |
| `--verbose` | `false` | Enable verbose logging |

## How it works

**Session discovery.** Scans `~/.claude/projects/` for JSONL session files. Reads `sessions-index.json` when available, falls back to glob.

**Token estimation.** Text: `len / 4`. Images: `len(base64) / 750`. Tool use: `(name + input) / 4`. These are estimates — actual tokenization varies, but they track closely enough for relative comparison.

**Compaction detection.** Monitors `message.usage` fields on assistant messages. A drop of >50K tokens between consecutive assistant messages indicates compaction. Growth rate is calculated from tokens accumulated since the last compaction.

**Compaction distance.** `(165,000 - current_tokens) / avg_tokens_per_turn` = estimated turns remaining.

**Chain repair.** When deleting message D, all entries where `parentUuid == D.uuid` get `parentUuid = D.parentUuid`. Walks up deletion chains to find the nearest surviving ancestor.

**Image replacement.** Replaces base64 image data with a 1x1 transparent PNG (68 bytes). The `{type: "image"}` structure is preserved — Claude sees `[image]` but the context cost drops from megabytes to bytes.

**Post-compaction warning.** Messages from before the last compaction are already excluded from active context. Deleting them saves file size but not context tokens. The TUI warns about this.

## Safety

- **Mandatory backup.** Every modification creates a `.bak` file first. Refuses to proceed if `.bak` already exists (preventing accidental double-edits).
- **Active session protection.** Sessions modified less than 60 seconds ago are read-only in the TUI.
- **Undo.** Restore from backup with `u` key in the TUI.
- **Dry-run.** Preview what would change without modifying anything.
- **Chain validation.** Verifies parentUuid integrity after edits.
- **No network.** ContextSpectre never phones home, never sends data anywhere.

## Architecture

```
contextspectre/
├── cmd/contextspectre/main.go      # Entry point (LDFLAGS version injection)
├── internal/
│   ├── commands/                   # Cobra CLI: sessions, stats, clean, version
│   ├── jsonl/                      # JSONL parser, types, writer (streaming, 1MB buffer)
│   ├── session/                    # Session discovery from ~/.claude/projects/
│   ├── analyzer/                   # Context stats, compaction detection, token estimation
│   │   ├── analyzer.go            # Session analysis + compaction distance
│   │   ├── estimator.go           # Per-message token estimation
│   │   └── impact.go              # Deletion impact prediction
│   ├── editor/                     # Message deletion, image replacement, chain repair
│   ├── backup/                     # .bak create/restore
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

## See also: CLI status line

Claude Code CLI supports a custom status line hook that shows context usage in real time:

```
Opus 4.6 | ctx:41% [########------------] | $11.13 | +1874/-2
```

This gives you live awareness while working. ContextSpectre complements it — the status line tells you *how full* you are; ContextSpectre tells you *what's filling it* and lets you trim.

To set up the status line, create `~/.claude/statusline.sh` that reads JSON from stdin and outputs a formatted line. Claude Code passes `context_window.used_percentage`, `model.display_name`, `cost.total_cost_usd`, and line change stats. Configure it in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "statusLine": {
      "type": "command",
      "command": "~/.claude/statusline.sh",
      "padding": 2
    }
  }
}
```

## Roadmap

- **Session repair** (`contextspectre fix`) — detect and remove content filter blocks, oversized images, orphaned tool results, and malformed entries. Diagnose first (`--dry-run`), fix on demand (`--apply`).
- **Post-compaction distinction** — visually differentiate "fresh session at 5%" from "just compacted from 82% to 5%."
- **Compaction imminent warning** — `⚠ COMPACTION IMMINENT` label at >85% context usage.
- **Image weight tracking** — per-message image cost display, warning when images dominate context budget.

## Known limitations

- **Token estimates are approximate.** The 4 chars/token heuristic is close but not exact. Actual BPE tokenization varies by content.
- **Compaction threshold is empirical.** The ~165K trigger point is observed behavior, not documented by Anthropic. It may change.
- **No real-time updates.** ContextSpectre reads the file once on open. It does not watch for changes during a live session.
- **Claude Code format only.** If Claude Code changes its JSONL schema, ContextSpectre needs updating.
- **Large files are slow to parse.** Sessions over 100MB take a few seconds to load. The parser is streaming but analysis is in-memory.
- **Active session edits are blocked.** Files modified in the last 60 seconds are read-only. Close or wait before editing.

## License

MIT License — see [LICENSE](LICENSE).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Issues and pull requests welcome.

Built by [Obsta Labs](https://obstalabs.dev).
