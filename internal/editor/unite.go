package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// UniteSection represents a single ## section parsed from a markdown file.
type UniteSection struct {
	SourceFile    string
	SourceModTime int64
	Heading       string
	Summary       string
	MetadataLines []string
	Content       string
	TokenEstimate int
}

// UniteOpts controls the unite operation.
type UniteOpts struct {
	OutputPath  string
	ProjectName string
}

// UniteResult holds the output of a unite operation.
type UniteResult struct {
	SectionsIncluded int
	SourceFileCount  int
	TotalTokens      int
	OutputPath       string
}

// ParseMarkdownSections parses a markdown file into sections split on "## " headings.
// Returns sections, project name extracted from # heading, and any error.
func ParseMarkdownSections(path string) ([]UniteSection, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}

	fi, _ := os.Stat(path)
	modTime := int64(0)
	if fi != nil {
		modTime = fi.ModTime().Unix()
	}

	baseName := filepath.Base(path)
	lines := strings.Split(string(data), "\n")

	projectName := ""
	var sections []UniteSection
	var current *UniteSection

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") && projectName == "" {
			projectName = extractProjectFromH1(line)
		}

		if strings.HasPrefix(line, "## ") {
			if current != nil {
				current.TokenEstimate = estimateSectionTokens(current)
				sections = append(sections, *current)
			}
			heading := strings.TrimPrefix(line, "## ")
			heading = strings.TrimSpace(heading)
			current = &UniteSection{
				SourceFile:    baseName,
				SourceModTime: modTime,
				Heading:       heading,
				Summary:       cleanSummary(heading),
			}
			continue
		}

		if current == nil {
			continue
		}

		if strings.TrimSpace(line) == "---" {
			continue
		}

		if isMetadataLine(line) {
			current.MetadataLines = append(current.MetadataLines, line)
			continue
		}

		current.Content += line + "\n"
	}

	if current != nil {
		current.TokenEstimate = estimateSectionTokens(current)
		sections = append(sections, *current)
	}

	return sections, projectName, nil
}

// UniteSections merges selected sections into a single markdown file.
func UniteSections(sections []UniteSection, selectedIndices []int, opts UniteOpts) (*UniteResult, error) {
	if len(selectedIndices) == 0 {
		return nil, fmt.Errorf("no sections selected")
	}

	for _, idx := range selectedIndices {
		if idx < 0 || idx >= len(sections) {
			return nil, fmt.Errorf("section index %d out of range (0-%d)", idx, len(sections)-1)
		}
	}

	projectName := opts.ProjectName
	if projectName == "" {
		projectName = "Project"
	}

	totalTokens := 0
	sourceFiles := make(map[string]bool)
	for _, idx := range selectedIndices {
		s := sections[idx]
		totalTokens += s.TokenEstimate
		sourceFiles[s.SourceFile] = true
	}

	result := &UniteResult{
		SectionsIncluded: len(selectedIndices),
		SourceFileCount:  len(sourceFiles),
		TotalTokens:      totalTokens,
		OutputPath:       opts.OutputPath,
	}

	var b strings.Builder

	fmt.Fprintf(&b, "# United Context — %s\n\n", projectName)
	fmt.Fprintf(&b, "United %s from %d files (%d sections selected)\n\n",
		time.Now().Format("2006-01-02 15:04"),
		len(sourceFiles), len(selectedIndices))

	b.WriteString("| Metric | Value |\n")
	b.WriteString("|--------|-------|\n")
	fmt.Fprintf(&b, "| Sections | %d |\n", len(selectedIndices))
	fmt.Fprintf(&b, "| Source files | %d |\n", len(sourceFiles))
	fmt.Fprintf(&b, "| Tokens | ~%s |\n", formatTokensCompact(totalTokens))
	b.WriteString("\n---\n\n")

	for _, idx := range selectedIndices {
		s := sections[idx]
		fmt.Fprintf(&b, "## %s\n\n", s.Heading)
		fmt.Fprintf(&b, "*Source: %s*\n\n", s.SourceFile)

		for _, ml := range s.MetadataLines {
			b.WriteString(ml)
			b.WriteString("\n")
		}
		if len(s.MetadataLines) > 0 {
			b.WriteString("\n")
		}

		content := strings.TrimRight(s.Content, "\n")
		if content != "" {
			b.WriteString(content)
			b.WriteString("\n")
		}
		b.WriteString("\n---\n\n")
	}

	if err := os.WriteFile(opts.OutputPath, []byte(b.String()), 0o644); err != nil {
		return nil, fmt.Errorf("write unite output: %w", err)
	}

	return result, nil
}

// DeduplicateSections returns a map of index → duplicate-of-index for sections
// with identical headings. The earlier occurrence wins.
func DeduplicateSections(sections []UniteSection) map[int]int {
	seen := make(map[string]int)
	dupes := make(map[int]int)
	for i, s := range sections {
		if first, exists := seen[s.Heading]; exists {
			dupes[i] = first
		} else {
			seen[s.Heading] = i
		}
	}
	return dupes
}

func extractProjectFromH1(line string) string {
	line = strings.TrimPrefix(line, "# ")
	for _, sep := range []string{" — ", " -- ", " - "} {
		if idx := strings.Index(line, sep); idx >= 0 {
			return strings.TrimSpace(line[idx+len(sep):])
		}
	}
	return strings.TrimSpace(line)
}

func isMetadataLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "- **") && strings.Contains(trimmed, ":**")
}

var sectionPrefixRe = regexp.MustCompile(`^(?:Topic \d+|Branch #\d+|Session History):\s*`)

func cleanSummary(heading string) string {
	return sectionPrefixRe.ReplaceAllString(heading, "")
}

func estimateSectionTokens(s *UniteSection) int {
	total := len(s.Heading)
	for _, ml := range s.MetadataLines {
		total += len(ml)
	}
	total += len(s.Content)
	return total / 4
}
