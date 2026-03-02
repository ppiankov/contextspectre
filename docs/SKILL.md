# contextspectre

Claude Code conversation context manager. Shows context usage, predicts compaction, and enables selective cleanup to extend conversation lifespan.

## Install

```bash
brew install ppiankov/tap/contextspectre
```

From source:
```bash
go install github.com/ppiankov/contextspectre/cmd/contextspectre@latest
```

## Commands

### `contextspectre sessions`

List all conversation sessions with context usage.

```bash
contextspectre sessions --format json
```

**Flags:** `--format json`

**JSON schema:**
```json
{
  "sessions": [
    {
      "id": "string",
      "project": "string",
      "branch": "string",
      "messages": 0,
      "tokens": 0,
      "context_percent": 0.0,
      "compactions": 0,
      "file_size_bytes": 0,
      "last_modified": "2026-01-01T00:00:00Z",
      "active": false,
      "images": 0
    }
  ],
  "total": 0
}
```

**Exit codes:** 0 success, 1 error

### `contextspectre stats <session-id-or-path>`

Show context statistics for a session.

```bash
contextspectre stats abc123 --format json
```

**Flags:** `--format json`

**JSON schema:**
```json
{
  "session_id": "string",
  "context": {
    "tokens": 0,
    "percent": 0.0,
    "window": 200000,
    "turns_remaining": 0
  },
  "compactions": {
    "count": 0,
    "events": [
      {
        "line_index": 0,
        "from_tokens": 0,
        "to_tokens": 0,
        "tokens_drop": 0
      }
    ]
  },
  "messages": {
    "total": 0,
    "user": 0,
    "assistant": 0,
    "progress": 0,
    "snapshots": 0,
    "system": 0
  },
  "images": {
    "count": 0,
    "bytes_total": 0,
    "estimated_tokens": 0
  },
  "growth_rate": {
    "tokens_per_turn": 0.0,
    "since_last_compaction": false
  }
}
```

**Exit codes:** 0 success, 1 error

### `contextspectre clean <session-id-or-path>`

Clean a session by removing noise and replacing images. Always creates a backup first.

```bash
# Run all cleanup operations
contextspectre clean abc123 --all --format json

# Individual operations
contextspectre clean abc123 --images
contextspectre clean abc123 --progress
contextspectre clean abc123 --separators
contextspectre clean abc123 --snapshots
contextspectre clean abc123 --dedup-reads
contextspectre clean abc123 --truncate-output
contextspectre clean abc123 --failed-retries
contextspectre clean abc123 --sidechains
contextspectre clean abc123 --tangents
```

**Flags:**
- `--all` — run all cleanup operations
- `--images` — replace base64 images with placeholders
- `--progress` — remove progress messages
- `--separators` — strip decorative separator lines
- `--snapshots` — remove file-history-snapshot entries
- `--dedup-reads` — remove stale duplicate file reads
- `--truncate-output` — truncate large Bash outputs
- `--output-threshold N` — byte threshold for truncation (default 4096)
- `--keep-lines N` — lines to keep at start/end (default 10)
- `--failed-retries` — remove failed tool attempts that were retried
- `--sidechains` — remove sidechain entries
- `--tangents` — remove cross-repo tangent sequences
- `--format json`

**JSON schema (--all):**
```json
{
  "session_id": "string",
  "operations": [
    {
      "type": "string",
      "entries_affected": 0,
      "tokens_saved": 0,
      "bytes_saved": 0
    }
  ],
  "summary": {
    "entries_removed": 0,
    "entries_modified": 0,
    "tokens_saved": 0,
    "bytes_saved": 0
  }
}
```

**Exit codes:** 0 success, 1 error, 2 session active (read-only)

### `contextspectre doctor`

Check tool health and environment.

```bash
contextspectre doctor --format json
```

**Exit codes:** 0 all checks pass, 1 error

### `contextspectre version`

Print version information.

## What This Does NOT Do

- Does not interpret conversation semantics or "improve" Claude's behavior
- Does not modify Claude Code itself — works with JSONL files as an external tool
- Does not provide real-time monitoring — point-in-time analysis
- Does not connect to any cloud service — fully local, no network access
- Does not auto-clean — always shows what would change, requires explicit flags

## Agent Workflow Examples

```bash
# Find sessions nearing compaction
contextspectre sessions --format json | jq '.sessions[] | select(.context_percent > 80)'

# Get context stats for a specific session
contextspectre stats abc123 --format json | jq '.context.turns_remaining'

# Automated cleanup pipeline
contextspectre clean abc123 --all --format json | jq '.summary.tokens_saved'

# Health check
contextspectre doctor --format json | jq '.sessions.status'
```
