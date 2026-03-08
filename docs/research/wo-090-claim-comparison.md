# WO-090: Claim Comparison — Research Findings

**Date:** 2026-03-08
**Status:** Complete
**Decision:** Identity tracking over content comparison (TCP model)

## Problem

The spar-extract protocol, dead branch detection, and restatement inflation measurement all depend on one primitive operation: given two claims, are they the same claim in different words?

Without this, deduplication is manual. With it, checkpoint extraction can be automated — "did this exchange produce a new claim or restate an existing one?"

Examples from real sessions:

| Claim A | Claim B | Same? |
|---------|---------|-------|
| "Use Cobra for CLI commands" | "CLI should use Cobra framework" | Yes |
| "Keep conclusions, remove scaffolding" | "Exploratory reasoning becomes dead weight after decisions" | Yes (semantically) |
| "Use Redis for caching" | "Add a caching layer" | Partially (A is more specific) |
| "Tests are mandatory" | "Coverage target >85%" | No (related but different claims) |

The core tension: deterministic approaches can't handle synonyms ("branching" = "exploration", "gems" = "insights"). Embeddings handle synonyms but violate the no-ML philosophy. The research question is whether the gap between deterministic and ML is small enough to live with, or whether a different framing eliminates the gap.

## Existing Infrastructure

### ContextSpectre decision extraction

Decisions are currently identified, not compared:

- **Archaeology hints** — keyword matching: "decided", "chose", "going with", "opted", "trade-off", "rather than" (`archaeology.go:221-237`)
- **Commit points** — explicit user markers with Goal + Decisions[] + Constraints[] (`markers.go`)
- **Decision economics** — CPD/TTC/CDR metrics count decisions but don't deduplicate them (`cost.go:296-362`)
- **Lineage** — traces decisions across sessions by label substring match (`lineage.go:118-140`)

None of these compare claim content. `lineage --decision "auth"` finds all commit points containing "auth" in goal or decisions — but it can't tell if two different-worded decisions about authentication are the same decision.

### VectorPad drift detection

6 production-grade meaning axes with `drift.Detect(original, rewritten)`:

| Axis | What it detects | Example |
|------|----------------|---------|
| Modality | can/must/should/will/might strength changes | "should" → "must" = upgrade |
| Negation | Polarity flips | "can" → "cannot" = flip |
| Numeric | Number and unit changes | "3 retries" → "5 retries" |
| Scope | Quantifier boundary changes | "at least" → "at most" |
| Conditional | Guard condition changes | "if enabled" → "unless disabled" |
| Commitment | Certainty level changes | "I think" → "definitely" |

These detect meaning *changes*, not equivalence. But they can be inverted: if `drift.Detect(a, b)` produces zero drifts across all axes, the two texts are structurally equivalent — they make the same modal, numeric, and conditional commitments.

## Approaches Evaluated

### 1. Token Overlap (Jaccard on Content Words)

**Method:** Stem both claims, remove stop words, compute Jaccard similarity (intersection / union) on remaining content words. Threshold at 0.6 for "same claim."

**Analysis:**

```
"Use Cobra for CLI commands"
  stems: {use, cobra, cli, command}

"CLI should use Cobra framework"
  stems: {cli, should, use, cobra, framework}

Jaccard = |{use, cobra, cli}| / |{use, cobra, cli, command, should, framework}|
        = 3/6 = 0.50 → below threshold → MISS (false negative)
```

The problem is clear: even near-identical claims with different function words score below threshold.

| Metric | Estimate | Reasoning |
|--------|----------|-----------|
| Precision | ~90% | High overlap usually means same claim |
| Recall | ~40% | Misses synonyms, rephrasing, structural restatements |
| Speed | O(n) | Fast — stemming + set operations |
| Determinism | 100% | No external dependencies |

**Verdict:** Useful as a fast first-pass filter. Catches exact and near-exact restatements. Must be combined with other approaches.

### 2. Structural Fingerprint (VectorPad Drift Axes)

**Method:** Two claims are "structurally equivalent" if:
1. `drift.Detect(a, b).Allowed == true` (no meaning drift)
2. They share the same structural markers (numbers, scope quantifiers, conditionals)

**Analysis:**

VectorPad's axes are designed to catch meaning-*altering* rewrites, not meaning-*preserving* ones. Zero drift means "no dangerous semantic shift" — but it doesn't mean "same claim."

Problem case: Two completely different claims can both have zero drift if neither contains modals, negation, numbers, scope, conditionals, or commitment markers:

```
"Use Redis for caching"        → axes: {} (no markers)
"Deploy to Kubernetes"          → axes: {} (no markers)
drift.Detect(a, b) → zero drift → FALSE POSITIVE
```

Conversely, two restatements of the same claim can trigger drift if one adds a qualifier:

```
"Tests are mandatory"           → axes: {}
"Tests must pass"               → axes: {modality: "must"}
drift.Detect(a, b) → modality added → FALSE NEGATIVE
```

| Metric | Estimate | Reasoning |
|--------|----------|-----------|
| Precision | ~70% | Zero-drift can be coincidental for marker-free claims |
| Recall | ~50% | Catches structural restatements, misses semantic ones |
| Speed | O(n) | Fast — regex matching per axis |
| Determinism | 100% | Regex-based, no ML |

**Verdict:** Useful as a second filter layer after token overlap. The combination (token overlap > 0.3 AND zero drift) raises precision to ~85% at the cost of some recall.

### 3. Output Equivalence

**Method:** If two claims produce the same observable output (commit point, WO, code change, decision), they are functionally equivalent regardless of wording.

**Analysis:**

This is the most philosophically consistent approach — compare what claims *produce*, not what they *say*. It sidesteps the synonym problem entirely.

Current infrastructure for output mapping:
- Commit points have Goal + Decisions[] — two commit points with matching goals are output-equivalent
- Archaeology hints map to compaction epochs — two hints in the same epoch addressing the same files are likely output-equivalent
- WO references in user text (`WO-\d+` regex in `export_tasks.go`) provide explicit output mapping

Limitation: only ~30% of claims have observable outputs. Most claims are exploratory, not decision-producing. The approach is highly trustworthy when applicable but has narrow coverage.

| Metric | Estimate | Reasoning |
|--------|----------|-----------|
| Precision | ~95% | Same output ≈ same claim (rare exceptions) |
| Recall | ~30% | Only works for acted-upon claims |
| Speed | O(n) | Requires output registry lookup |
| Determinism | 100% | String matching on outputs |

**Verdict:** The most trustworthy signal when available. Best as a validation layer, not a primary detector.

### 4. LLM Judge (Haiku, $0.001/comparison)

**Method:** Send claim pairs to Haiku 4.5 with a structured prompt: "Are these the same claim stated differently? Respond: same/different/partial + confidence 0-1."

**Analysis:**

Effective but philosophically incompatible with ContextSpectre's "structural detection over semantic guessing" principle. The judge would:
- Handle synonyms perfectly
- Understand context and implication
- Produce non-reproducible results (temperature, version changes)
- Create an API dependency for a local-first tool
- Cost ~$0.001/comparison (~$0.10 for a 100-claim session)

| Metric | Estimate | Reasoning |
|--------|----------|-----------|
| Precision | ~95% | LLMs are good at semantic comparison |
| Recall | ~90% | Handles synonyms, paraphrasing, context |
| Speed | ~500ms | API round-trip |
| Determinism | ~0% | Non-reproducible |

**Verdict:** Not recommended as a default. If needed for specific use cases, must be opt-in and clearly labeled as non-deterministic.

### 5. TCP Model (Identity at Creation)

**Method:** Don't compare claim content at all. Assign identity at creation, detect duplicates by ID.

**The TCP analogy:**

| TCP Concept | Claim System Equivalent |
|-------------|------------------------|
| Sequence number | Claim ID (assigned at first assertion) |
| ACK | Claim captured at checkpoint extraction |
| Duplicate detection | Same ID, no new payload = restatement |
| Window size | Max unACKed claims before mandatory checkpoint |
| Retransmission | Legitimate clarification (receiver didn't ACK) |
| Congestion control | Too many unACKed claims = back off and extract |

**Who assigns claim IDs?**

| Option | Analogy | Implementation |
|--------|---------|---------------|
| VectorPad tags at authoring | Sender assigns (like TCP) | VectorPad adds `[claim:C001]` tags to directives before sending |
| Checkpoint extraction assigns | Receiver assigns (like a registry) | ContextSpectre assigns IDs when claims first appear in commit points |
| Auto-detect from first assertion | Hybrid | First unique assertion per exchange gets auto-ID |

**Option (a) — VectorPad as sender — is most natural.** VectorPad already has the text before it enters the context window. Adding a claim ID is a lightweight pre-flight operation. ContextSpectre then tracks which IDs have been ACKed (captured in commit points) and which are still unACKed (floating in context).

**Restatement detection becomes trivial:**

```
Turn 1: [claim:C001] "Use Cobra for CLI"
Turn 15: [claim:C001] "CLI should use Cobra framework"  ← restatement (same ID)
Turn 30: [claim:C002] "Tests are mandatory"              ← new claim (new ID)
Turn 45: [claim:C001] (still no ACK)                     ← window pressure
```

**Implementation sketch:**

```
VectorPad:
  - claimRegistry: map[string]Claim  (ID → first assertion text + timestamp)
  - nextClaimID(): monotonic counter with session prefix
  - tagClaims(text): scan for assertion patterns, assign IDs to new, tag existing
  - exportClaims(): serialize registry for ContextSpectre

ContextSpectre:
  - importClaims(registry): load claim IDs from VectorPad export
  - matchClaimIDs(entries): scan for [claim:XXXX] tags in session content
  - restatementReport(): claims asserted N times without ACK
  - inflationScore(): restatement count / total claim mentions
```

| Metric | Estimate | Reasoning |
|--------|----------|-----------|
| Precision | 100% | ID match is exact |
| Recall | 100% (new) / 0% (legacy) | Only works when IDs are present |
| Speed | O(1) per comparison | Hash lookup |
| Determinism | 100% | ID comparison |

**Verdict:** The recommended direction. Makes all semantic comparison approaches unnecessary for new sessions. Legacy sessions fall back to the tiered deterministic approach.

## Recommendation

### Primary: TCP Model for New Sessions

VectorPad assigns claim IDs at authoring time. ContextSpectre's checkpoint extraction uses IDs to detect restatements. No semantic comparison needed. This is philosophically consistent — identity tracking is structural, not semantic.

**Implementation path:**
1. Add claim registry to VectorPad (WO in VectorPad)
2. Add claim ID detection to ContextSpectre's archaeology (WO in ContextSpectre)
3. Add restatement inflation metric to decision economics

### Fallback: Tiered Deterministic for Legacy Sessions

For sessions without claim IDs:

```
Token overlap (Jaccard > 0.3)     → ~40% recall, ~90% precision
  ↓ (remaining)
Structural fingerprint (zero drift) → +10% recall
  ↓ (remaining)
Output equivalence (same WO/commit) → +10% recall (when available)

Total: ~60% recall, ~85% precision
```

The ~40% that's missed is acceptable — legacy sessions are retrospective analysis, not real-time. The false negatives are semantic restatements that would require NLP to catch, and we've decided that's outside scope.

### Not Recommended: LLM Judge

Effective (~95%/~90%) but violates deterministic philosophy. If a future use case requires it, it must be:
- Opt-in via configuration
- Clearly labeled in output as "non-deterministic"
- Not a default path

## Decision

**Identity tracking over content comparison.** The TCP model eliminates the semantic comparison problem by reframing it as an identity assignment problem. This is consistent with ContextSpectre's philosophy: structural detection over semantic guessing. The tool assigns identity at creation and tracks it through the session — it doesn't guess whether two different texts mean the same thing.

The core insight: claim comparison is hard because it's the wrong problem. The right problem is claim identity — and identity is assigned, not discovered.

## Open Questions

1. **Tag format:** `[claim:C001]` in text vs. structured metadata in VectorPad export?
2. **Claim granularity:** Is "Use Cobra for CLI" one claim or two ("use Cobra" + "for CLI")?
3. **Cross-session claims:** Same claim across sessions — do they share IDs or get session-scoped IDs?
4. **Bootstrap:** How to retrofit IDs onto legacy sessions — run tiered deterministic once, assign IDs to detected clusters?

## Connected Work

- VectorPad: needs claim registry infrastructure
- ContextSpectre WO-064 (idea markers, branch ROI): claim comparison would make branch ROI calculable
- ContextSpectre decision economics: restatement inflation as a new CDR component
- VectorPad negative space analysis: untagged claims as a gap class
