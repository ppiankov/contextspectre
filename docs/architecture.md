# Architecture

## Directory structure

```
contextspectre/
├── cmd/contextspectre/main.go      # Entry point (LDFLAGS version injection)
├── internal/
│   ├── commands/                   # Cobra CLI: sessions, stats, clean, quick-clean, fix, doctor, relocate
│   ├── jsonl/                      # JSONL parser, types, writer (streaming, 10MB buffer)
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

## How it works

**Session discovery.** Scans `~/.claude/projects/` for JSONL session files. Both Claude Code CLI and Claude for Mac store sessions in the same directory with the same schema. Reads `sessions-index.json` when available, falls back to glob.

**Token estimation.** Text: `len / 4`. Images: `len(base64) / 750`. Tool use: `(name + input) / 4`. These are estimates — actual tokenization varies, but they track closely enough for relative comparison.

**Compaction detection.** Monitors `message.usage` fields on assistant messages. A drop of >50K tokens between consecutive assistant messages indicates compaction. The period between two compactions is a **compaction epoch** — the fundamental unit of reasoning history.

**Compaction distance.** `(165,000 - current_tokens) / avg_tokens_per_turn` = estimated turns remaining.

**Cost attribution.** Every assistant message carries `usage` fields: `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`. Combined with model pricing, this produces exact dollar cost per turn, per epoch, per session. No estimation — the data is in the JSONL.

**Chain repair.** When deleting message D, all entries where `parentUuid == D.uuid` get `parentUuid = D.parentUuid`. Walks up deletion chains to find the nearest surviving ancestor.

**Image replacement.** Replaces `{type: "image"}` blocks with `{type: "text", text: "[image removed by contextspectre]"}`. The image data is fully removed — no placeholder images that could cause API validation errors. Context cost drops from megabytes to a few bytes.

**Live cleanup.** Uses mtime-based race detection: check mtime before each sub-operation, abort and restore from backup if Claude wrote to the file during cleanup. Requires 2s idle period before starting. Safe because Claude Code re-reads the JSONL between turns.

## Safety model

- **Mandatory backup.** Every modification creates a `.bak` file first. Refuses to proceed if `.bak` already exists (preventing accidental double-edits).
- **Active session protection.** Sessions modified less than 60 seconds ago are read-only in the TUI.
- **Race detection.** Live cleanup aborts and restores the original file if the session is modified during cleanup.
- **Undo.** Restore from backup with `u` key in the TUI.
- **Chain validation.** Verifies parentUuid integrity after edits.
- **No network.** ContextSpectre never phones home, never sends data anywhere.
- **Multi-instance awareness.** Claude Code CLI and Claude for Mac can have separate sessions in the same project directory. Cleaning one session does not affect others, but you must identify the correct session before modifying it. See [Session Architecture](session-architecture.md) for details on how sessions are stored and how to avoid cleaning the wrong one.

## Design decisions

- **Streaming parser.** JSONL files can be hundreds of megabytes. The parser uses `bufio.Scanner` with a 1MB buffer, not `ioutil.ReadAll`.
- **Atomic writes.** File modifications write to a temp file then rename — no partial writes on crash.
- **No Claude API dependency.** Works entirely on local files. No network, no tokens consumed.
- **Bubbletea TUI.** Keyboard-driven, no mouse required. Lipgloss for styling.
- **Read-only by default.** The TUI shows data. Modifications require explicit selection and confirmation.
- **Tiered operations.** Cleanup operations are classified by safety level. Live mode only runs the safest tiers.
- **Structural, not semantic.** All analysis uses observable facts: token counts, file paths, UUID chains, usage fields. No ML, no probabilistic guessing.
