package analyzer

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

// TangentGroup represents a contiguous block of entries referencing external repos.
type TangentGroup struct {
	StartIndex        int      // first entry in the tangent
	EndIndex          int      // last entry in the tangent (inclusive)
	EntryIndices      []int    // all entry indices in this tangent
	ExternalPaths     []string // unique external paths referenced
	EstimatedTokens   int      // total tokens across tangent entries
	MixedScopeEntries int      // entries referencing both CWD and external paths
}

// TangentResult summarizes all detected cross-repo tangents in a session.
type TangentResult struct {
	Groups       []TangentGroup
	TotalEntries int
	TotalTokens  int
	ExternalDirs int // unique external root directories
	SessionCWD   string
}

// AllTangentIndices returns every entry index across all tangent groups.
func (r *TangentResult) AllTangentIndices() map[int]bool {
	m := make(map[int]bool)
	for _, g := range r.Groups {
		for _, idx := range g.EntryIndices {
			m[idx] = true
		}
	}
	return m
}

// entryInfo holds path-analysis metadata for a single entry.
type entryInfo struct {
	externalPaths     []string // paths outside CWD
	modifiesCWD       bool     // writes/edits files inside CWD
	refsExternal      bool     // references any external path
	refsCWD           bool     // references any CWD path
	cwdPathCount      int      // number of CWD path refs
	externalPathCount int      // number of external path refs
}

// FindTangents detects cross-repo tangent sequences in a session.
// A tangent is a contiguous block of entries where tool_use inputs reference
// paths outside the session's CWD AND no file modifications occur in the CWD.
// Mixed-scope entries (referencing both CWD and external) are included at
// proportional token cost rather than terminating the tangent.
func FindTangents(entries []jsonl.Entry) *TangentResult {
	result := &TangentResult{}

	initialCWD := DetectSessionCWD(entries)
	if initialCWD == "" {
		return result
	}
	result.SessionCWD = initialCWD

	// Build dynamic CWD map
	cwds := buildActiveCWDMap(entries, initialCWD)

	infos := make([]entryInfo, len(entries))
	for i, e := range entries {
		cwd := cwds[i]
		paths, tools := extractAllPaths(e)
		for j, p := range paths {
			if isOutsideCWD(p, cwd) {
				infos[i].externalPaths = append(infos[i].externalPaths, p)
				infos[i].refsExternal = true
				infos[i].externalPathCount++
			} else {
				infos[i].refsCWD = true
				infos[i].cwdPathCount++
			}
			// Check if this is a file-modifying tool
			if j < len(tools) && isModifyingTool(tools[j]) && !isOutsideCWD(p, cwd) {
				infos[i].modifiesCWD = true
			}
		}
	}

	// Find contiguous blocks where entries reference external paths
	i := 0
	for i < len(entries) {
		info := infos[i]
		// Start of tangent requires purely external entry (not mixed, not CWD-only)
		if !info.refsExternal || info.modifiesCWD {
			i++
			continue
		}
		if info.refsCWD {
			// Mixed at start — don't start tangent here
			i++
			continue
		}

		// Found start of potential tangent
		start := i
		externalPathSet := make(map[string]bool)
		totalTokens := 0
		mixedCount := 0

		// Expand forward: include entries that are part of this tangent block
		for i < len(entries) {
			e := entries[i]
			info := infos[i]

			// CWD modification always terminates
			if info.modifiesCWD {
				break
			}

			// Pure CWD reference (no external) terminates
			if info.refsCWD && !info.refsExternal {
				break
			}

			// Mixed scope: include at proportional token cost
			if info.refsCWD && info.refsExternal {
				for _, p := range info.externalPaths {
					externalPathSet[p] = true
				}
				total := info.cwdPathCount + info.externalPathCount
				ratio := float64(info.externalPathCount) / float64(total)
				totalTokens += int(float64(e.RawSize/4) * ratio)
				mixedCount++
				i++
				continue
			}

			// Include if entry references external paths
			if info.refsExternal {
				for _, p := range info.externalPaths {
					externalPathSet[p] = true
				}
				totalTokens += e.RawSize / 4
				i++
				continue
			}

			// Include non-conversational entries (progress, snapshots) between tangent entries
			if !e.IsConversational() {
				totalTokens += e.RawSize / 4
				i++
				continue
			}

			// Conversational entry with no path refs: could be part of the tangent
			if isResponseToTangent(entries, infos, i, start) {
				totalTokens += e.RawSize / 4
				i++
				continue
			}

			break
		}

		end := i - 1
		if end < start {
			continue
		}

		// Build the group
		var indices []int
		for j := start; j <= end; j++ {
			indices = append(indices, j)
		}

		// Only flag if there are at least 2 entries and enough tokens to matter
		if len(indices) >= 2 && totalTokens >= minTangentTokens {
			var uniquePaths []string
			for p := range externalPathSet {
				uniquePaths = append(uniquePaths, p)
			}

			group := TangentGroup{
				StartIndex:        start,
				EndIndex:          end,
				EntryIndices:      indices,
				ExternalPaths:     uniquePaths,
				EstimatedTokens:   totalTokens,
				MixedScopeEntries: mixedCount,
			}

			result.Groups = append(result.Groups, group)
			result.TotalEntries += len(indices)
			result.TotalTokens += totalTokens
		}
	}

	// Count unique external root directories using per-entry CWDs
	extDirs := make(map[string]bool)
	for _, g := range result.Groups {
		for _, p := range g.ExternalPaths {
			// Use initial CWD for root dir computation (stable reference)
			extDirs[externalRootDir(p, initialCWD)] = true
		}
	}
	result.ExternalDirs = len(extDirs)

	return result
}

// DetectSessionCWD extracts the CWD from the first entry that has one set.
func DetectSessionCWD(entries []jsonl.Entry) string {
	for _, e := range entries {
		if e.CWD != "" {
			return e.CWD
		}
	}
	return ""
}

// extractAllPaths extracts file paths and corresponding tool names from an entry's tool_use/tool_result blocks.
func extractAllPaths(e jsonl.Entry) (paths []string, toolNames []string) {
	if e.Message == nil {
		return nil, nil
	}
	blocks, err := jsonl.ParseContentBlocks(e.Message.Content)
	if err != nil {
		return nil, nil
	}

	for _, b := range blocks {
		switch b.Type {
		case "tool_use":
			p := extractToolPath(b.Input)
			if p != "" {
				paths = append(paths, p)
				toolNames = append(toolNames, b.Name)
			}
		case "tool_result":
			// Tool results can contain path references in their content
			// but we primarily care about tool_use inputs for path detection
		}
	}

	// Also check for Bash commands referencing paths
	for _, b := range blocks {
		if b.Type == "tool_use" && isBashLikeTool(b.Name) {
			cmdPaths := extractBashCommandPaths(b.Input)
			for _, p := range cmdPaths {
				paths = append(paths, p)
				toolNames = append(toolNames, b.Name)
			}
		}
	}

	return paths, toolNames
}

// extractToolPath extracts a file path from a tool_use input.
func extractToolPath(input json.RawMessage) string {
	var fields struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return ""
	}
	if fields.FilePath != "" {
		return fields.FilePath
	}
	return fields.Path
}

// extractBashCommandPaths extracts absolute paths from Bash command inputs.
func extractBashCommandPaths(input json.RawMessage) []string {
	var fields struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &fields); err != nil {
		return nil
	}

	var paths []string
	// Split on whitespace and look for absolute paths
	for _, word := range strings.Fields(fields.Command) {
		// Clean quotes
		word = strings.Trim(word, `"'`)
		if strings.HasPrefix(word, "/") && !isSystemPath(word) {
			paths = append(paths, word)
		}
	}
	return paths
}

// isOutsideCWD checks if a path is outside the session's CWD.
func isOutsideCWD(path, cwd string) bool {
	if path == "" || cwd == "" {
		return false
	}
	// Normalize paths
	absPath := filepath.Clean(path)
	absCWD := filepath.Clean(cwd)

	// Check if path is under CWD
	rel, err := filepath.Rel(absCWD, absPath)
	if err != nil {
		return true // can't determine relationship — treat as external
	}
	// If the relative path starts with "..", it's outside CWD
	return strings.HasPrefix(rel, "..")
}

// isModifyingTool returns true for tool names that modify files.
func isModifyingTool(name string) bool {
	switch name {
	case "Write", "Edit", "write_file", "edit_file", "WriteFile",
		"Bash", "bash", "execute_command", "run_command",
		"NotebookEdit":
		return true
	}
	return false
}

// isBashLikeTool returns true for Bash/shell tool names.
func isBashLikeTool(name string) bool {
	switch name {
	case "Bash", "bash", "execute_command", "run_command":
		return true
	}
	return false
}

// isSystemPath returns true for common system paths that shouldn't be treated as project paths.
func isSystemPath(path string) bool {
	prefixes := []string{"/usr/", "/bin/", "/sbin/", "/etc/", "/tmp/", "/var/", "/dev/", "/proc/", "/sys/"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// minTangentTokens is the minimum estimated tokens for a tangent group to be flagged.
// Groups below this threshold are too small to matter.
const minTangentTokens = 100

// isResponseToTangent checks if a conversational entry with no path refs
// is likely a response within a tangent (e.g., assistant answering about external repo).
// Requires the preceding conversational entry to have explicitly referenced external paths.
func isResponseToTangent(entries []jsonl.Entry, infos []entryInfo, idx, tangentStart int) bool {
	if idx <= tangentStart {
		return false
	}

	// Walk backward to find the nearest preceding conversational entry.
	// It must have explicitly referenced external paths to continue the tangent.
	for j := idx - 1; j >= tangentStart; j-- {
		if entries[j].IsConversational() {
			return infos[j].refsExternal
		}
	}

	return false
}

// externalRootDir extracts the project root directory from an external path.
// E.g., "/home/user/dev/other-repo/src/main.go" → "/home/user/dev/other-repo"
func externalRootDir(path, cwd string) string {
	// Find the common parent between cwd and path, then take the next component
	cwdParts := strings.Split(filepath.Clean(cwd), string(filepath.Separator))
	pathParts := strings.Split(filepath.Clean(path), string(filepath.Separator))

	// Find where they diverge
	common := 0
	for i := 0; i < len(cwdParts) && i < len(pathParts); i++ {
		if cwdParts[i] != pathParts[i] {
			break
		}
		common = i + 1
	}

	// Take one more component from the path after divergence
	if common < len(pathParts) {
		return strings.Join(pathParts[:common+1], string(filepath.Separator))
	}
	return path
}
