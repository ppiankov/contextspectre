# Concepts & Glossary

## Glossary

| Term | Definition |
|------|------------|
| **Compaction** | Claude Code's automatic context compression. Triggers at ~165K tokens, summarizes the conversation, resets to ~40-50K. You lose specificity. |
| **Compaction epoch** | The period between two compactions. The fundamental unit of reasoning history — each epoch has its own topic, cost, and drift profile. |
| **Compaction archaeology** | Forensic reconstruction of what a compaction preserved and discarded: files touched, tools used, decisions made, compression ratio. |
| **Context deadlock** | Terminal state where a session is too large to continue (API rejects new messages) and too large to compact (compaction is itself an API call). See [Context Deadlock](deadlock.md). |
| **Signal / Noise** | Signal = content that contributes to productive reasoning. Noise = waste that fills the session file without value: progress metadata, duplicate file reads, failed retries, decorative separators, cross-repo tangents. Some noise is never sent to the API (progress, snapshots). Some is sent but adds nothing (stale reads, tangents). |
| **Vector health score** | A-F grade measuring signal-to-noise ratio. A = >95% signal. F = <20% signal. |
| **Reasoning contamination** | Old exploratory scaffolding persisting in context and biasing future responses off-vector. Not token waste — reasoning drift. |
| **Reasoning phases** | Three lifecycle stages: **exploratory** (searching, reading, brainstorming — temporary), **decision** (commit point — permanent), **operational** (execution after a decision — forward-only). |
| **Scope drift** | When tool calls leave the session's project directory. Detected structurally by comparing file paths against the session's root. |
| **Tangent** | A sequence of entries operating in a different repository. A specific type of scope drift. |
| **Sidechain** | An orphaned conversation branch — tool results referencing tool uses that no longer exist. |
| **Re-read tax** | The cost of cache reads. Every turn, the full context is re-processed by the API. Noise tokens are re-read alongside signal — you pay for the bloat on every message. |
| **Compaction tax** | The quality cost of lossy compression. When compaction triggers, reasoning is summarized — decisions lose specificity, nuance disappears, and Claude works from a shadow of what existed. |
| **Re-explanation tax** | The cumulative cost of re-stating architecture, constraints, and decisions in every new session because prior sessions are inaccessible. |
| **Keep marker** | Human tag protecting an entry from cleanup. Survives all automated operations. |
| **Commit point** | A decision boundary. Exploration above it can be collapsed — the scaffolding served its purpose. |
| **Amputation** | Surgically removing entries from the end of a session to drop token count below the compaction threshold. Recovery operation for context deadlock. |
| **Split surgery** | Extracting a range of entries to portable markdown. Non-destructive by default; optionally prunes from the source. |
| **Separation surgery** | Marking conversation branches worth continuing, exporting them, starting fresh. Planned (Phase 4). |
| **Unite** | Merging multiple branch exports into a single context file with deduplication and token budgeting. |
| **Context distillation** | Increasing the signal-to-noise ratio — not making sessions smaller, but making what remains more useful. |
| **Vector snapshot** | A decisions-only extract of a project's canonical constraints and architecture. A north star document. |
| **Ghost context** | Stale compaction summaries that describe code or decisions that no longer exist. Detected by comparing files referenced in compaction summaries against current state. |
| **Live cleanup** | Cleaning an active session between Claude's turns. Uses mtime-based race detection to avoid corrupting a session Claude is writing to. |
| **Tier (1-7)** | Safety classification for cleanup operations. Tier 1 (progress removal) is always safe. Tier 7 (tangent removal) requires the session to be inactive. See [Commands](commands.md#cleanup-operations). |
| **Conversation branch** | A segment of a session between compaction boundaries or significant time gaps. The navigable unit within a long session. |
| **Namespace fragmentation** | Same conceptual project, multiple session namespaces, no unifying abstraction. Caused by launching Claude Code from different directories in the same repo. |
| **Context partitioning drift** | When operational partitioning (filesystem paths) diverges from conceptual partitioning (project identity). The root cause of split session contexts. |
| **Federated project identity** | Multiple physical session roots mapped to one logical project. Decouples identity from storage location. |
| **Logical project overlay** | A view abstraction over session roots — like a materialized view in database terms. Sessions stay in their original directories; the overlay groups them for commands. |
| **Sidechain** | An orphaned conversation branch — tool results referencing tool uses that no longer exist. Created by cleanup or amputation that removes entries mid-chain. |
| **Vector sharpening** | Proactive cleanup that keeps reasoning on the development vector for longer. Not reactive housekeeping — deliberate noise removal at decision boundaries to extend session runway and delay compaction. The opposite of letting context decay until compaction forces lossy compression. |
| **Savings attribution** | Quantifying the downstream economic value of cleanup. Formula: `tokens_removed × remaining_turns × cache_read_price`. A cleanup that removes 7K tokens with 18 turns remaining saves ~126K cache-read tokens. Planned (Phase 4). |
| **Cleanup cadence** | The rhythm of proactive cleanup during a session. Optimal cadence is noise-ratio-driven (clean when noise > 15%), not event-driven (clean when context overflows). Planned (Phase 4). |
| **Cadence score** | A 0-100 composite metric measuring cleanup urgency. Weighted from noise ratio (40%), compaction proximity (30%), token growth rate (20%), and time since last cleanup (10%). Score > 70 = clean now. 40-70 = due. < 40 = healthy. Planned (Phase 4). |
| **Continuity index** | A 0-100 score measuring cross-session efficiency for a project. 100 = no redundant reads or re-explanation. 0 = every session starts from scratch. Based on unique vs total file reads and text block deduplication. Planned (Phase 4). |
| **Vector Control** | TUI instrument panel for reasoning lifecycle management. Three panels: Now (current state), What-if (projected cost without cleanup), If clean now (projected gains). One-key actions: C (clean), S (split tangent), E (export). The reasoning flight instrument panel. Planned (Phase 4). |
| **Expert hygiene mode** | Opt-in auto-clean for safe tiers (1-3: progress, snapshots, stale reads). Triggers on user actions only (stats, TUI refresh, watch poll), never in background. Everything tier 4+ remains manual. Planned (Phase 4). |
| **Budget protection** | Combined risk assessment from compaction proximity, noise ratio, and weekly budget remaining. Produces ranked action recommendations with cost-efficiency estimates. Planned (Phase 4). |
| **Cooldown** | Claude Code's weekly usage limit enforcement. When the limit is reached, a cooldown period prevents further usage until the billing week resets. Invisible to users until they hit the wall. |
| **Burn rate** | Dollars per hour or per turn for the current session or billing week. Used to project time-to-limit and cost-to-compaction. |
| **Status line telemetry** | Fast-path contextspectre output designed for Claude Code's status line hook. Sub-200ms via mtime-based caching. Injects noise/signal/cadence/savings into the live status bar. Planned (Phase 4). |
| **Session timeline** | Chronological reasoning map combining compaction epochs, marks, scope drift events, and costs. The "git log for reasoning." Planned (Phase 4). |
| **Reasoning entropy** | Composite 0-100 score combining all three axes of context decay: noise ratio (reasoning), compaction pressure (economic), scope drift and sidechains (structural). LOW (0-20), MEDIUM (20-50), HIGH (50-75), CRITICAL (75-100). Planned (Phase 4). |
| **Bookmark** | A navigational anchor in a session — checkpoint ("I was here"), milestone ("something completed"), keep marker ("protect from cleanup"), commit point ("decision made here"). |
| **Reasoning graph** | Structural graph of session relationships. Nodes are sessions. Edges are explicit structural evidence: shared files, project aliases, decision references, continuity links. No semantic inference. Planned (Phase 5). |
| **Decision lineage** | Tracing a decision back through sessions to its origin. `git blame` for reasoning — which session decided this, what epoch, what cost. Planned (Phase 5). |
| **Decision conflict** | A later session contradicting an earlier decision. Detected structurally: scope violations (modifying constrained files), reversal patterns (deleting decision artifacts), constraint drift. Detected, not resolved. Planned (Phase 5). |
| **Project memory** | A compiled artifact synthesized from all sessions in a project: decisions, constraints, key files, recent work. Deterministic — same sessions = same output. Not AI-generated, not a second brain — a compiler output. Planned (Phase 5). |

## The three axes of context decay

Unmanaged context decays along three axes simultaneously:

| Axis | What decays | Symptoms | ContextSpectre instruments |
|------|-------------|----------|---------------------------|
| **Economic** | Money | Re-read tax (cache reads re-process noise every turn), re-explanation tax (re-stating context across sessions), token bleed (gradual waste accumulation) | Cost attribution, predictive cleanup, turn-gain estimates, savings attribution (planned), weekly telemetry (planned), budget protection (planned), continuity cost (planned) |
| **Reasoning** | Quality | Reasoning contamination (stale scaffolding biasing responses), context spoil (summaries of summaries losing specificity), compaction loss (lossy compression erasing nuance), ghost context (compaction summaries referencing deleted code) | Vector health score, reasoning phase markers, commit points, ghost detection, reasoning entropy (planned), session timeline (planned), decision lineage (planned) |
| **Structural** | Organization | Namespace fragmentation (same project, split sessions), context partitioning drift (paths diverge from projects), scope drift (tool calls leaving project directory), sidechains (orphaned branches) | Scope drift detection, sidechain repair (planned), federated project identity (planned), reasoning graph (planned), conflict detection (planned) |

The informal terms — **token bleed** and **context spoil** — describe the same decay in visceral shorthand. Token bleed is the economic axis felt as waste. Context spoil is the reasoning axis felt as drift. Both are continuous, invisible, and compound over time.

## Reasoning phases

LLM sessions move through three phases. Claude Code treats them identically — all persist in context forever. ContextSpectre lets you distinguish them and act accordingly.

**Exploratory.** Searching, reading files, brainstorming approaches. High volume, low permanence. Most of this becomes noise after a decision is made. Keeping it in context pulls future reasoning toward abandoned alternatives.

**Decision.** The commit point where a choice is made. These are the entries worth preserving — they define what the project is and why. Decisions should survive compaction and carry forward to new sessions.

**Operational.** Execution after a decision. Writing code, running tests, fixing errors. Forward-only — the decision is made, the work is being done. Operational context is valuable while active but ages quickly.

The transition from exploratory to decision is the key moment. ContextSpectre's commit points mark this boundary. Everything above a commit point can be collapsed — the scaffolding served its purpose.
