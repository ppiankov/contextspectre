# How-To Guide

Practical recipes for common contextspectre tasks.

## Rename a project folder without losing Claude session history

Claude Code stores session history under `~/.claude/projects/` (all platforms including Windows: `C:\Users\<you>\.claude\projects\`) using path-encoded directory names. If you rename your project folder, Claude won't find old sessions because the encoded path no longer matches.

**Recommended: use `contextspectre relocate`**

```bash
# Dry run — see what would move
contextspectre relocate --from ~/dev/old-name --to ~/dev/new-name

# Apply the move
contextspectre relocate --from ~/dev/old-name --to ~/dev/new-name --apply
```

**Manual alternative:**

```bash
# 1. Rename the project folder
mv ~/dev/old-name ~/dev/new-name

# 2. Rename the matching Claude projects directory
mv ~/.claude/projects/-Users-you-dev-old-name \
   ~/.claude/projects/-Users-you-dev-new-name
```

**How it works:** Claude Code encodes your project's absolute path by replacing `/` with `-` (on Windows, `\` becomes `-` too). The directory `~/dev/myproject` becomes `-Users-you-dev-myproject` inside `~/.claude/projects/`. Renaming both keeps all session history, memory files, markers, and analytics intact.

**What's preserved:** Session JSONL files, `.markers.json` sidecars, `.bak` backups, and the `memory/` directory (agent memory). Everything stays exactly as it was — only the parent directory name changes.

## Fix "No conversation found with session ID" in Claude Code

`claude --resume <session-id>` fails with "No conversation found" when the session was created from a parent directory. Claude Code files sessions under the path you launched from — if you ran `claude` from `~/dev/myproject` but the session was started from `~/dev`, it lives under `~/dev`'s encoded directory, not `~/dev/myproject`.

**Find where the session actually is:**

```bash
# By UUID prefix
contextspectre find 88789f29

# By slug or custom title (claude --name)
contextspectre find async-submission-queue
# Session:     88789f29-eb9d-4b3b-8022-246fc4136f6d
# Name:        async-submission-queue
# Project:     /Users/you/dev
# Resume:   claude --resume "async-submission-queue"
# Print:    claude -p --resume 88789f29-eb9d-4b3b-8022-246fc4136f6d
```

**Move it to the correct project:**

```bash
contextspectre find 88789f29 --move ~/dev/myproject
# Moved session 88789f29-eb9d-4b3b-8022-246fc4136f6d
#   From: /Users/you/dev
#   To:   /Users/you/dev/myproject
# Resume:   claude --resume "async-submission-queue"
```

**Discover misplaced sessions automatically:**

```bash
cd ~/dev/myproject
contextspectre sessions --cwd
# No sessions found.
#
# Sessions may exist under a parent or related project:
#   /Users/you/dev (3 sessions)
#
# To find a specific session by ID:
#   contextspectre find <session-id>
```

**Why this happens:** Claude Code encodes the working directory path into the project folder name. `/Users/you/dev` and `/Users/you/dev/myproject` produce different encoded directories. Sessions don't automatically follow you into subdirectories.

**Bulk fix:** If many sessions are under the wrong project, use `relocate` instead:

```bash
contextspectre relocate --from ~/dev --to ~/dev/myproject --apply
```

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

## Identify CLI vs Mac sessions before resuming

`claude -r` lists all sessions from `~/.claude/projects/` without distinguishing whether they were created by Claude CLI or Claude for Mac. Resuming a Mac session from the CLI (or vice versa) can cause unexpected behavior. This is a [known upstream bug](https://github.com/anthropics/claude-code/issues/37665).

**contextspectre tells you which is which:**

```bash
# TUI shows C (CLI) or M (Mac) next to each session
contextspectre

# id command prints client type
contextspectre id 88275ecc
# Full ID:    88275ecc-6767-...
# Client:     desktop
# Project:    /Users/you/dev/myproject
# Path:       /Users/you/.claude/projects/...

# sessions JSON includes client_type field
contextspectre sessions --cwd --format json | jq '.sessions[] | {id: .short_id, client: .client_type, slug: .slug}'
```

**How detection works:** CLI sessions contain `file-history-snapshot` entries. Mac/desktop sessions start with a `queue-operation` entry. contextspectre uses these markers to classify each session as `cli`, `desktop`, or `unknown`.

**Best practice:** Before running `claude --resume`, check the client type with `contextspectre id <prefix>`. Only resume sessions that match your current client.

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

## Clean and resume in one step

`launch` runs the full pre-flight pipeline — checkpoint, fix, clean — then exec-s into `claude --resume`. The session must be idle (not actively used) for fix to converge.

```bash
# Most recent session in current directory
contextspectre launch --cwd
# Session:  88275ecc (toasty-jumping-hare) [Cli]
# Entries:  4231 | Size: 12.4 MB
#
# Checkpoint... saved → docs/context.txt
# Fix...        2 removed, 1 chains repaired, 1 parents reconnected
# Rewire...     clean
# Clean...      312 entries removed, 2 images replaced, 18.3 KB freed
#
# Launching: claude -r toasty-jumping-hare

# Specific session
contextspectre launch 88275ecc

# Dry run — preview without modifying
contextspectre launch --cwd --dry-run

# Skip fix, just clean and launch
contextspectre launch --cwd --clean-only

# Headless mode (claude -p --resume)
contextspectre launch --cwd --print
```

If the session is still active (modified < 60s), launch refuses. Use `--wait` to poll until idle, or `--force` to skip the check:

```bash
# Poll until session idles, then launch
contextspectre launch --cwd --wait

# Skip the active-session check entirely
contextspectre launch --cwd --force
```

## Fix unrecoverable "API Error 400" sessions

When Claude Code drops a `tool_result`, the session becomes permanently stuck — every message returns "API Error: 400 due to tool use concurrency issues" and all recovery mechanisms (rewind, restore, summarize) also fail. See [anthropics/claude-code#39316](https://github.com/anthropics/claude-code/issues/39316).

`rewire` fixes this by injecting synthetic `tool_result` entries for any orphaned `tool_use` blocks:

```bash
# Dry run — show orphaned tool_use blocks
contextspectre rewire <session-id>

# Fix — inject synthetic tool_results
contextspectre rewire <session-id> --apply

# Then resume
contextspectre launch <session-id>
```

`launch` runs rewire automatically as part of its pre-flight pipeline.

## Move a project and its Claude sessions

Claude Code stores sessions under `~/.claude/projects/` in directories named after the project's filesystem path (with `/` replaced by `-`). When you move a project, the session directory must be renamed to match.

```bash
# 1. Move the project
mv ~/dev/old-location/myproject ~/dev/new-location/myproject

# 2. Rename the Claude session directory
mv ~/.claude/projects/-Users-you-dev-old-location-myproject \
   ~/.claude/projects/-Users-you-dev-new-location-myproject

# 3. Verify sessions resolve
cd ~/dev/new-location/myproject
contextspectre sessions --cwd
```

**Common scenarios:**

- **Nested by mistake** (e.g., `clickpulse/mysqlpulse` → `mysqlpulse`): move the dir, rename the session path, verify
- **Two copies exist**: compare `git log --oneline -1` in each to find which is ahead, push the newer one, delete the older
- **Session in wrong project**: use `contextspectre find <session-id> --move` to relocate a session between project directories

**What breaks if you skip step 2:**
- `contextspectre sessions --cwd` won't find old sessions
- `contextspectre launch --cwd` won't find the session to resume
- Claude Code's `--continue` may still work (it searches by recency, not path) but `--resume` with a slug may not

## Resume a Claude Code session

Claude CLI v2.1+ supports resuming by name (interactive) or UUID (scripted):

```bash
# Interactive resume by slug or custom title
claude --resume "async-submission-queue"

# Interactive picker (fuzzy search)
claude --resume

# Scripted/print mode (requires full UUID)
claude -p --resume $(contextspectre id 79109cdc)

# Resume the most recent session in current directory
claude --resume $(contextspectre sessions --cwd --format json | jq -r '.sessions[0].id')
```

**Note:** `claude --continue` resumes the most recent session without needing an ID. Use `--resume` when you want a specific older session. In `--print` mode, `--resume` requires a full UUID — slugs and names only work in interactive mode.

## Auto-save resume info on session exit

A `SessionEnd` hook can automatically save the resume command (with slug) every time a session ends. This way you always know how to resume your last session without hunting for the ID.

**Setup.** Add a `SessionEnd` hook in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/save-resume.sh"
          }
        ]
      }
    ]
  }
}
```

**Hook script.** Save as `~/.claude/hooks/save-resume.sh` and make executable (`chmod +x`):

```bash
#!/bin/bash
# SessionEnd hook: save resume command to docs/resume.md
# Shows slug (for interactive resume) and UUID (for --print mode).

HOOK_INPUT=$(cat)

SESSION_ID=$(echo "$HOOK_INPUT" | python3 -c \
  "import sys,json; d=json.load(sys.stdin); print(d.get('session_id',''))" 2>/dev/null)

if [ -z "$SESSION_ID" ]; then
  exit 0
fi

# Determine project root
work_dir=$(git rev-parse --show-toplevel 2>/dev/null || echo "$PWD")

# Resolve slug from session JSONL
SLUG=""
project_key=$(echo "$work_dir" | sed 's|/|-|g; s|^-||')
session_file="$HOME/.claude/projects/$project_key/$SESSION_ID.jsonl"
if [ -f "$session_file" ]; then
  SLUG=$(grep -o '"slug":"[^"]*"' "$session_file" | tail -1 | sed 's/"slug":"//; s/"//')
fi

# Build resume command — prefer slug for interactive, show UUID for --print
if [ -n "$SLUG" ]; then
  resume_cmd="claude --resume $SLUG"
else
  resume_cmd="claude --resume $SESSION_ID"
fi

# Write resume file
mkdir -p "$work_dir/docs" 2>/dev/null
cat > "$work_dir/docs/resume.md" << EOF
# Resume

\`\`\`bash
$resume_cmd
\`\`\`

**Project:** $work_dir
**Slug:** ${SLUG:-n/a}
**Session:** $SESSION_ID
**Saved:** $(date +%Y-%m-%d\ %H:%M)
EOF

exit 0
```

**What it does:** On every session exit, writes `docs/resume.md` with the `claude --resume` command using the session's slug (or UUID fallback). The file shows the project, slug, session ID, and timestamp. Gitignore `docs/resume.md` to keep it local.

**Why slug?** Claude CLI v2.1+ supports `--resume <slug>` in interactive mode — shorter and memorable. The full UUID is included for `--print` mode which requires it.

## Recover from a killed session (false positive, model switch, crash)

Claude Code's safety classifier can false-positive on benign technical terminology (e.g., robotics, physics, security research), killing the session with a "Usage Policy" error. Once triggered, it cascades — every subsequent message in the same session hits the same filter. See [anthropics/claude-code#34977](https://github.com/anthropics/claude-code/issues/34977).

**What makes it worse:** Switching models with `/model` wipes the context window entirely. The "fix" destroys the session too.

**How to recover using contextspectre:**

```bash
# 1. Open a new terminal / new claude session

# 2. Find the dead session
contextspectre sessions --cwd

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

## Use contextspectre in WSL2

If you run Claude Code on the Windows side (PowerShell, Windows Terminal), your sessions live at `C:\Users\<you>\.claude\`. Inside WSL2, that path is `/mnt/c/Users/<you>/.claude/`.

**contextspectre auto-detects this.** If `~/.claude/projects/` doesn't exist or is empty inside WSL2, it checks `/mnt/c/Users/<you>/.claude/` automatically. No `--claude-dir` flag needed.

**Install:**

```bash
# Homebrew (works in WSL2)
brew install ppiankov/tap/contextspectre

# Or direct download
curl -L https://github.com/ppiankov/contextspectre/releases/latest/download/contextspectre_$(curl -s https://api.github.com/repos/ppiankov/contextspectre/releases/latest | grep tag_name | cut -d'"' -f4 | tr -d v)_linux_amd64.tar.gz | tar xz
sudo mv contextspectre /usr/local/bin/
```

**Manual override** if auto-detection doesn't find your sessions:

```bash
contextspectre sessions --claude-dir /mnt/c/Users/yourname/.claude
```
