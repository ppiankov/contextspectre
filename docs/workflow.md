# Workflow Patterns

## Core workflow

1. **Explore** in a fresh session — read files, compare approaches, design
2. **Commit** the plan — mark the decision boundary
3. **Collapse** exploration into decisions — `clean --all` at the plan-to-code boundary
4. **Execute** in Claude Code CLI with focused, scoped prompts
5. **Monitor** ctx/sig/clean/ips in the status line
6. **Clean continuously** — run `contextspectre watch` in a side terminal
7. **Offload** mechanical work during cooldowns

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
contextspectre | Opus 4.6 15c93cef | ctx:73% | sig:A clean:3K ips:77 | $86.66
```

When a session has a broken parent chain, a red `⚠` appears at the end of the status line. This indicates the session's JSONL file has structural corruption that will prevent resume — run `contextspectre fix <session-id> --apply` to repair it. See [chain integrity](#chain-integrity) below.

The status line shows repo, model, session ID (first 8 chars), context fill, signal grade, cleanable tokens, input purity score, and session cost — all at a glance while you work. The session ID lets you cross-reference with `contextspectre stats`, `fix`, or `doctor` without hunting for the full UUID — essential when debugging corruption or managing multiple sessions. Labels stay neutral; only values are color-coded so the numbers pop when they need attention.

**Setup.** Register a status line hook in `~/.claude/settings.json` and point it at the hook script:

```json
{
  "statusLine": {
    "type": "command",
    "command": "~/.claude/statusline.sh",
    "padding": 2
  }
}
```

The hook reads ContextSpectre data from a background cache that refreshes every 60 seconds — it never blocks the CLI prompt. Full hook script with color logic: [statusline-hook.md](statusline-hook.md).

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

## Recommended: continuous cleanup in a side terminal

The highest-leverage operating pattern for long sessions:

```
Terminal 1: Claude Code          Terminal 2: contextspectre watch
┌─────────────────────┐          ┌─────────────────────────────┐
│ claude               │          │ contextspectre clean         │
│                      │          │   --active --all --watch     │
│ (you work here)      │          │   (runs automatically)       │
└─────────────────────┘          └─────────────────────────────┘
```

Watch mode polls active sessions, detects idle gaps between Claude's turns, and cleans all 9 tiers automatically. Progress messages, stale reads, snapshots, retries, and tangents are removed as they accumulate — not after the session overflows.

Noise compounds with every turn. Without cleanup, compaction triggers sooner and reasoning quality drops. Continuous watch keeps the session clean so compaction happens later (or never), and when it does happen, there's less noise to compress into the summary.

```bash
# Default: smart mtime-based polling (5s check, 30s cooldown)
contextspectre clean --active --all --watch

# Fixed 30-second interval
contextspectre clean --active --all --watch --interval 30

# Watch only sessions for the current project
contextspectre clean --active --all --watch --project myproject
```

## Directive clarity — scope before execution

A vague directive with write access is not a task — it is an incident waiting to happen.

AI agents interpret ambiguity by filling in gaps with reasonable-sounding defaults. When the directive is "clean up the README" and the agent has write access to 18 repos, "reasonable" can mean replacing 500 lines of vivid documentation with a 72-line template — across every repo, in one pass, with no review gate. The content is gone before you notice.

**Vague directive signals:**
- "Clean up" / "align" / "standardize" without specifying what to change and what to preserve
- Cross-repo operations without per-repo review criteria
- "Make it consistent" without a reference example
- Any bulk operation touching documentation, configuration, or public-facing content

**What to do instead:**
1. State what you think the directive means
2. State what would be destroyed or changed
3. Ask: "What specifically should change, and what must be preserved?"
4. Do not proceed until the scope is explicit

The pattern applies to any autonomous agent — not just Claude Code. The more repos, files, or artifacts in scope, the more important it is to define what "done" looks like before the agent starts writing.

**Structural defense.** Directive clarity is a policy — it depends on the agent following instructions. For defense in depth, pair it with structural guards: hooks that block destructive writes (content shrinkage, section removal), required sections that cannot be deleted, and per-repo diff review before cross-repo operations land. Policy catches the intent; structure catches the execution.

See [Ambiguous vector](concepts.md#glossary) in the glossary for the formal definition of this failure mode.

## Reasoning effort and token budget

High effort on routine work is token bleed: 3-5x more thinking tokens per turn with no quality improvement, faster compaction, lost context. Most coding work — flag additions, string changes, bug fixes — is mechanical. Medium effort handles it fine. Save high effort for architecture decisions and subtle multi-file reasoning.

**Rule of thumb:** If you can describe the change in one sentence, medium effort is enough.

Use `ctx:` as the effort gauge:

| ctx:     | Suggested max effort | Why                                              |
|----------|---------------------|--------------------------------------------------|
| < 40%    | High (ultrathink)   | Plenty of runway, deep reasoning affordable      |
| 40-74%   | Medium              | Conserve tokens, most tasks don't need deep think |
| 75%+     | Low or medium       | Every token counts, compaction imminent           |

**The queue trick.** When context is above 75% and you need deep reasoning — don't force it. Switch to small mechanical items (flag wiring, doc fixes, test additions) at low effort. After compaction or a fresh session, tackle the hard problem with ultrathink and a clean context. The hard problem gets better reasoning *and* more runway.

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
