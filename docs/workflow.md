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

**Setup.** Add `contextspectre status-line --stdin` as your Claude Code status line hook. It reads the session path from stdin and outputs telemetry in the configured format (tab, shell, human, or json). The mtime cache ensures near-zero overhead — it only re-parses when the session file actually changes.

**What to watch for:**
- Signal grade dropping from A/B to D/F — noise is accumulating
- Cleanable tokens growing — run `quick-clean` or `clean --all`
- Context above 75% — approaching compaction territory
- Cost velocity spikes — session may be drifting

This gives you live awareness while working. The status line tells you *how full* and *how clean* your context is. `contextspectre status` or the TUI tells you the details and lets you act.

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
