package analyzer

import (
	"math"
	"path/filepath"
	"sort"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// ScopeDrift holds the complete scope drift analysis for a session.
type ScopeDrift struct {
	SessionProject string            // CWD detected from entries
	EpochScopes    []EpochScope      // per-epoch scope distribution
	TangentSeqs    []TangentSequence // contiguous out-of-scope sequences
	TotalInScope   int
	TotalOutScope  int
	OverallDrift   float64 // TotalOutScope / (TotalInScope + TotalOutScope)
}

// EpochScope holds scope distribution for a single compaction epoch.
type EpochScope struct {
	EpochIndex     int
	InScope        int            // entries with CWD path refs
	OutScope       int            // entries with external path refs
	MixedScope     int            // entries referencing both CWD and external paths
	OutScopeByRepo map[string]int // external repo root -> count
	DriftRatio     float64
	DriftCost      float64 // dollar cost of out-of-scope assistant turns
}

// TangentSequence is a contiguous block of entries primarily about another project.
type TangentSequence struct {
	StartIdx           int
	EndIdx             int
	TargetRepo         string // dominant external repo (basename)
	EntryIndices       []int
	TokenCost          int
	DollarCost         float64
	ReExplanationFiles []string // CWD files re-read after tangent
}

// entryScopeInfo holds scope classification for a single entry.
type entryScopeInfo struct {
	hasExternal       bool
	hasCWD            bool
	externalRepos     map[string]bool // external repo root paths
	cwdPathCount      int
	externalPathCount int
}

// AnalyzeScopeDrift performs per-epoch scope analysis and tangent sequence detection.
// cwd can be provided explicitly or "" to auto-detect from entries.
func AnalyzeScopeDrift(entries []jsonl.Entry, compactions []CompactionEvent, cwd string) *ScopeDrift {
	result := &ScopeDrift{}

	if cwd == "" {
		cwd = DetectSessionCWD(entries)
	}
	if cwd == "" {
		return result
	}
	result.SessionProject = cwd

	// Build dynamic CWD map — tracks CWD changes through the session
	cwds := buildActiveCWDMap(entries, cwd)

	// Build per-entry scope info using per-entry CWDs
	infos := classifyEntryScopes(entries, cwds)

	// Detect model for cost computation
	model := detectModel(entries)

	// Build epoch boundaries (same logic as CalculateEpochCosts)
	boundaries := make([]int, len(compactions))
	for i, c := range compactions {
		boundaries[i] = c.LineIndex
	}

	// Segment entries into epochs and compute per-epoch scope
	result.EpochScopes = computeEpochScopes(entries, infos, boundaries, model)

	// Aggregate totals
	for _, es := range result.EpochScopes {
		result.TotalInScope += es.InScope
		result.TotalOutScope += es.OutScope
	}
	total := result.TotalInScope + result.TotalOutScope
	if total > 0 {
		result.OverallDrift = float64(result.TotalOutScope) / float64(total)
	}

	// Detect tangent sequences
	result.TangentSeqs = detectTangentSequences(entries, infos, cwds, model)

	return result
}

// buildActiveCWDMap returns a per-entry CWD slice. Each entry's active CWD
// is the most recent non-empty CWD seen up to that point.
func buildActiveCWDMap(entries []jsonl.Entry, initialCWD string) []string {
	cwds := make([]string, len(entries))
	active := initialCWD
	for i, e := range entries {
		if e.CWD != "" {
			active = e.CWD
		}
		cwds[i] = active
	}
	return cwds
}

// pathMixRatio returns the fraction of external paths for a mixed-scope entry.
// Returns 0 if the entry has no paths.
func pathMixRatio(info entryScopeInfo) float64 {
	total := info.cwdPathCount + info.externalPathCount
	if total == 0 {
		return 0
	}
	return float64(info.externalPathCount) / float64(total)
}

// DriftIndices returns all entry indices flagged as out-of-scope.
func (d *ScopeDrift) DriftIndices() map[int]bool {
	if d == nil {
		return nil
	}
	m := make(map[int]bool)
	// Include all entries from tangent sequences
	for _, ts := range d.TangentSeqs {
		for _, idx := range ts.EntryIndices {
			m[idx] = true
		}
	}
	return m
}

// DriftRepoForIndex returns the dominant external repo basename for a given entry,
// or "" if the entry is not in a tangent sequence.
func (d *ScopeDrift) DriftRepoForIndex(idx int) string {
	if d == nil {
		return ""
	}
	for _, ts := range d.TangentSeqs {
		for _, i := range ts.EntryIndices {
			if i == idx {
				return ts.TargetRepo
			}
		}
	}
	return ""
}

// classifyEntryScopes builds scope classification for each entry using per-entry CWDs.
func classifyEntryScopes(entries []jsonl.Entry, cwds []string) []entryScopeInfo {
	infos := make([]entryScopeInfo, len(entries))
	for i, e := range entries {
		cwd := cwds[i]
		if cwd == "" {
			continue
		}
		paths, _ := extractAllPaths(e)
		repos := make(map[string]bool)
		for _, p := range paths {
			if isOutsideCWD(p, cwd) {
				infos[i].hasExternal = true
				infos[i].externalPathCount++
				root := externalRootDir(p, cwd)
				repos[root] = true
			} else {
				infos[i].hasCWD = true
				infos[i].cwdPathCount++
			}
		}
		if len(repos) > 0 {
			infos[i].externalRepos = repos
		}
	}
	return infos
}

// computeEpochScopes segments entries by compaction boundaries and computes
// scope distribution per epoch.
func computeEpochScopes(entries []jsonl.Entry, infos []entryScopeInfo, boundaries []int, model string) []EpochScope {
	pricing := PricingForModel(model)

	type epochRange struct {
		start, end int
	}
	var ranges []epochRange

	// Build epoch ranges from boundaries
	start := 0
	boundaryPos := 0
	for i := range entries {
		if boundaryPos < len(boundaries) && i >= boundaries[boundaryPos] {
			ranges = append(ranges, epochRange{start, i})
			start = i
			boundaryPos++
		}
	}
	ranges = append(ranges, epochRange{start, len(entries)})

	var scopes []EpochScope
	for epochIdx, r := range ranges {
		es := EpochScope{
			EpochIndex:     epochIdx,
			OutScopeByRepo: make(map[string]int),
		}
		var driftCost float64
		var inScopeAcc, outScopeAcc float64

		for i := r.start; i < r.end; i++ {
			info := infos[i]
			// Only count entries that have path refs
			if !info.hasExternal && !info.hasCWD {
				continue
			}

			if info.hasCWD && info.hasExternal {
				// Mixed scope: proportional attribution
				ratio := pathMixRatio(info)
				inScopeAcc += 1.0 - ratio
				outScopeAcc += ratio
				es.MixedScope++
				for repo := range info.externalRepos {
					es.OutScopeByRepo[repo]++
				}
				// Attribute proportional cost for mixed entries
				e := entries[i]
				if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Usage != nil {
					u := e.Message.Usage
					cost := float64(u.InputTokens)/1_000_000*pricing.InputPerMillion +
						float64(u.OutputTokens)/1_000_000*pricing.OutputPerMillion +
						float64(u.CacheCreationInputTokens)/1_000_000*pricing.CacheWritePerMillion +
						float64(u.CacheReadInputTokens)/1_000_000*pricing.CacheReadPerMillion
					driftCost += cost * ratio
				}
			} else if info.hasCWD {
				inScopeAcc += 1.0
			} else if info.hasExternal {
				outScopeAcc += 1.0
				for repo := range info.externalRepos {
					es.OutScopeByRepo[repo]++
				}
				// Attribute full cost for purely out-of-scope entries
				e := entries[i]
				if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Usage != nil {
					u := e.Message.Usage
					cost := float64(u.InputTokens)/1_000_000*pricing.InputPerMillion +
						float64(u.OutputTokens)/1_000_000*pricing.OutputPerMillion +
						float64(u.CacheCreationInputTokens)/1_000_000*pricing.CacheWritePerMillion +
						float64(u.CacheReadInputTokens)/1_000_000*pricing.CacheReadPerMillion
					driftCost += cost
				}
			}
		}

		es.InScope = int(math.Round(inScopeAcc))
		es.OutScope = int(math.Round(outScopeAcc))

		total := es.InScope + es.OutScope
		if total > 0 {
			es.DriftRatio = float64(es.OutScope) / float64(total)
		}
		es.DriftCost = driftCost

		scopes = append(scopes, es)
	}

	return scopes
}

// detectTangentSequences finds contiguous blocks of entries primarily about other projects.
// Similar to FindTangents but adds cost attribution and re-explanation file detection.
// Uses per-entry CWDs for dynamic root tracking.
func detectTangentSequences(entries []jsonl.Entry, infos []entryScopeInfo, cwds []string, model string) []TangentSequence {
	pricing := PricingForModel(model)

	// Build entryInfo for tangent detection
	tangentInfos := make([]entryInfo, len(entries))
	for i := range entries {
		cwd := cwds[i]
		if infos[i].hasExternal {
			tangentInfos[i].refsExternal = true
			for repo := range infos[i].externalRepos {
				tangentInfos[i].externalPaths = append(tangentInfos[i].externalPaths, repo)
			}
		}
		if infos[i].hasCWD {
			tangentInfos[i].refsCWD = true
		}
		// Check for CWD modification
		paths, tools := extractAllPaths(entries[i])
		for j, p := range paths {
			if !isOutsideCWD(p, cwd) && j < len(tools) && isModifyingTool(tools[j]) {
				tangentInfos[i].modifiesCWD = true
			}
		}
	}

	var sequences []TangentSequence

	i := 0
	for i < len(entries) {
		info := tangentInfos[i]
		// Start of tangent requires purely external (not mixed)
		if !info.refsExternal || (info.refsCWD && !info.refsExternal) || info.modifiesCWD {
			if !info.refsExternal || info.modifiesCWD {
				i++
				continue
			}
			// Mixed scope at start — don't start tangent
			if info.refsCWD {
				i++
				continue
			}
		}

		// Found start of potential tangent sequence
		start := i
		repoCount := make(map[string]int)
		totalTokens := 0
		var dollarCost float64
		var indices []int

		for i < len(entries) {
			info := tangentInfos[i]

			// CWD modification always breaks a tangent
			if info.modifiesCWD {
				break
			}

			// Pure CWD reference (no external) breaks the tangent
			if info.refsCWD && !info.refsExternal {
				break
			}

			// Mixed scope: include at proportional token cost
			if info.refsCWD && info.refsExternal {
				ratio := pathMixRatio(infos[i])
				for repo := range infos[i].externalRepos {
					repoCount[repo]++
				}
				totalTokens += int(float64(entries[i].RawSize/4) * ratio)
				indices = append(indices, i)

				e := entries[i]
				if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Usage != nil {
					u := e.Message.Usage
					cost := float64(u.InputTokens)/1_000_000*pricing.InputPerMillion +
						float64(u.OutputTokens)/1_000_000*pricing.OutputPerMillion +
						float64(u.CacheCreationInputTokens)/1_000_000*pricing.CacheWritePerMillion +
						float64(u.CacheReadInputTokens)/1_000_000*pricing.CacheReadPerMillion
					dollarCost += cost * ratio
				}
				i++
				continue
			}

			if info.refsExternal {
				for repo := range infos[i].externalRepos {
					repoCount[repo]++
				}
				totalTokens += entries[i].RawSize / 4
				indices = append(indices, i)

				// Cost attribution for assistant turns
				e := entries[i]
				if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Usage != nil {
					u := e.Message.Usage
					cost := float64(u.InputTokens)/1_000_000*pricing.InputPerMillion +
						float64(u.OutputTokens)/1_000_000*pricing.OutputPerMillion +
						float64(u.CacheCreationInputTokens)/1_000_000*pricing.CacheWritePerMillion +
						float64(u.CacheReadInputTokens)/1_000_000*pricing.CacheReadPerMillion
					dollarCost += cost
				}
				i++
				continue
			}

			// Non-conversational entries (progress, snapshots) between tangent entries
			if !entries[i].IsConversational() {
				totalTokens += entries[i].RawSize / 4
				indices = append(indices, i)
				i++
				continue
			}

			// Conversational entry with no path refs
			if isResponseToTangent(entries, tangentInfos, i, start) {
				totalTokens += entries[i].RawSize / 4
				indices = append(indices, i)

				e := entries[i]
				if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Usage != nil {
					u := e.Message.Usage
					cost := float64(u.InputTokens)/1_000_000*pricing.InputPerMillion +
						float64(u.OutputTokens)/1_000_000*pricing.OutputPerMillion +
						float64(u.CacheCreationInputTokens)/1_000_000*pricing.CacheWritePerMillion +
						float64(u.CacheReadInputTokens)/1_000_000*pricing.CacheReadPerMillion
					dollarCost += cost
				}
				i++
				continue
			}

			break
		}

		end := i - 1
		if end < start || len(indices) < 2 {
			continue
		}

		// Determine dominant repo
		targetRepo := dominantRepo(repoCount)

		// Detect re-explanation files using the active CWD at the end of the tangent
		activeCWD := cwds[end]
		reExplFiles := detectReExplanationFiles(entries, end+1, activeCWD)

		sequences = append(sequences, TangentSequence{
			StartIdx:           start,
			EndIdx:             end,
			TargetRepo:         targetRepo,
			EntryIndices:       indices,
			TokenCost:          totalTokens,
			DollarCost:         dollarCost,
			ReExplanationFiles: reExplFiles,
		})
	}

	return sequences
}

// dominantRepo returns the basename of the most-referenced external repo.
func dominantRepo(repoCount map[string]int) string {
	if len(repoCount) == 0 {
		return ""
	}

	type repoEntry struct {
		path  string
		count int
	}
	var sorted []repoEntry
	for p, c := range repoCount {
		sorted = append(sorted, repoEntry{p, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	return filepath.Base(sorted[0].path)
}

// detectReExplanationFiles scans the first N entries after a tangent for CWD file reads.
func detectReExplanationFiles(entries []jsonl.Entry, afterIdx int, cwd string) []string {
	const lookAhead = 5
	seen := make(map[string]bool)
	var files []string

	end := afterIdx + lookAhead
	if end > len(entries) {
		end = len(entries)
	}

	for i := afterIdx; i < end; i++ {
		paths, tools := extractAllPaths(entries[i])
		for j, p := range paths {
			if isOutsideCWD(p, cwd) {
				continue
			}
			// Only count read-like tools (Read, Glob, Grep, etc.)
			if j < len(tools) && isModifyingTool(tools[j]) {
				continue
			}
			if !seen[p] {
				seen[p] = true
				files = append(files, p)
			}
		}
	}

	return files
}

// detectModel finds the model string from the first assistant entry with usage.
func detectModel(entries []jsonl.Entry) string {
	for _, e := range entries {
		if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Model != "" {
			return e.Message.Model
		}
	}
	return ""
}

// RangeMetadata holds computed metadata for an entry range.
type RangeMetadata struct {
	TargetRepo  string
	TokenCost   int
	DollarCost  float64
	ReExplFiles []string
}

// ComputeRangeMetadata computes tangent metadata for entries[from:to+1].
func ComputeRangeMetadata(entries []jsonl.Entry, from, to int, cwd string) *RangeMetadata {
	if from < 0 || to >= len(entries) || from > to {
		return &RangeMetadata{}
	}

	model := detectModel(entries)
	pricing := PricingForModel(model)

	meta := &RangeMetadata{}
	repoCount := make(map[string]int)

	for i := from; i <= to; i++ {
		e := entries[i]
		meta.TokenCost += e.RawSize / 4

		// Path analysis for repo detection
		paths, _ := extractAllPaths(e)
		for _, p := range paths {
			if isOutsideCWD(p, cwd) {
				root := externalRootDir(p, cwd)
				repoCount[root]++
			}
		}

		// Dollar cost from assistant usage
		if e.Type == jsonl.TypeAssistant && e.Message != nil && e.Message.Usage != nil {
			u := e.Message.Usage
			meta.DollarCost += float64(u.InputTokens)/1_000_000*pricing.InputPerMillion +
				float64(u.OutputTokens)/1_000_000*pricing.OutputPerMillion +
				float64(u.CacheCreationInputTokens)/1_000_000*pricing.CacheWritePerMillion +
				float64(u.CacheReadInputTokens)/1_000_000*pricing.CacheReadPerMillion
		}
	}

	meta.TargetRepo = dominantRepo(repoCount)
	meta.ReExplFiles = detectReExplanationFiles(entries, to+1, cwd)

	return meta
}
