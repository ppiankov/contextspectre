# Claude CLI Compatibility

contextspectre reads Claude Code session files (JSONL) directly. As Claude CLI evolves, new entry types and fields appear. This matrix tracks which contextspectre versions support which Claude CLI features.

## JSONL Entry Types

| Entry type | Claude CLI version | contextspectre version | Notes |
|---|---|---|---|
| `user` | all | v0.1.0+ | User messages |
| `assistant` | all | v0.1.0+ | Assistant responses with usage stats |
| `progress` | all | v0.1.0+ | Streaming progress (cleanable noise) |
| `file-history-snapshot` | all | v0.1.0+ | File state snapshots (cleanable noise) |
| `system` | all | v0.1.0+ | System messages |
| `queue-operation` | ~v1.0+ | v0.30.0+ | Desktop/Mac session indicator |
| `custom-title` | v2.1+ | **v0.43.0+** | User-set session name (`--name` flag) |
| `agent-name` | v2.1+ | **v0.43.0+** | Agent name echo |

## Session Identity Fields

| Field | Claude CLI version | contextspectre version | Notes |
|---|---|---|---|
| `sessionId` | all | v0.1.0+ | UUID, always present |
| `slug` | ~v1.0+ | v0.20.0+ | Auto-generated 3-word name (e.g. `glimmering-wiggling-thunder`) |
| `customTitle` | v2.1+ | **v0.43.0+** | User-set name via `claude --name "my-session"` |

## Resume Behavior

| Resume method | Claude CLI version | contextspectre support |
|---|---|---|
| `claude --resume <uuid>` | all | `find <uuid>` — all versions |
| `claude --resume <slug>` | v2.1+ (interactive only) | `find <slug>` — v0.43.0+ |
| `claude --resume <custom-title>` | v2.1+ (interactive only) | `find <name>` — v0.43.0+ |
| `claude --resume` (interactive picker) | v2.1+ | N/A (Claude CLI feature) |
| `claude -p --resume <uuid>` | all | N/A (print mode requires UUID) |

**Note:** In Claude CLI v2.1+, `--resume` without a value opens an interactive picker that searches by name/slug. In `--print` (`-p`) mode, `--resume` still requires a full UUID.

## Display Priority

When a session has both a `slug` and a `customTitle`, contextspectre displays the custom title. The priority is:

1. `customTitle` (user-set via `--name`)
2. `slug` (auto-generated)
3. Short ID (first 8 chars of UUID)

## Version Mapping

| contextspectre | Minimum Claude CLI | Key features |
|---|---|---|
| v0.43.0 | v2.1+ recommended | Custom title parsing, name-based find |
| v0.40.0–v0.42.x | v1.0+ | Full feature set minus custom titles |
| v0.30.0–v0.39.x | v1.0+ | Queue-operation detection, slug display |
| v0.1.0–v0.29.x | any | Basic session analysis |

All versions are backward-compatible — newer contextspectre works with older Claude CLI sessions. Sessions without newer fields simply show auto-slugs or short IDs.
