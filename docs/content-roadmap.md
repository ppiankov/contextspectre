# Vector Control Content Roadmap

*Editorial plan for essays, domains, positioning, and HN strategy.*

---

## Domain Architecture

| Domain | Purpose | Content | Status |
|--------|---------|---------|--------|
| `vectorcontrol.app` | Concept landing page | One-page explainer: what vector control is, link to essays, link to tool | Not registered |
| `vectorcontrol.dev` | Essay site / blog | Long-form posts with real data, screenshots, framework explanations | Not registered |
| `github.com/ppiankov/contextspectre` | Reference implementation | Open source tool, README, command reference, glossary | Live |

**Key principle:** Vector Control is the practice. ContextSpectre is one implementation. Like "observability" is the concept and Prometheus is an implementation. Keep them separate — if the concept spreads, it's bigger than one tool.

---

## Essay Pipeline

Four essays, ordered by dependency. Each builds on the vocabulary and evidence of the previous.

| # | Title | Status | Target length | Data needed | HN? |
|---|-------|--------|---------------|-------------|-----|
| 1 | Context Decay in Long AI Sessions | Draft (~3,800 words in essay-draft.md) | ~4,500 words | Session screenshots, Knuth reference | No |
| 2 | The Economics of AI Coding | Raw material (methodology.md, cost.go) | ~3,000 words | Cost breakdowns, epoch-level attribution | Maybe |
| 3 | Vector Control — Keeping AI Reasoning on Track | Raw material (essay-draft.md raw sections) | ~3,500 words | Before/after cleanup data, watch output | No |
| 4 | The $2,400 AI Coding Sessions | Not started | ~4,000 words | Heavy: real sessions, costs, screenshots, before/after | **Yes** |

### Essay 1: Context Decay in Long AI Sessions

*The thesis essay. Introduces the problem, the three-axis model, and the vocabulary.*

**Current state:** Draft complete. Solid structure: Knuth opening → three-axis model → CPD → session lifecycle → observed results → vocabulary → regime transition → operational evidence. Seven "Raw material" appendix sections with seeds for later essays.

**Editorial notes:**
- Knuth opening is strong. Keep it.
- Front-loaded with framework — needs one concrete narrative (real session story) before the model explanation.
- "Raw material" sections (RHL, Cliff→Plateau, Stack alignment, Vector Drift Cascade, RFR, Reasoning Diff, Decision Economics) are seeds for Essays 2-4. Extract to separate outlines; don't publish in Essay 1.
- Add 2-3 screenshots of contextspectre output (stats, watch, TUI) — sanitized.
- Add one before/after session narrative: "here's what a session looks like at 82% with Signal F, here's after cleanup."
- Trim to ~4,500 words. Current draft with raw material is ~3,800; publishable core is ~2,500.

### Essay 2: The Economics of AI Coding

*The money essay. Cache reads as hidden cost driver, re-read tax, cost per decision.*

**Source material:**
- `docs/methodology.md` — formulas for cost attribution, cache read pricing
- `internal/analyzer/cost.go` — ModelPricing table, epoch cost calculation
- `docs/concepts.md` — CPD, TTC, CDR definitions, savings attribution
- `docs/essay-draft.md` §Cost Per Decision, §Raw: Decision Economics

**Key arguments:**
- Cache reads account for ~80% of session cost
- Every noise token is re-processed on every turn (re-read tax)
- CPD ($cost / decisions) is the metric that connects cost to output
- Compaction is an economic reset with quality cost (compaction tax)
- Real cost breakdowns from epoch-level attribution

### Essay 3: Vector Control — Keeping AI Reasoning on Track

*The practice essay. The control loop, when to clean, the discipline.*

**Source material:**
- `docs/essay-draft.md` §Raw: Stack alignment, §Raw: Cliff→Compression Plateau
- `docs/concepts.md` — vector control, vector sharpening, cleanup cadence
- `docs/workflow.md` — operational patterns

**Key arguments:**
- The inspect → clean → verify → continue loop
- Plan-to-code boundary as optimal cleanup moment
- The explore → execute → collapse cycle
- How the practice emerged from operational discovery, not theory
- ContextSpectre as reference implementation (not product pitch)

### Essay 4: The $2,400 AI Coding Sessions (HN candidate)

*The evidence essay. Data-heavy, story-first, framework as conclusion.*

**Not started.** This essay needs accumulated data:
- Multiple session narratives with real costs
- Before/after cleanup comparisons with screenshots
- The monster session story ($373, 8K entries, Signal F→A)
- Total lifetime stats from analytics.jsonl

**Format:** story → data → pattern → framework → tool mention (last).

---

## Disclosure Rules

Three levels. Every piece of content must pass the level check before publishing.

### Level 1 — Public (README, GitHub)

- What the tool does, install instructions, command reference
- Workflow patterns (explore → plan → implement → commit)
- NO orchestration details, NO agent configs, NO workflow mechanics

### Level 2 — Essays (vectorcontrol.dev)

Allowed:
- The three-axis framework (economic, reasoning, structural decay)
- Vocabulary (re-read tax, ghost context, compaction loss, context dilution, etc.)
- Operational evidence with real numbers (sessions, costs, tokens saved)
- Model specialization philosophy (models as components, not monoliths)
- Tool screenshots showing concepts in action
- The control loop concept (inspect → detect → correct → continue)

NOT allowed:
- Dispatch mechanics (tokencontrol, runforge, codex exec patterns)
- Multi-model stack details (which model for what, switching logic)
- CLAUDE.md contents, skill files, hook implementations
- Agent workflow (work orders, plan mode usage, subagent patterns)
- Specific prompting techniques

### Level 3 — Internal (never published)

- Multi-model orchestration, dispatch strategies
- Agent configs (CLAUDE.md, SKILL.md, AGENTS.md, .claude/)
- Work orders, runbooks, plans
- Specific prompting techniques and model-switching logic
- Cost optimization strategies beyond what's in the essays

### Pre-publish Checklist

- [ ] Does it expose methodology without revealing machinery?
- [ ] Could a competitor reproduce the tooling from this alone? (Answer should be: no)
- [ ] Does it reference any Level 3 artifacts? (Must be: no)
- [ ] Is ContextSpectre positioned as implementation, not the concept?
- [ ] Are all screenshots sanitized (no session IDs, no private paths, no CLAUDE.md)?
- [ ] Is the tool mentioned last, after evidence and framework?

---

## Positioning

**Core thesis:** Context decay is the next infrastructure problem. It will define how human-AI collaboration is engineered.

This is the "observability" moment for AI reasoning. Observability was a concept before any specific tool existed. Vector control is the same — a practice that will outlive any specific implementation.

### Audience → Message → Medium

| Audience | Message | Medium |
|----------|---------|--------|
| Developers who feel the pain | "Your sessions degrade. Here's why, and here are the numbers." | Essay 1 |
| Cost-conscious teams/managers | "80% of your AI spend is cache reads re-processing noise." | Essay 2 |
| AI tooling builders | "Vector control is the practice. Here's the control loop." | Essay 3 |
| HN / broader tech community | "Real data from $2,400 of AI coding sessions." | Essay 4 |

### Voice

- Evidence-first, framework-second, tool-mention-last
- Not promotional. No call to action. No pricing.
- Credibility comes from: (1) Knuth reference, (2) real session data, (3) open source code
- Tone: systems engineer explaining a phenomenon, not a founder pitching a product

### Differentiation

Nobody else is talking about this:
- Cursor, Windsurf, Copilot — focus on code generation, not session management
- Bigger context windows are the industry "solution" — but more runway doesn't prevent decay
- No existing vocabulary for the failure modes (re-read tax, ghost context, compaction loss)
- First-mover on the framework gives naming authority

---

## HN Strategy

**Not ready yet.** Prerequisites:

- [ ] Essay 1 published on vectorcontrol.dev
- [ ] Essay 4 written with heavy data and screenshots
- [ ] vectorcontrol.dev live with clean design
- [ ] Tool has >50 GitHub stars (social proof minimum)
- [ ] At least 2 essays published (shows sustained thinking, not one-off)

**When ready:**

- Post Essay 4 as the HN link (the data essay, not the tool)
- NOT a Show HN. Not the repo. The essay URL.
- Title: data-forward. Example: "What $2,400 of AI Coding Sessions Taught Me About Context Decay"
- The essay builds credibility; the tool is discovered via link in closing paragraph
- Mention tool in HN comments only if asked
- Never post the tool repo directly — essays first, tool follows

**Anti-patterns to avoid:**
- "I built a tool" framing (triggers HN skepticism)
- Feature lists (nobody cares about commands)
- "AI will change everything" hyperbole (instant downvote)
- Comparing to competitors (invites flame wars)

---

## Content Calendar

| Phase | Action |
|-------|--------|
| **Now** | Finalize this roadmap. Save plan for Essay 1 polish. |
| **Next** | Polish Essay 1 to publication quality. Add screenshots + narrative. Extract raw material sections to per-essay outline files. |
| **Later** | Register domains. Set up static site (Hugo or Astro, minimal, fast). |
| **Later** | Write Essay 2 (economics — the money angle). |
| **Later** | Write Essay 4 (HN candidate — heavy data, story-first). |
| **Last** | HN post when all prerequisites met. |

No dates. The essays should be written when the data is rich and the tool is polished. Rushing weakens the evidence.

---

## Asset Requirements

### Screenshots (sanitized, Level 2 safe)

- `contextspectre stats` — real session showing context%, signal grade, CPD, cost
- `contextspectre watch` — live context bar with signal indicator
- TUI overview panel — vector gauge, entropy, cleanup recommendations
- Before/after cleanup — signal grade improving, noise ratio dropping
- Epoch cost breakdown — showing where money went

### Data

- Aggregate lifetime stats (total sessions, costs, tokens saved, cleanup cycles)
- The monster session narrative ($373, 8K+ entries)
- Compaction archaeology example (what was lost, what survived)
- Watch mode operational stats (811 cycles, 2.6M tokens, 18h runtime)

### Sanitization rules

- No real session UUIDs (use slugs or redact)
- No file paths that reveal internal project structure
- No CLAUDE.md or agent config content visible in screenshots
- No dispatch/orchestration artifacts visible
- Replace private project names with generic labels if needed

---

## What This is NOT

- Not the essays themselves. This is the editorial plan.
- Not a marketing strategy. No pricing, no funnel, no CTA.
- Not a site build. Domain registration and static site setup are separate tasks.
- Not urgent. Quality over speed. The data accumulates; the essays can wait.
