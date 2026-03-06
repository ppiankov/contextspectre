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
contextspectre | Opus 4.6 | ctx:73% | sig:A clean:3K ips:77 | $86.66
```

When a session has a broken parent chain, a red `⚠` appears at the end of the status line. This indicates the session's JSONL file has structural corruption that will prevent resume — run `contextspectre fix <session-id> --apply` to repair it. See [chain integrity](#chain-integrity) below.

The status line shows repo, model, context fill, signal grade, cleanable tokens, input purity score, and session cost — all at a glance while you work. Labels stay neutral; only values are color-coded so the numbers pop when they need attention.

**Setup.** Create a status line hook in your settings. Claude Code calls this script on every turn, passing session metadata as JSON on stdin.

1. Create the hook script (e.g., `~/.claude/statusline.sh`):

```bash
#!/bin/bash
# Status line: model, context %, signal grade, cleanable tokens, IPS, cost
# contextspectre data via background cache (never blocks)

input=$(cat)

root=$(git rev-parse --show-toplevel 2>/dev/null || echo "$PWD")
repo=$(basename "$root")
model=$(echo "$input" | jq -r '.model.display_name // "?"')
ctx_pct=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
cost=$(printf '%.2f' "$(echo "$input" | jq -r '.cost.total_cost_usd // 0')")

# contextspectre: read cached data (signal, cleanable, IPS)
# Background refresh keeps data fresh without blocking
cache="/tmp/contextspectre-status-$PPID.json"
signal=""
cleanable=""

if [ -f "$cache" ]; then
  cache_age=$(( $(date +%s) - $(stat -f %m "$cache" 2>/dev/null || echo 0) ))
  if [ "$cache_age" -lt 60 ]; then
    signal=$(jq -r '.signal_grade // empty' "$cache" 2>/dev/null)
    ips_raw=$(jq -r '.input_purity // empty' "$cache" 2>/dev/null)
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
if [ "$ctx_pct" -ge 80 ]; then ctx_color="\033[31m"      # red
elif [ "$ctx_pct" -ge 60 ]; then ctx_color="\033[33m"     # yellow
else ctx_color="\033[32m"; fi                              # green
reset="\033[0m"

# Color cleanable tokens by severity
clean_seg=""
if [ -n "$cleanable" ]; then
  if [ "$cleanable_raw" -ge 500000 ] 2>/dev/null; then
    clean_color="\033[31m"    # red: >500K
  elif [ "$cleanable_raw" -ge 100000 ] 2>/dev/null; then
    clean_color="\033[33m"    # yellow: >100K
  else
    clean_color="\033[32m"    # green: <100K
  fi
  clean_seg=" clean:${clean_color}${cleanable}${reset}"
fi

# Color signal grade by health
sig_seg=""
if [ -n "$signal" ]; then
  case "$signal" in
    A|B) sig_color="\033[32m" ;;  # green
    C|D) sig_color="\033[33m" ;;  # yellow
    *)   sig_color="\033[31m" ;;  # red
  esac
  sig_seg=" | sig:${sig_color}${signal}${reset}"
fi

# Color input purity score
ips_seg=""
if [ -n "$ips_raw" ] && [ "$ips_raw" != "0" ]; then
  ips_int=$(printf '%.0f' "$ips_raw")
  if [ "$ips_int" -ge 80 ] 2>/dev/null; then
    ips_color="\033[32m"      # green: well-purified
  elif [ "$ips_int" -ge 50 ] 2>/dev/null; then
    ips_color="\033[33m"      # yellow: room to improve
  else
    ips_color="\033[31m"      # red: mostly raw input
  fi
  ips_seg=" ips:${ips_color}${ips_int}${reset}"
fi

# Chain integrity — red ⚠ if broken
chain_seg=""
chain_raw=$(jq -r '.chain_healthy // true' "$cache" 2>/dev/null)
if [ "$chain_raw" = "false" ]; then
  chain_seg=" \033[31m⚠\033[0m"
fi

# Assemble and print
cs_seg="${sig_seg}${clean_seg}${ips_seg}${chain_seg}"
printf '%b' "${repo} | ${model} | ctx:${ctx_color}${ctx_pct}%${reset}${cs_seg} | \$${cost}"
```

2. Make it executable:

```bash
chmod +x ~/.claude/statusline.sh
```

3. Register the hook in `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "~/.claude/statusline.sh",
    "padding": 2
  }
}
```

The hook runs `contextspectre summary --cwd --format json` in the background every 60 seconds, caching the result. The status line reads from cache so it never blocks — even on large sessions.

**Color-coded indicators.** Labels stay neutral (white). Only values are colored — the number pops when it needs attention.

`ctx:` — context fill:
- 🟢 Green (< 60%) — healthy headroom
- 🟡 Yellow (60-79%) — monitor, approaching compaction
- 🔴 Red (80%+) — compaction imminent, consolidate or clean

`sig:` — signal grade (signal-to-noise ratio):
- 🟢 Green (A/B) — healthy signal, minimal noise
- 🟡 Yellow (C/D) — degrading, noise accumulating
- 🔴 Red (F) — noise-dominated, clean immediately

`clean:` — cleanable tokens (watch mode handles tiers 1-3 only):
- 🟢 Green (< 100K) — healthy, watch mode handling it
- 🟡 Yellow (100K-500K) — consider running `clean --all` manually
- 🔴 Red (> 500K) — manual intervention needed, tangents accumulating

`ips:` — input purity score (how much tool output is pre-compressed):
- 🟢 Green (≥ 80) — well-purified, input compression working
- 🟡 Yellow (50-79) — room to improve, some raw output entering context
- 🔴 Red (< 50) — mostly raw input, consider adding input purification (e.g., [RTK](https://github.com/rtk-ai/rtk))

`⚠` — chain integrity (appears only when broken):
- 🔴 Red — session has a broken parent chain, resume will fail. Run `contextspectre fix <id> --apply`

**When `clean:` is red**, watch mode is running but the bulk of the waste is tangents (tier 7) and retries (tier 5) that watch mode intentionally skips on active sessions. Run `contextspectre clean <session> --all` to clear them.

**Reading the status line at a glance:** All green = healthy session. Any yellow = monitor. Any red = act now. Red `⚠` = act immediately (session is unresumable). The five indicators cover the full lifecycle: how full (ctx), how clean (sig), how much to clean (clean), how pure the input (ips), and whether the session file is structurally sound (⚠).

## Chain integrity

Claude Code stores conversations as JSONL files with entries linked by `parentUuid`. When multiple subagents write to the same file concurrently, the writer can drop an assistant message — leaving an orphaned `tool_result` whose parent doesn't exist. If this orphan lands in the active parent chain, the session becomes permanently unresumable: the API rejects every request, and `claude --resume` hangs on large files.

This is a known Claude Code bug ([anthropics/claude-code#31328](https://github.com/anthropics/claude-code/issues/31328)). ContextSpectre detects it early so you can repair before the session is lost.

**Detection.** The `stats`, `doctor`, and `watch` commands all check chain integrity. The status line shows a red `⚠` when a session has a broken chain. Detection is cheap — it walks the active parent chain (typically 20-50 entries after compaction), not the entire file.

**Repair.** Run `contextspectre fix <session-id> --apply` to amputate the broken entry and re-parent the chain. The `doctor` command also reports integrity issues across all sessions.

**When it happens.** The corruption pattern is specific: 3+ concurrent subagents writing to the same JSONL file. The risk increases with heavy parallel tool use (e.g., multiple file reads or shell commands running simultaneously). Single-agent sessions are not affected.

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

## Reasoning effort and token budget

Claude Code's reasoning effort setting controls how many thinking tokens the model uses per turn. Higher effort means deeper reasoning but also faster context fill, more frequent compactions, and higher cost per turn.

**Match effort to the task.** Most coding work — flag additions, string changes, bug fixes, wiring a field through a few files — is mechanical. Medium effort handles it fine. High effort (ultrathink) is for architecture decisions, subtle multi-file refactors, or debugging where the model needs to hold many constraints in mind simultaneously.

Running high effort on routine work is token bleed: you burn 3-5x more thinking tokens per turn with no quality improvement, hit compaction sooner, and lose reasoning context faster. ContextSpectre makes this visible — watch the `ctx:` indicator climb faster on high-effort sessions doing simple work.

**Rule of thumb:** If you can describe the change in one sentence, medium effort is enough. Save high effort for turns where you'd want a human engineer to stop and think carefully before writing code.

**Reading the signals.** The status line already tells you whether high effort is affordable. Use `ctx:` as the effort gauge:

| ctx:     | Suggested max effort | Why                                              |
|----------|---------------------|--------------------------------------------------|
| < 40%    | High (ultrathink)   | Plenty of runway, deep reasoning affordable      |
| 40-74%   | Medium              | Conserve tokens, most tasks don't need deep think |
| 75%+     | Low or medium       | Every token counts, compaction imminent           |

This is guidance, not a rule. A subtle architecture bug at 80% context may genuinely need ultrathink — but you're making a tradeoff: one deep turn might trigger compaction and lose earlier reasoning. The table helps you make that choice consciously instead of leaving effort on max by default.

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
