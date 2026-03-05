# Workflow Patterns

## The explore-execute-collapse cycle

For large projects, separating exploratory reasoning from structured execution reduces context drift. One effective pattern:

- **Explore and design** in a conversational interface (Claude for Mac, or a fresh CLI session).
- **Execute structured work** in Claude Code CLI with focused, scoped prompts.
- **Use ContextSpectre at decision boundaries** — collapse exploration into decisions before continuing. The scaffolding that got you to the decision becomes noise once you've committed.

ContextSpectre does not require this workflow — it works with any Claude Code session. But it complements structured development particularly well.

**When to clean: the plan-to-code boundary.** The highest-value cleanup moment is after plan approval, before implementation starts. Planning generates exploratory reasoning — file reads, architecture comparisons, rejected approaches, design iterations. Once the plan is committed, that scaffolding becomes noise that fills context during the execution phase where you need room for code, tests, and tool output. A `clean --all` at this boundary recovers the exploration tokens and gives implementation maximum runway.

## Philosophy in practice

**Context distillation over context deletion.** The goal is not to make sessions smaller. It's to increase the signal-to-noise ratio of what Claude sees. Progress messages, stale file reads, failed retries, and decorative separators are pure noise. Decisions, constraints, and working code are pure signal.

**Expose the hidden economics of reasoning.** Tokens are abstract. Percentages are abstract. Dollars are visceral. "$32 for that debugging detour that got compacted away" changes behavior faster than "82% context usage" ever will.

**The historian, not the operator.** ContextSpectre does not run your sessions or tell you what to do next. It records what happened, shows what it cost, and lets you decide what to carry forward. The operator explores and decides. The historian preserves the decisions and discards the scaffolding.

## CLI status line integration

Claude Code CLI supports a custom status line hook. ContextSpectre's `status-line` command is designed for it — sub-2ms on repeat calls via mtime-based caching:

```
contextspectre | Opus 4.6 | ctx:65% [#############-------] | sig:F clean:149K | $160.81
```

The status line shows model, context fill, signal grade, cleanable tokens, and session cost — all at a glance while you work. When the signal grade drops or cleanable tokens grow, you know it's time to act.

**Setup.** Create a status line hook in your settings. Claude Code calls this script on every turn, passing session metadata as JSON on stdin.

1. Create the hook script (e.g., `~/.claude/hooks/statusline.sh`):

```bash
#!/bin/bash
# Status line hook: model, context %, signal grade, cleanable tokens, cost
# contextspectre data via background cache (never blocks the UI)

input=$(cat)

repo=$(basename "$(git rev-parse --show-toplevel 2>/dev/null || echo "$PWD")")
model=$(echo "$input" | jq -r '.model.display_name // "?"')
ctx_pct=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
cost=$(printf '%.2f' "$(echo "$input" | jq -r '.cost.total_cost_usd // 0')")
added=$(echo "$input" | jq -r '.cost.total_lines_added // 0')
removed=$(echo "$input" | jq -r '.cost.total_lines_removed // 0')

# contextspectre: read cached signal grade and cleanable tokens
# Background refresh keeps data fresh without blocking
cache="/tmp/contextspectre-status-$PPID.json"
signal=""
cleanable=""

if [ -f "$cache" ]; then
  cache_age=$(( $(date +%s) - $(stat -f %m "$cache" 2>/dev/null || echo 0) ))
  if [ "$cache_age" -lt 60 ]; then
    signal=$(jq -r '.signal_grade // empty' "$cache" 2>/dev/null)
    cleanable_raw=$(jq -r '.cleanable_tokens // 0' "$cache" 2>/dev/null)
    if [ "$cleanable_raw" -gt 1000 ] 2>/dev/null; then
      cleanable="$(( cleanable_raw / 1000 ))K"
    fi
  fi
fi

# Fork background refresh (non-blocking, never stalls the prompt)
if [ ! -f "$cache" ] || [ "${cache_age:-999}" -ge 60 ]; then
  (contextspectre summary --cwd --format json > "$cache" 2>/dev/null &)
fi

# Color context by usage level
if [ "$ctx_pct" -ge 80 ]; then ctx_color="\033[31m"    # red
elif [ "$ctx_pct" -ge 60 ]; then ctx_color="\033[33m"   # yellow
else ctx_color="\033[32m"; fi                            # green
reset="\033[0m"

# Build 20-char progress bar
filled=$((ctx_pct / 5)); empty=$((20 - filled))
bar=""; i=0; while [ $i -lt $filled ]; do bar="${bar}#"; i=$((i+1)); done
i=0; while [ $i -lt $empty ]; do bar="${bar}-"; i=$((i+1)); done

# Assemble contextspectre segment
cs_seg=""
[ -n "$signal" ] && cs_seg=" | sig:${signal}"
[ -n "$cleanable" ] && cs_seg="${cs_seg} clean:${cleanable}"

printf '%b' "${repo} | ${model} | ${ctx_color}ctx:${ctx_pct}%${reset} [${bar}]${cs_seg} | \$${cost} | +${added}/-${removed}"
```

2. Make it executable:

```bash
chmod +x ~/.claude/hooks/statusline.sh
```

3. Register the hook in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "StatusLine": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "bash ~/.claude/hooks/statusline.sh"
          }
        ]
      }
    ]
  }
}
```

The hook runs `contextspectre summary --cwd --format json` in the background every 60 seconds, caching the result. The status line reads from cache so it never blocks — even on large sessions.

**What to watch for:**
- Signal grade dropping from A/B to D/F — noise is accumulating
- Cleanable tokens growing — run `quick-clean` or `clean --all`
- Context above 75% — approaching compaction territory
- Cost velocity spikes — session may be drifting

This gives you live awareness while working. The status line tells you *how full* and *how clean* your context is. `contextspectre status` or the TUI tells you the details and lets you act.

## Continuous cleanup in a side terminal

The highest-leverage workflow: run continuous cleanup in a terminal next to Claude Code.

```bash
contextspectre clean --active --all --watch
```

This watches all active sessions and cleans them between Claude's turns — progress messages, stale reads, snapshots, and other noise are removed automatically as the session grows. You work in one terminal, contextspectre keeps context clean in the other.

**What it does:**
- Polls active sessions using mtime-based detection (5s check, 30s cooldown)
- Runs all 9 cleanup tiers when a session is idle (no writes in the last few seconds)
- Skips sessions that Claude is actively writing to (mtime race detection)
- Prints a running summary: tokens recovered, cost saved, sessions cleaned
- Ctrl+C exits cleanly with a final summary; double Ctrl+C force-quits

**Why it matters:** In a long session, noise accumulates continuously — every file read, every progress message, every failed retry. Without cleanup, noise compounds until compaction triggers and reasoning quality drops. Continuous watch keeps the session clean so compaction happens later (or never), and when it does happen, there's less noise to compress into the summary.

**Variants:**

```bash
# Fixed 30-second interval instead of smart mtime polling
contextspectre clean --active --all --watch --interval 30

# Watch only sessions for the current project
contextspectre clean --active --all --watch --project myproject
```

## Working during cooldowns

Long AI sessions can hit provider limits or cooldown periods. When this happens, the most effective workflow is to shift mechanical work away from the primary reasoning session.

Tasks that can be offloaded:
- Running tests
- Security scanning
- Static analysis
- Formatting and refactoring
- CI validation
- Dependency updates

These tasks do not require large reasoning context and can be executed by cheaper agents, local tools, or CI systems.

While those run, your primary session remains focused on architectural decisions and high-signal reasoning. ContextSpectre helps keep the reasoning session clean so it remains useful when you return — clean noise before the session resumes, not after it overflows.
