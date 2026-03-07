package analyzer

import (
	"time"
)

// SessionEffInput holds per-session data for efficiency computation.
// Uses flat fields to avoid import cycles with session package.
type SessionEffInput struct {
	SessionID   string
	Slug        string
	Created     time.Time
	Modified    time.Time
	TotalTokens int
	Cost        float64
}

// GitCommit holds parsed commit data from git log.
type GitCommit struct {
	Hash      string
	Timestamp time.Time
	Files     []GitFileStat
}

// GitFileStat holds per-file line changes from a commit.
type GitFileStat struct {
	Path    string
	Added   int
	Deleted int
}

// EfficiencyResult holds the reasoning efficiency analysis for a project.
type EfficiencyResult struct {
	TotalTokens     int                 `json:"total_tokens"`
	TotalLOC        int                 `json:"total_loc"`
	TokensPerLOC    float64             `json:"tokens_per_loc"`
	Sessions        []SessionEfficiency `json:"sessions"`
	TopEfficient    []SessionEfficiency `json:"top_efficient"`
	LowYield        []SessionEfficiency `json:"low_yield"`
	ReasoningSinks  []SessionEfficiency `json:"reasoning_sinks"`
	PackageHotspots []PackageHotspot    `json:"package_hotspots"`
}

// SessionEfficiency holds efficiency metrics for a single session.
type SessionEfficiency struct {
	SessionID   string  `json:"session_id"`
	Slug        string  `json:"slug,omitempty"`
	TotalTokens int     `json:"total_tokens"`
	LOCAdded    int     `json:"loc_added"`
	Efficiency  float64 `json:"efficiency"` // LOC per 1000 tokens
	CommitCount int     `json:"commit_count"`
	Cost        float64 `json:"cost"`
}

// PackageHotspot shows reasoning cost aggregated by package directory.
type PackageHotspot struct {
	Package      string  `json:"package"`
	TotalTokens  int     `json:"total_tokens"`
	TotalLOC     int     `json:"total_loc"`
	SessionCount int     `json:"session_count"`
	Cost         float64 `json:"cost"`
}

// correlationBuffer is how long after a session ends we still associate commits.
const correlationBuffer = 30 * time.Minute

// ComputeEfficiency correlates sessions with git commits to produce efficiency metrics.
func ComputeEfficiency(sessions []SessionEffInput, commits []GitCommit) *EfficiencyResult {
	result := &EfficiencyResult{}

	// Build session efficiencies with commit correlation.
	sessionEffs := make([]SessionEfficiency, len(sessions))
	// Track package attribution: package → {tokens, loc, sessions set}.
	type pkgAccum struct {
		tokens   int
		loc      int
		sessions map[string]bool
		cost     float64
	}
	pkgMap := make(map[string]*pkgAccum)

	for i, s := range sessions {
		se := SessionEfficiency{
			SessionID:   s.SessionID,
			Slug:        s.Slug,
			TotalTokens: s.TotalTokens,
			Cost:        s.Cost,
		}
		result.TotalTokens += s.TotalTokens

		// Find commits that fall within this session's time range.
		var matchedCommits []GitCommit
		for _, c := range commits {
			if commitBelongsToSession(c.Timestamp, s.Created, s.Modified) {
				se.CommitCount++
				for _, f := range c.Files {
					se.LOCAdded += f.Added
				}
				matchedCommits = append(matchedCommits, c)
			}
		}

		// Attribute session tokens to packages.
		// Distribute session tokens evenly across all files in all matched commits.
		totalFiles := 0
		for _, c := range matchedCommits {
			totalFiles += len(c.Files)
		}
		if totalFiles > 0 {
			tokensPerFile := s.TotalTokens / totalFiles
			costPerFile := s.Cost / float64(totalFiles)
			for _, c := range matchedCommits {
				for _, f := range c.Files {
					pkg := packageFromPath(f.Path)
					if pkg == "" {
						continue
					}
					acc, ok := pkgMap[pkg]
					if !ok {
						acc = &pkgAccum{sessions: make(map[string]bool)}
						pkgMap[pkg] = acc
					}
					acc.loc += f.Added
					acc.tokens += tokensPerFile
					acc.cost += costPerFile
					acc.sessions[s.SessionID] = true
				}
			}
		}

		result.TotalLOC += se.LOCAdded
		if se.TotalTokens > 0 {
			se.Efficiency = float64(se.LOCAdded) / (float64(se.TotalTokens) / 1000)
		}
		sessionEffs[i] = se
	}

	result.Sessions = sessionEffs

	// Overall tokens per LOC.
	if result.TotalLOC > 0 {
		result.TokensPerLOC = float64(result.TotalTokens) / float64(result.TotalLOC)
	}

	// Classify sessions.
	result.TopEfficient = topEfficient(sessionEffs, 5)
	result.LowYield = lowYield(sessionEffs, 5)
	result.ReasoningSinks = reasoningSinks(sessionEffs, 5)

	// Build package hotspots.
	for pkg, acc := range pkgMap {
		result.PackageHotspots = append(result.PackageHotspots, PackageHotspot{
			Package:      pkg,
			TotalTokens:  acc.tokens,
			TotalLOC:     acc.loc,
			SessionCount: len(acc.sessions),
			Cost:         acc.cost,
		})
	}
	// Sort hotspots by token cost descending.
	sortPackageHotspots(result.PackageHotspots)

	return result
}

// commitBelongsToSession returns true if a commit timestamp falls within
// a session's active window (created..modified + buffer).
func commitBelongsToSession(commitTime, sessionCreated, sessionModified time.Time) bool {
	if sessionCreated.IsZero() || sessionModified.IsZero() {
		return false
	}
	end := sessionModified.Add(correlationBuffer)
	return !commitTime.Before(sessionCreated) && !commitTime.After(end)
}

// packageFromPath extracts the package directory from a file path.
// Examples: "internal/analyzer/foo.go" → "internal/analyzer",
//
//	"cmd/main.go" → "cmd"
func packageFromPath(path string) string {
	// Walk path to find last directory component before the file.
	lastSlash := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			lastSlash = i
			break
		}
	}
	if lastSlash <= 0 {
		return ""
	}
	return path[:lastSlash]
}

// topEfficient returns sessions with highest LOC per token, that have commits.
func topEfficient(sessions []SessionEfficiency, n int) []SessionEfficiency {
	var withLOC []SessionEfficiency
	for _, s := range sessions {
		if s.LOCAdded > 0 && s.CommitCount > 0 {
			withLOC = append(withLOC, s)
		}
	}
	sortByEfficiency(withLOC)
	if len(withLOC) > n {
		withLOC = withLOC[:n]
	}
	return withLOC
}

// lowYield returns sessions with lowest efficiency that still have commits.
func lowYield(sessions []SessionEfficiency, n int) []SessionEfficiency {
	var withLOC []SessionEfficiency
	for _, s := range sessions {
		if s.CommitCount > 0 && s.LOCAdded > 0 && s.TotalTokens > 50000 {
			withLOC = append(withLOC, s)
		}
	}
	// Sort ascending by efficiency (lowest first).
	for i := 0; i < len(withLOC); i++ {
		for j := i + 1; j < len(withLOC); j++ {
			if withLOC[j].Efficiency < withLOC[i].Efficiency {
				withLOC[i], withLOC[j] = withLOC[j], withLOC[i]
			}
		}
	}
	if len(withLOC) > n {
		withLOC = withLOC[:n]
	}
	return withLOC
}

// reasoningSinks returns sessions with most tokens that have no commits.
func reasoningSinks(sessions []SessionEfficiency, n int) []SessionEfficiency {
	var sinks []SessionEfficiency
	for _, s := range sessions {
		if s.CommitCount == 0 && s.TotalTokens > 10000 {
			sinks = append(sinks, s)
		}
	}
	// Sort descending by tokens.
	for i := 0; i < len(sinks); i++ {
		for j := i + 1; j < len(sinks); j++ {
			if sinks[j].TotalTokens > sinks[i].TotalTokens {
				sinks[i], sinks[j] = sinks[j], sinks[i]
			}
		}
	}
	if len(sinks) > n {
		sinks = sinks[:n]
	}
	return sinks
}

// sortByEfficiency sorts sessions by efficiency descending.
func sortByEfficiency(sessions []SessionEfficiency) {
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].Efficiency > sessions[i].Efficiency {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}
}

// sortPackageHotspots sorts by total tokens descending.
func sortPackageHotspots(hotspots []PackageHotspot) {
	for i := 0; i < len(hotspots); i++ {
		for j := i + 1; j < len(hotspots); j++ {
			if hotspots[j].TotalTokens > hotspots[i].TotalTokens {
				hotspots[i], hotspots[j] = hotspots[j], hotspots[i]
			}
		}
	}
}
