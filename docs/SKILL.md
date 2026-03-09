# contextspectre

Reasoning hygiene layer for Claude Code. Shows context usage, predicts compaction, and enables selective cleanup to extend conversation lifespan.

Follows the [Agent-Native CLI Convention](https://ancc.dev).

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
- `--tombstone` — replace orphaned entries with placeholders instead of deleting (preserves Claude for Mac scroll-back)
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

### `contextspectre quick-clean`

Find and clean the most recent session. No session ID needed.

```bash
# Full cleanup on most recent session
contextspectre quick-clean

# Live cleanup on active session (Tier 1-3, safe between turns)
contextspectre quick-clean --live

# Aggressive live cleanup (Tier 1-5)
contextspectre quick-clean --live --aggressive

# Scoped to current directory
contextspectre quick-clean --cwd

# Tombstone mode (preserves Mac scroll-back)
contextspectre quick-clean --tombstone
```

**Flags:**
- `--live` — safe cleanup for active sessions (Tier 1-3)
- `--aggressive` — include Tier 4-5 (requires --live)
- `--cwd` — scope to current directory's project
- `--project <name>` — scope to a specific project
- `--tombstone` — replace orphaned entries with placeholders instead of deleting
- `--format json`

**Exit codes:** 0 success, 1 error

### `contextspectre fix <session-id-or-path>`

Diagnose and repair session problems (filter blocks, orphaned results, chain breaks).

```bash
# Dry run (report only)
contextspectre fix abc123

# Apply repairs
contextspectre fix abc123 --apply

# Apply with tombstone mode (replace orphans with placeholders)
contextspectre fix abc123 --apply --tombstone
```

**Flags:**
- `--apply` — apply repairs (default: dry-run)
- `--cwd` — use most recent session for current directory
- `--tombstone` — replace orphaned entries with placeholders instead of deleting

**Exit codes:** 0 success (or no issues), 1 error

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

### `contextspectre stats <session-id-or-path> --health`

Show vector gauge health state (stable/degrading/unstable/emergency).

```bash
contextspectre stats abc123 --health --format json
```

### `contextspectre injection --cwd`

Detect vector injection patterns in session content.

```bash
contextspectre injection --cwd --format json
```

**Exit codes:** 0 clean, 1 error, 2 findings detected

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
