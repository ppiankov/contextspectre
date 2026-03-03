# CLI & TUI Reference

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
| `contextspectre amputate <id> --last N --apply` | Surgically remove last N entries to unblock stuck sessions |
| `contextspectre split <id> --from N --to M` | Export entry range to markdown |
| `contextspectre mark <id> <uuid> keep` | Mark an entry to protect from cleanup |
| `contextspectre collapse <id> --commit-point <uuid>` | Collapse exploration above a commit point |
| `contextspectre fix <id>` | Diagnose and repair session issues |
| `contextspectre doctor` | Health check across all sessions |
| `contextspectre relocate --scan` | Find orphaned sessions after project moves |
| `contextspectre relocate --from <old> --to <new>` | Migrate sessions to new project path |
| `contextspectre version` | Print version, commit, and build date |

### Global flags

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
| 4 | Replace base64 images with text placeholders | `--images` | Aggressive |
| 4 | Strip decorative separator lines | `--separators` | Aggressive |
| 5 | Truncate large Bash outputs (keep first/last N lines) | `--truncate-output` | Aggressive |
| 6 | Remove failed tool retries | `--failed-retries` | No |
| 6 | Remove sidechain entries | `--sidechains` | No |
| 7 | Remove cross-repo tangent sequences | `--tangents` | No |

`--all` runs all 9. `--live` runs Tier 1-2. `--live --aggressive` runs Tier 1-5.
