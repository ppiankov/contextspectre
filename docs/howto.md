# How-To Guide

Practical recipes for common contextspectre tasks.

## Rename a project folder without losing Claude session history

Claude Code stores session history in `~/.claude/projects/` using path-encoded directory names. If you rename your project folder, Claude won't find old sessions because the encoded path no longer matches.

**Steps:**

```bash
# 1. Rename the project folder
mv ~/dev/old-name ~/dev/new-name

# 2. Rename the matching Claude projects directory
mv ~/.claude/projects/-Users-you-dev-old-name \
   ~/.claude/projects/-Users-you-dev-new-name
```

**How it works:** Claude Code encodes your project's absolute path by replacing `/` with `-`. The directory `~/dev/myproject` becomes `-Users-you-dev-myproject` inside `~/.claude/projects/`. Renaming both keeps all session history, memory files, markers, and analytics intact.

**Finding the encoded name:** If you're unsure of the exact encoded name:

```bash
ls ~/.claude/projects/ | grep old-name
```

**What's preserved:** Session JSONL files, `.markers.json` sidecars, `.bak` backups, and the `memory/` directory (agent memory). Everything stays exactly as it was — only the parent directory name changes.

## Auto-checkpoint before compaction

ContextSpectre can automatically save a resume brief when your context window reaches 70%, before Claude's compaction destroys specificity.

**Setup:** If you use the status-line hook, auto-checkpoint is built in. When context pressure crosses 70%, it writes a structured brief to `docs/context.txt` in your project directory. It fires once per epoch — no repeated writes.

The brief contains decisions, findings, user requests, and files touched in the current epoch. Reference it from your CLAUDE.md so the next session picks up where you left off.

## Preserve decisions before cleaning

By default, `clean --all` removes noise entries. Some of those entries may contain useful decisions or findings buried in tool output.

```bash
# Extract decisions/findings before cleaning
contextspectre clean <session> --all --preserve

# The sidecar file accumulates across multiple cleans
cat ~/.claude/projects/.../session-id.preserved.md
```

The `--preserve` flag scans entries about to be deleted for decision keywords ("decided", "chose", "because") and finding keywords ("found that", "root cause", "confirmed"). Results are appended to a `.preserved.md` sidecar file next to the session.

## Fix Mac session API errors

Claude for Mac splits multi-tool calls into separate JSONL entries, causing API 400 errors ("unexpected tool_use_id in tool_result blocks"). Coalesce merges them back:

```bash
# Dry run — see what would be merged
contextspectre coalesce <session>

# Apply the fix
contextspectre coalesce <session> --apply

# Or fix everything at once (coalesce runs automatically)
contextspectre clean <session> --all
contextspectre fix <session> --apply
```

Mac sessions regrow these errors as new entries are written. Run coalesce periodically or use `clean --all` which includes it.

## Generate a session resume brief

When switching between sessions or before closing a long session, generate a checkpoint:

```bash
# Print to stdout
contextspectre checkpoint --cwd

# Write to file for CLAUDE.md reference
contextspectre checkpoint --cwd --output docs/context.txt

# JSON for scripting
contextspectre checkpoint --cwd --format json
```

The checkpoint extracts from the active epoch: decisions made, findings discovered, user requests, files touched, and any commit points you've marked.

## Recover from a killed session (false positive, model switch, crash)

Claude Code's safety classifier can false-positive on benign technical terminology (e.g., robotics, physics, security research), killing the session with a "Usage Policy" error. Once triggered, it cascades — every subsequent message in the same session hits the same filter. See [anthropics/claude-code#34977](https://github.com/anthropics/claude-code/issues/34977).

**What makes it worse:** Switching models with `/model` wipes the context window entirely. The "fix" destroys the session too.

**How to recover using contextspectre:**

```bash
# 1. Open a new terminal / new claude session

# 2. Find the dead session (use project name or list all)
contextspectre sessions --project myproject

# 3. Extract a resume brief
contextspectre checkpoint <session-id>

# 4. Or export structured data
contextspectre export decisions <session-id>
contextspectre export tasks <session-id>

# 5. Paste the checkpoint into your new session as context
```

**Prevention tips:**
- Run `contextspectre checkpoint --cwd --output docs/context.txt` periodically during long sessions
- Don't switch models (`/model`) when you hit a false positive — it wipes context. Start a fresh session instead
- If a question uses dual-use terminology (targeting, dropping, accuracy, injection), rephrase before sending
- Save external API output to files instead of pasting raw content into the session
