package analyzer

import (
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
	hasExternal   bool
	hasCWD        bool
	externalRepos map[string]bool // external repo root paths
}

// AnalyzeScopeDrift performs per-epoch scope analysis and tangent sequence detection.
// cwd can be provided explicitly or "" to auto-detect from entries.
func AnalyzeScopeDrift(entries []jsonl.Entry, compactions []CompactionEvent, cwd string) *ScopeDrift {
	result := &ScopeDrift{}

	if cwd == "" {
		cwd = detectSessionCWD(entries)
	}
	if cwd == "" {
		return result
	}
	result.SessionProject = cwd

	// Build per-entry scope info
	infos := classifyEntryScopes(entries, cwd)

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
	result.TangentSeqs = detectTangentSequences(entries, infos, cwd, model)

	return result
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

// classifyEntryScopes builds scope classification for each entry.
func classifyEntryScopes(entries []jsonl.Entry, cwd string) []entryScopeInfo {
	infos := make([]entryScopeInfo, len(entries))
	for i, e := range entries {
		paths, _ := extractAllPaths(e)
		repos := make(map[string]bool)
		for _, p := range paths {
			if isOutsideCWD(p, cwd) {
				infos[i].hasExternal = true
				root := externalRootDir(p, cwd)
				repos[root] = true
			} else {
				infos[i].hasCWD = true
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

		for i := r.start; i < r.end; i++ {
			info := infos[i]
			// Only count entries that have path refs
			if !info.hasExternal && !info.hasCWD {
				continue
			}

			// Mixed refs (both CWD and external) count as in-scope
			if info.hasCWD {
				es.InScope++
			} else if info.hasExternal {
				es.OutScope++
				for repo := range info.externalRepos {
					es.OutScopeByRepo[repo]++
				}
				// Attribute assistant turn cost to drift
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
func detectTangentSequences(entries []jsonl.Entry, infos []entryScopeInfo, cwd, model string) []TangentSequence {
	pricing := PricingForModel(model)

	// Reuse the tangent detection infrastructure from tangents.go
	// Build entryInfo for tangent detection
	tangentInfos := make([]entryInfo, len(entries))
	for i := range entries {
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
		// Skip entries that reference CWD or are non-external
		if !tangentInfos[i].refsExternal || tangentInfos[i].refsCWD || tangentInfos[i].modifiesCWD {
			i++
			continue
		}

		// Found start of potential tangent sequence
		start := i
		repoCount := make(map[string]int)
		totalTokens := 0
		var dollarCost float64
		var indices []int

		for i < len(entries) {
			info := tangentInfos[i]

			if info.refsCWD || info.modifiesCWD {
				break
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

		// Detect re-explanation files: CWD files read in first 5 entries after tangent
		reExplFiles := detectReExplanationFiles(entries, end+1, cwd)

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
