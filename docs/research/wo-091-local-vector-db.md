# WO-091: Local Vector DB for Stashed Ideas — Research Findings

**Date:** 2026-03-08
**Status:** Complete
**Depends on:** WO-090 (claim comparison — complete), WO-087 (VectorPad stash)
**Decision:** Viable. SQLite + local embeddings + cosine similarity. Embeddings supplement TCP claim IDs, don't replace them.

## Problem

Sparring sessions produce gems at ~5-15% survival rate. The gems go into WO notes, but there's no structured way to detect duplicates, see clusters, track idea evolution, or measure distances between ideas. Cost context: $40 spent panning 3 gems across spar sessions — the extraction process needs a registry to avoid re-mining the same vein.

## Architecture Evaluation

### Storage: SQLite

**Verdict: Use it.**

| Factor | Assessment |
|--------|-----------|
| Portability | Single file, no server, already on every Mac/Linux |
| Performance | Handles 10K+ rows with vector columns trivially |
| Vector support | No native vector type, but BLOB column + Go math works fine |
| Concurrency | WAL mode supports concurrent reads, single writer — sufficient for CLI tool |
| Backup | Copy one file. Fits contextspectre's backup philosophy |

SQLite is the obvious choice. No need for Postgres, Pinecone, or any vector-specific DB at this scale. A stash of 1,000 ideas with 768-dim embeddings is ~3MB — fits in memory, no indexing needed.

**Schema sketch:**

```sql
CREATE TABLE stash (
    id          TEXT PRIMARY KEY,    -- stash-001, monotonic
    title       TEXT NOT NULL,
    type        TEXT NOT NULL,       -- insight, question, pattern, constraint
    project     TEXT,
    raw_text    TEXT NOT NULL,
    tags        TEXT,                -- JSON array
    refs        TEXT,                -- JSON array of WO/claim refs
    claim_id    TEXT,                -- TCP claim ID from WO-090, nullable
    embedding   BLOB,               -- float32 array, 768 dims
    created_at  TEXT NOT NULL,       -- ISO 8601
    updated_at  TEXT
);

CREATE TABLE similarity_cache (
    id_a        TEXT NOT NULL,
    id_b        TEXT NOT NULL,
    score       REAL NOT NULL,       -- cosine similarity
    computed_at TEXT NOT NULL,
    PRIMARY KEY (id_a, id_b)
);
```

The similarity cache avoids recomputing pairwise similarities on every query. Invalidate when either stash entry is updated.

### Embeddings: Ollama + nomic-embed-text

**Verdict: Viable with caveats.**

| Factor | Assessment |
|--------|-----------|
| Local execution | Ollama runs locally, no cloud dependency |
| Reproducibility | Same model + input = same vector (deterministic at inference) |
| Model size | nomic-embed-text: ~274MB download, ~500MB RAM |
| Dimension | 768 dimensions — standard, good balance of quality/size |
| Speed | ~50ms per embedding on M-series Mac |
| Quality | Trained on diverse text, handles short claims well |
| Availability | Requires Ollama installed + model pulled |

**Caveats:**

1. **Ollama dependency.** Not everyone has Ollama installed. The stash command should degrade gracefully: store ideas without embeddings when Ollama is unavailable, compute embeddings lazily when it becomes available.

2. **Model versioning.** If nomic-embed-text is updated, existing embeddings become incomparable. Mitigation: store model version in DB metadata, warn when version mismatch detected, provide `stash reindex` command.

3. **Not truly deterministic across versions.** Same model version + same input = same vector. But model updates break this. This is acceptable — version-lock the model, reindex when upgrading.

**Alternative models evaluated:**

| Model | Dims | Size | Quality | Notes |
|-------|------|------|---------|-------|
| nomic-embed-text | 768 | 274MB | Good | Best balance for local use |
| all-minilm | 384 | 46MB | Decent | Smaller but lower quality on short text |
| mxbai-embed-large | 1024 | 670MB | Better | Overkill for claim-level text |
| bge-small | 384 | 67MB | Good | Alternative to all-minilm |

**Recommendation:** nomic-embed-text as default. Allow model override via config for users who want smaller/larger models.

### Similarity: Cosine Similarity

**Verdict: Standard approach, no issues.**

Cosine similarity on normalized embeddings is a dot product — trivial to implement in Go:

```go
func cosineSimilarity(a, b []float32) float64 {
    var dot, normA, normB float64
    for i := range a {
        dot += float64(a[i]) * float64(b[i])
        normA += float64(a[i]) * float64(a[i])
        normB += float64(b[i]) * float64(b[i])
    }
    return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
```

At 1,000 stash entries, brute-force comparison takes <1ms. No approximate nearest neighbor (ANN) indexing needed until 100K+ entries, which is years away.

### Similarity Thresholds

Without a calibrated test corpus, these thresholds are initial estimates based on embedding literature and the claim comparison context:

| Score | Interpretation | Action |
|-------|---------------|--------|
| >0.90 | Near-duplicate | Auto-link, warn "you already stashed this" |
| 0.80–0.90 | Same idea, different words | Suggest as related, ask operator |
| 0.65–0.80 | Related topic | Show in "see also" cluster |
| <0.65 | Different idea | No action |

These thresholds need calibration on real spar gems. The first 50 stashed ideas should include manual same/different labels to tune thresholds empirically.

**Calibration protocol:**
1. Stash 50 ideas from real spar sessions
2. Manually label all pairs: same / related / different
3. Compute embeddings, measure cosine similarity for each pair
4. Plot ROC curve, find optimal thresholds for each category
5. Store calibrated thresholds in config

## Integration with TCP Model (WO-090)

WO-090 concluded: identity tracking over content comparison. The question for WO-091 is: how do embeddings and TCP claim IDs interact?

**Answer: They're complementary layers, not alternatives.**

| Layer | Function | When used |
|-------|----------|-----------|
| TCP claim ID | Identity assignment | At authoring time (VectorPad assigns) |
| Embedding similarity | Discovery | At stash time (find related existing stash entries) |
| Cosine threshold | Deduplication hint | At stash time (warn if >0.90 match exists) |

**Flow:**

```
New idea arrives (from spar, manual entry, or extraction)
  ↓
Has claim ID? ──yes──→ Look up claim ID in stash registry
  │                      ↓
  │                    Found? → Restatement (update existing entry)
  │                    Not found? → New stash entry with this claim ID
  │
  no (legacy, untagged)
  ↓
Compute embedding → Compare against all stash embeddings
  ↓
Score > 0.90 → "This looks like stash-017. Same idea?"
Score 0.65-0.90 → "Related to: stash-003, stash-017"
Score < 0.65 → New stash entry, assign new claim ID
```

Embeddings solve the bootstrap problem from WO-090: how to assign claim IDs to ideas that arrive without them. The embedding finds the closest existing stash entry, and the operator confirms whether it's the same claim (assigning the existing ID) or a new one (getting a new ID).

## Scope: Per-Project vs Global

**Recommendation: Global DB with project tags.**

- Ideas cross project boundaries ("branching is the gem generator" applies to both VectorPad and ContextSpectre)
- Per-project DBs would fragment the stash and miss cross-project duplicates
- Project field on each entry enables project-scoped queries when needed
- Single file: `~/.vectorpad/stash.db` (or `~/.claude/stash.db` if co-located with ContextSpectre data)

## Philosophy: ML Tension

The WO notes the tension: embeddings are ML, which conflicts with "determinism over ML."

**Resolution: Embeddings are infrastructure, not decisions.**

The distinction that matters is *who decides*:

| Component | Role | Decides? |
|-----------|------|----------|
| Embedding model | Computes similarity score | No — produces a number |
| Cosine threshold | Flags potential duplicates | No — surfaces candidates |
| Operator | Confirms same/different | **Yes** — final authority |

This is the same pattern as ContextSpectre's signal grade: the tool computes A-F, but the operator decides what to do about it. Embeddings are a similarity *mirror*, not a deduplication *oracle*.

Additional mitigations:
- Local model only — no cloud API, no data leaves the machine
- Fixed model version — reproducible results within a version
- Scores always shown — the operator sees the number, not a binary same/different
- Override always available — operator can mark two high-similarity ideas as distinct, or two low-similarity ideas as the same

## Implementation Sketch

### Commands (VectorPad, not ContextSpectre)

```
vectorpad stash add "branching is the gem generator" --type insight --project contextspectre
vectorpad stash list [--project <name>] [--type <type>] [--tag <tag>]
vectorpad stash compare "new idea text"              # find similar stashed ideas
vectorpad stash show <id>                             # show stash entry with similar entries
vectorpad stash cluster                               # show idea clusters by proximity
vectorpad stash evolve <claim-id>                     # show how a claim's phrasing changed
vectorpad stash reindex                               # recompute all embeddings (after model upgrade)
```

### Dependencies

| Dependency | Required? | Notes |
|-----------|-----------|-------|
| SQLite | Yes | Go stdlib `database/sql` + `github.com/mattn/go-sqlite3` |
| Ollama | Optional | Degrades gracefully without it |
| nomic-embed-text | Optional | Pulled via `ollama pull nomic-embed-text` |

### Go packages

```
internal/stash/
  db.go        — SQLite operations (open, migrate, CRUD)
  embed.go     — Ollama embedding client (HTTP to localhost:11434)
  similarity.go — cosine similarity, threshold logic
  cluster.go   — proximity clustering (greedy, no ML)

internal/commands/
  stash.go     — CLI commands (add, list, compare, show, cluster, evolve, reindex)
```

## Open Questions

1. **Embedding on stash vs. on compare?** Stash-time embedding is eager (compute once, store). Compare-time is lazy (only compute when comparing). Recommendation: eager — storage is cheap, and it enables clustering without re-embedding.

2. **Similarity cache invalidation.** When a stash entry is updated, its pairwise similarities become stale. Invalidate by deleting rows from `similarity_cache` where `id_a` or `id_b` matches the updated entry.

3. **Cluster algorithm.** Greedy agglomerative: start with each idea as its own cluster, merge pairs with score > 0.70, repeat until no merges possible. Simple, deterministic, O(n^2) but fine at expected scale.

4. **VectorPad or ContextSpectre?** The stash is a VectorPad feature (pre-flight, authoring-side). ContextSpectre could read the stash DB for cross-referencing (e.g., `lineage --stash` to see which stashed ideas a session touched), but the primary interface belongs in VectorPad.

## Decision

**Viable. Build it in VectorPad.** SQLite + nomic-embed-text + cosine similarity is the right stack. Embeddings supplement TCP claim IDs — they solve the bootstrap problem (assigning IDs to untagged ideas) and enable discovery (finding related ideas the operator didn't know were related). The operator always decides; the DB surfaces candidates.

## Connected Work

- WO-090 (complete): TCP model for claim identity — stash DB is the physical registry
- WO-087 (VectorPad): Stash schema — the data model this DB stores
- VectorPad spar-extract: Output target for extracted gems
- ContextSpectre lineage: Could cross-reference stash IDs in session content
