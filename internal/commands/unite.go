package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/spf13/cobra"
)

var (
	uniteScan   bool
	uniteAuto   bool
	uniteDryRun bool
	uniteBudget int
	uniteOutput string
)

var uniteCmd = &cobra.Command{
	Use:   "unite [file1.md file2.md ...]",
	Short: "Combine distilled and exported markdown files into a single context file",
	Long: `Merge multiple markdown files produced by distill or export into one
portable context file you can load into a new session.

Accepts explicit file paths as arguments, or use --scan to auto-discover
distilled-*.md and branch-export-*.md files in the current directory.

Examples:
  contextspectre unite file1.md file2.md
  contextspectre unite --scan --dry-run
  contextspectre unite --scan --auto
  contextspectre unite --scan --budget 50000
  contextspectre unite --scan --output united.md
  contextspectre unite --scan --format json`,
	RunE: runUnite,
}

func runUnite(cmd *cobra.Command, args []string) error {
	files, err := resolveUniteFiles(args)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		if isJSON() {
			return printJSON(UniteListJSON{Sections: []UniteSectionJSON{}})
		}
		fmt.Println("No input files found.")
		return nil
	}

	var allSections []editor.UniteSection
	projectName := ""
	for _, f := range files {
		sections, pName, err := editor.ParseMarkdownSections(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skip %s: %v\n", f, err)
			continue
		}
		if projectName == "" && pName != "" {
			projectName = pName
		}
		allSections = append(allSections, sections...)
	}

	if len(allSections) == 0 {
		if isJSON() {
			return printJSON(UniteListJSON{Sections: []UniteSectionJSON{}})
		}
		fmt.Println("No sections found in input files.")
		return nil
	}

	dupes := editor.DeduplicateSections(allSections)

	if uniteDryRun {
		if isJSON() {
			return printJSON(buildUniteListJSON(allSections, dupes))
		}
		printUniteSectionList(allSections, dupes)
		return nil
	}

	var selectedIndices []int
	if uniteBudget > 0 {
		selectedIndices = selectByBudget(allSections, dupes, uniteBudget)
	} else if uniteAuto || isJSON() {
		for i := range allSections {
			if _, isDupe := dupes[i]; !isDupe {
				selectedIndices = append(selectedIndices, i)
			}
		}
	} else {
		printUniteSectionList(allSections, dupes)
		fmt.Print("\nSelect sections (e.g. 0,2,4-7 or \"all\"): ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" || strings.EqualFold(input, "all") {
			for i := range allSections {
				if _, isDupe := dupes[i]; !isDupe {
					selectedIndices = append(selectedIndices, i)
				}
			}
		} else {
			selectedIndices, err = parseNumberRanges(input, len(allSections))
			if err != nil {
				return err
			}
		}
	}

	if len(selectedIndices) == 0 {
		fmt.Println("No sections selected.")
		return nil
	}

	if uniteOutput == "" {
		uniteOutput = fmt.Sprintf("united-%s-%s.md",
			sanitizeFilename(projectName),
			time.Now().Format("2006-01-02"))
	}

	result, err := editor.UniteSections(allSections, selectedIndices, editor.UniteOpts{
		OutputPath:  uniteOutput,
		ProjectName: projectName,
	})
	if err != nil {
		return fmt.Errorf("unite: %w", err)
	}

	if isJSON() {
		return printJSON(buildUniteOutputJSON(allSections, selectedIndices, result))
	}

	fmt.Printf("United %d sections from %d files to %s\n",
		result.SectionsIncluded, result.SourceFileCount, result.OutputPath)
	fmt.Printf("  Tokens: ~%s\n", formatTokens(result.TotalTokens))

	return nil
}

func resolveUniteFiles(args []string) ([]string, error) {
	if len(args) > 0 && uniteScan {
		return nil, fmt.Errorf("provide file arguments or --scan, not both")
	}

	if len(args) > 0 {
		for _, f := range args {
			if _, err := os.Stat(f); os.IsNotExist(err) {
				return nil, fmt.Errorf("file not found: %s", f)
			}
		}
		return args, nil
	}

	if !uniteScan {
		return nil, fmt.Errorf("provide file arguments or use --scan")
	}

	var files []string
	patterns := []string{"distilled-*.md", "branch-export-*.md"}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		files = append(files, matches...)
	}

	sort.Slice(files, func(i, j int) bool {
		fi, _ := os.Stat(files[i])
		fj, _ := os.Stat(files[j])
		if fi == nil || fj == nil {
			return false
		}
		return fi.ModTime().After(fj.ModTime())
	})

	return files, nil
}

func selectByBudget(sections []editor.UniteSection, dupes map[int]int, budget int) []int {
	type indexed struct {
		idx     int
		modTime int64
	}
	var candidates []indexed
	for i, s := range sections {
		if _, isDupe := dupes[i]; isDupe {
			continue
		}
		candidates = append(candidates, indexed{idx: i, modTime: s.SourceModTime})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime > candidates[j].modTime
	})

	var selected []int
	remaining := budget
	for _, c := range candidates {
		est := sections[c.idx].TokenEstimate
		if est > remaining {
			continue
		}
		selected = append(selected, c.idx)
		remaining -= est
	}

	sort.Ints(selected)
	return selected
}

func printUniteSectionList(sections []editor.UniteSection, dupes map[int]int) {
	totalTokens := 0
	sourceFiles := make(map[string]bool)
	for _, s := range sections {
		totalTokens += s.TokenEstimate
		sourceFiles[s.SourceFile] = true
	}

	fmt.Printf("%d sections from %d files (~%s tokens)\n\n",
		len(sections), len(sourceFiles), formatTokens(totalTokens))

	for i, s := range sections {
		dupeMarker := ""
		if dupOf, isDupe := dupes[i]; isDupe {
			dupeMarker = fmt.Sprintf(" [dup of %d]", dupOf)
		}

		summary := s.Summary
		if len([]rune(summary)) > 40 {
			summary = string([]rune(summary)[:37]) + "..."
		}

		fmt.Printf("  %3d  %-30s  %7s  %s%s\n",
			i,
			s.SourceFile,
			formatTokens(s.TokenEstimate)+" tok",
			summary,
			dupeMarker)
	}

	if len(dupes) > 0 {
		fmt.Printf("\n  %d duplicate(s) detected\n", len(dupes))
	}
}

func buildUniteListJSON(sections []editor.UniteSection, dupes map[int]int) *UniteListJSON {
	out := &UniteListJSON{
		Total:      len(sections),
		Duplicates: len(dupes),
	}

	sourceFiles := make(map[string]bool)
	for i, s := range sections {
		sourceFiles[s.SourceFile] = true
		out.TotalTokens += s.TokenEstimate

		sj := UniteSectionJSON{
			Index:         i,
			SourceFile:    s.SourceFile,
			Heading:       s.Heading,
			Summary:       s.Summary,
			TokenEstimate: s.TokenEstimate,
		}
		if dupOf, isDupe := dupes[i]; isDupe {
			sj.IsDuplicate = true
			sj.DuplicateOf = dupOf
		}
		out.Sections = append(out.Sections, sj)
	}
	out.SourceFiles = len(sourceFiles)

	if out.Sections == nil {
		out.Sections = []UniteSectionJSON{}
	}
	return out
}

func buildUniteOutputJSON(sections []editor.UniteSection, selectedIndices []int, result *editor.UniteResult) *UniteOutputJSON {
	out := &UniteOutputJSON{
		SectionsIncluded: result.SectionsIncluded,
		SourceFiles:      result.SourceFileCount,
		TotalTokens:      result.TotalTokens,
		OutputPath:       result.OutputPath,
	}
	for _, idx := range selectedIndices {
		s := sections[idx]
		out.Sections = append(out.Sections, UniteSectionJSON{
			Index:         idx,
			SourceFile:    s.SourceFile,
			Heading:       s.Heading,
			Summary:       s.Summary,
			TokenEstimate: s.TokenEstimate,
		})
	}
	if out.Sections == nil {
		out.Sections = []UniteSectionJSON{}
	}
	return out
}

func init() {
	uniteCmd.Flags().BoolVar(&uniteScan, "scan", false, "Scan CWD for distilled-*.md and branch-export-*.md files")
	uniteCmd.Flags().BoolVar(&uniteAuto, "auto", false, "Select all sections (skip interactive prompt)")
	uniteCmd.Flags().BoolVar(&uniteDryRun, "dry-run", false, "Show section list without merging")
	uniteCmd.Flags().IntVar(&uniteBudget, "budget", 0, "Auto-select sections up to token budget (newest first)")
	uniteCmd.Flags().StringVar(&uniteOutput, "output", "", "Output file path (default: united-<project>-<date>.md)")
	rootCmd.AddCommand(uniteCmd)
}
