# CLI & TUI Reference

## TUI

Running `contextspectre` without arguments opens the interactive TUI.

**Session browser** — sessions sorted by last modified with responsive columns that adapt to terminal width. Three breakpoints: wide (>160 cols), medium (120-160), narrow (<120). Columns: project, slug, session ID, branch, messages, size, context %, compaction count, signal grade, cost, modified time. Active sessions show a `●` indicator. Press `s` to cycle sort column (modified → cost → context → signal → size). Press `/` to search.

**Detail view** — drill into a session with four tabbed panels: Overview, Messages, Cleanup, Ghost. Switch panels with `Tab` or `1-4`. Overview shows stats, cost, compactions, decision economics, and vector gauge. Messages panel shows every entry with type, tokens, timestamp, and preview. Cleanup panel shows noise breakdown. Ghost panel shows stale compaction references.

**Messages panel** — context meter at the top shows current usage, compaction history (capped at 3 with overflow hint), ghost file references (capped at 3), and estimated turns remaining. Entries are labeled by type: stale reads, failed retries, sidechains, tangents. Space to select individual messages. Bulk selectors: `x` progress, `h` snapshots, `r` stale reads, `c` sidechains, `g` tangents, `i` images, `s` separators, `t` truncate outputs, `a` all. Selected messages show live impact prediction: token savings, new context percentage, and turns gained. `d` to delete with confirmation, `u` to undo from backup.

## TUI keybindings

### Navigation (all panels)

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `G` / `End` | Jump to last entry |
| `gg` / `Home` | Jump to first entry (double-tap g) |
| `Ctrl+D` | Half page down |
| `Ctrl+U` | Half page up |
| `Space` / `Ctrl+F` | Full page down (sessions/overview) |
| `Ctrl+B` | Full page up |
| `H` | Jump to top of visible screen |
| `M` | Jump to middle of visible screen |
| `L` | Jump to bottom of visible screen |
| `/` | Activate search |
| `n` / `N` | Next / previous search match |
| `?` | Toggle help overlay |
| `q` / `Esc` | Back / quit |

### Sessions view

| Key | Action |
|-----|--------|
| `Enter` | Open session detail |
| `s` | Cycle sort column |
| `v` | Vector control panel |

### Detail view

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Next / previous panel |
| `1` | Overview panel |
| `2` | Messages panel |
| `3` | Cleanup panel |
| `4` | Ghost panel |
| `Enter` | Go to Messages |

### Messages panel

| Key | Action |
|-----|--------|
| `Space` | Toggle selection on current entry |
| `x` | Select all progress messages |
| `h` | Select all snapshots |
| `r` | Select all stale reads |
| `c` | Select all sidechains |
| `g` | Select all tangents |
| `i` | Replace all images |
| `s` | Strip all separators |
| `t` | Truncate large outputs |
| `a` | Select all noise (all tiers) |
| `d` | Delete selected (with confirmation) |
| `u` | Undo last deletion from backup |
| `K` | Toggle keep marker |
| `N` | Toggle noise marker |
| `p` | Create commit point |
| `!` | Amputate range |
| `e` | Show epoch timeline |
| `1-3` | Set reasoning phase (exploratory/decision/operational) |
| `0` | Clear phase |

### Vector Control panel

| Key | Action |
|-----|--------|
| `C` | Clean session |
| `S` | Split tangent range |
| `E` | Export decisions |

## CLI commands

### Session discovery

| Command | Description |
|---------|-------------|
| `contextspectre sessions` | List all sessions (default: 20 most recent) |
| `contextspectre list` | Alias for `sessions` |
| `contextspectre sessions --all` | Show all sessions (no limit) |
| `contextspectre sessions --limit 50` | Show N most recent sessions |
| `contextspectre sessions --active` | Show sessions modified in last 5 minutes |
| `contextspectre sessions --project <name>` | Filter by project name or alias |
| `contextspectre active` | Show currently active sessions with signal grades |
| `contextspectre active --since 30m` | Custom activity window |
| `contextspectre active --quiet` | One-line summary output |

### Context analysis

| Command | Description |
|---------|-------------|
| `contextspectre stats <id>` | Detailed context analysis |
| `contextspectre stats --cwd` | Auto-detect session from current directory |
| `contextspectre stats <id> --epochs` | Show epoch timeline |
| `contextspectre stats <id> --scope` | Show scope drift analysis |
| `contextspectre stats <id> --health` | Show vector gauge health |
| `contextspectre stats <id> --record` | Record an analytics snapshot |
| `contextspectre summary <id>` | One-screen session summary |
| `contextspectre summary --cwd` | Auto-detect from current directory |
| `contextspectre summary --quiet` | Single-line output for hooks |
| `contextspectre status` | Alias for `summary` |
| `contextspectre watch <id>` | Real-time context stats tail |
| `contextspectre watch --cwd` | Auto-detect from current directory |
| `contextspectre watch --interval 10` | Custom refresh interval |
| `contextspectre watch --alert 80` | Terminal bell at context threshold |
| `contextspectre timeline <id>` | Chronological reasoning timeline |

### Cleanup

| Command | Description |
|---------|-------------|
| `contextspectre clean <id> --all` | Run all 9 cleanup operations |
| `contextspectre clean <id> --live` | Safe cleanup for active sessions (Tier 1-3) |
| `contextspectre clean <id> --live --aggressive` | Live cleanup including Tier 4-5 |
| `contextspectre clean --auto` | Find and clean most recent session |
| `contextspectre clean --active --all` | Batch clean all active sessions |
| `contextspectre clean --active --all --watch` | Continuous cleanup loop |
| `contextspectre clean --active --all --watch --interval 30` | Fixed-interval watch |
| `contextspectre quick-clean` | Find and clean most recent session |
| `contextspectre quick-clean --live` | Live cleanup on most recent session |
| `contextspectre quick-clean --project <name>` | Scoped to a specific project |
| `contextspectre quick-clean --cwd` | Scoped to current directory |

### Surgery

| Command | Description |
|---------|-------------|
| `contextspectre amputate <id> --last N --apply` | Remove last N entries to unblock deadlocked sessions |
| `contextspectre split <id> --from N --to M --output file.md` | Export entry range to markdown |
| `contextspectre split <id> --from N --to M --output file.md --clean` | Export and remove entries |
| `contextspectre collapse <id> --commit-point <uuid>` | Collapse exploration above a commit point |
| `contextspectre fix <id>` | Diagnose and repair session problems |
| `contextspectre repair <id>` | Repair or prune orphaned sidechains |
| `contextspectre sidechains <id>` | Report structural sidechains |

### Markers and phases

| Command | Description |
|---------|-------------|
| `contextspectre mark <id> <uuid> keep` | Protect entry from cleanup |
| `contextspectre mark <id> <uuid> candidate` | Mark as cleanup candidate |
| `contextspectre mark <id> <uuid> noise` | Mark as noise |
| `contextspectre mark <id> <uuid> checkpoint` | Create checkpoint |
| `contextspectre mark <id> <uuid> milestone` | Mark milestone |
| `contextspectre mark <id> --phase <uuid> exploratory` | Set reasoning phase |
| `contextspectre mark <id> --phase <uuid> decision` | Set decision phase |
| `contextspectre mark <id> --phase <uuid> operational` | Set operational phase |
| `contextspectre mark <id> --list` | List all markers |
| `contextspectre marks <id>` | List all marks, bookmarks, and commit points |

### Data export and transformation

| Command | Description |
|---------|-------------|
| `contextspectre export <id>` | Export branches to markdown |
| `contextspectre export tasks <id>` | Extract actionable work items |
| `contextspectre export decisions <id>` | Export decisions from commit points |
| `contextspectre export timeline <id>` | Export session timeline as artifact |
| `contextspectre distill <id>` | Compress session into concise markdown |
| `contextspectre unite file1.md file2.md` | Deduplicate and merge markdown files |
| `contextspectre vector --project <name>` | Extract project north star document |
| `contextspectre vector --cwd` | Vector from current directory's sessions |

### Cross-session analysis

| Command | Description |
|---------|-------------|
| `contextspectre continuity --project <name>` | Measure re-explanation tax across sessions |
| `contextspectre search <query> --project <name>` | Search session content |
| `contextspectre search <query> --cwd` | Search current directory's sessions |
| `contextspectre search <query> --all` | Search across all sessions |
| `contextspectre conflicts --project <name>` | Detect structural decision conflicts |
| `contextspectre graph --project <name>` | Show structural reasoning graph |

### Project management

| Command | Description |
|---------|-------------|
| `contextspectre project alias <name> <path>` | Create alias for project path |
| `contextspectre project list` | List all aliases with session counts |
| `contextspectre project remove <name>` | Remove an alias |
| `contextspectre memory build --project <name>` | Synthesize project memory from sessions |
| `contextspectre sync --project <name>` | Merge distilled decisions into CLAUDE.md |

### Telemetry and configuration

| Command | Description |
|---------|-------------|
| `contextspectre status-line --cwd` | Fast-path telemetry for status line hooks |
| `contextspectre status-line --path <file>` | Telemetry from specific JSONL file |
| `contextspectre status-line --stdin` | Read session from stdin |
| `contextspectre status-line --format json` | Output format: tab, shell, human, json |
| `contextspectre config set <key> <value>` | Set configuration value |
| `contextspectre config get <key>` | Get configuration value |
| `contextspectre config list` | List all configuration values |
| `contextspectre usage` | Show weekly usage telemetry |
| `contextspectre analytics` | Show aggregated session analytics |
| `contextspectre savings` | Show lifetime cleanup savings |

### Maintenance

| Command | Description |
|---------|-------------|
| `contextspectre doctor` | Check tool health and environment |
| `contextspectre gc` | Identify and clean up stale sessions |
| `contextspectre relocate --scan` | Find orphaned sessions after project moves |
| `contextspectre relocate --from <old> --to <new>` | Migrate sessions to new project path |
| `contextspectre version` | Print version, commit, and build date |

### Configuration keys

| Key | Default | Description |
|-----|---------|-------------|
| `cost-alert` | — | Session cost alert threshold (dollars) |
| `weekly-budget` | — | Weekly spending budget |
| `weekly-limit` | — | Weekly usage limit |
| `billing-week-start` | — | Day the billing week starts |
| `expert-mode` | `false` | Enable auto-clean of safe tiers (1-3) on context pressure |
| `health-context-warn` | `75` | Context % threshold for vector gauge warning |
| `health-cpd-warn` | `15` | CPD threshold for warning |
| `health-ttc-warn` | `90` | TTC threshold for warning |
| `health-cdr-warn` | `0.35` | CDR threshold for warning |

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
| 3 | Remove stale duplicate file reads | `--dedup-reads` | Yes (with --live) |
| 4 | Replace base64 images with text placeholders | `--images` | Aggressive |
| 4 | Strip decorative separator lines | `--separators` | Aggressive |
| 5 | Truncate large Bash outputs (keep first/last N lines) | `--truncate-output` | Aggressive |
| 6 | Remove failed tool retries | `--failed-retries` | No |
| 6 | Remove sidechain entries | `--sidechains` | No |
| 7 | Remove cross-repo tangent sequences | `--tangents` | No |

`--all` runs all 9. `--live` runs Tier 1-3. `--live --aggressive` runs Tier 1-5.

## Expert hygiene mode

When `expert-mode` is enabled (`contextspectre config set expert-mode true`), ContextSpectre automatically runs safe cleanup operations (Tier 1-3) when context pressure is detected. Triggers on user actions only (stats, watch poll, status-line), never in the background. Everything Tier 4+ remains manual.

## Session targeting

Most commands accept a session ID, full path, or partial match. Additionally:

- `--cwd` — auto-detect the most recent session for the current working directory
- `--auto` — find and use the most recent session across all projects
- `--project <name>` — filter by project name, path substring, or alias
