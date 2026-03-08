package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	bondPath string
	bondCmd  = &cobra.Command{
		Use:   "bond",
		Short: "Show cross-project bond relationship",
		Long: `Reads and displays docs/bond.md — the cross-project relationship file.
Shows identity, data flow, shared metrics, and debugger mapping between bonded projects.`,
		RunE: runBond,
	}
	bondVerifyCmd = &cobra.Command{
		Use:   "verify",
		Short: "Verify sibling repo has matching bond file",
		RunE:  runBondVerify,
	}
)

// BondFile represents a parsed bond document.
type BondFile struct {
	Title    string    `json:"title"`
	Identity []BondRow `json:"identity"`
	DataFlow string    `json:"data_flow"`
	Metrics  []BondRow `json:"metrics"`
	Debugger []BondRow `json:"debugger"`
	Lineage  string    `json:"lineage,omitempty"`
	Philos   []string  `json:"philosophy,omitempty"`
}

// BondRow is a generic markdown table row.
type BondRow struct {
	Cells []string `json:"cells"`
}

func init() {
	bondCmd.Flags().StringVar(&bondPath, "path", "", "Override bond file location")
	bondCmd.AddCommand(bondVerifyCmd)
	rootCmd.AddCommand(bondCmd)
}

func runBond(_ *cobra.Command, _ []string) error {
	path := resolveBondPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read bond file: %w (expected at %s)", err, path)
	}

	bf := parseBondFile(string(data))

	if isJSON() {
		return printJSON(bf)
	}

	// Compact text output
	fmt.Println(bf.Title)

	if len(bf.Identity) > 1 {
		// Identity table: first row is headers, rest are data
		headers := bf.Identity[0].Cells
		for _, row := range bf.Identity[1:] {
			if len(row.Cells) < len(headers) {
				continue
			}
			// Field  ProjectA  ProjectB
			field := row.Cells[0]
			vals := append([]string{}, row.Cells[1:]...)
			fmt.Printf("  %-10s %s\n", field, strings.Join(vals, "  /  "))
		}
	}

	if bf.DataFlow != "" {
		fmt.Printf("  Data flow: %s\n", strings.TrimSpace(bf.DataFlow))
	}

	if len(bf.Metrics) > 1 {
		names := make([]string, 0)
		for _, row := range bf.Metrics[1:] {
			if len(row.Cells) >= 2 {
				names = append(names, row.Cells[0])
			}
		}
		if len(names) > 0 {
			fmt.Printf("  Shared: %s\n", strings.Join(names, ", "))
		}
	}

	return nil
}

func runBondVerify(_ *cobra.Command, _ []string) error {
	path := resolveBondPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read bond file: %w (expected at %s)", err, path)
	}

	bf := parseBondFile(string(data))

	// Extract sibling repo names from identity table
	if len(bf.Identity) < 2 {
		return fmt.Errorf("bond file has no identity data")
	}

	headers := bf.Identity[0].Cells
	if len(headers) < 3 {
		return fmt.Errorf("identity table needs at least 3 columns")
	}

	// Find Repo row
	cwd, _ := os.Getwd()
	thisProject := filepath.Base(cwd)
	siblingName := ""

	for _, row := range bf.Identity[1:] {
		if len(row.Cells) < 3 && len(row.Cells) > 0 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(row.Cells[0]), "repo") {
			// Find the column that is NOT our project
			for i := 1; i < len(row.Cells); i++ {
				repo := strings.TrimSpace(row.Cells[i])
				repoName := filepath.Base(repo)
				if !strings.EqualFold(repoName, thisProject) {
					siblingName = repoName
					break
				}
			}
			break
		}
	}

	if siblingName == "" {
		return fmt.Errorf("could not identify sibling repo from identity table")
	}

	// Check sibling path
	parentDir := filepath.Dir(cwd)
	siblingBond := filepath.Join(parentDir, siblingName, "docs", "bond.md")

	type verifyResult struct {
		Sibling  string `json:"sibling"`
		BondPath string `json:"bond_path"`
		Exists   bool   `json:"exists"`
		ThisRepo string `json:"this_repo"`
	}

	result := verifyResult{
		Sibling:  siblingName,
		BondPath: siblingBond,
		ThisRepo: thisProject,
	}

	if _, err := os.Stat(siblingBond); err == nil {
		result.Exists = true
	}

	if isJSON() {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if result.Exists {
		fmt.Printf("Bond verified: %s ↔ %s\n", thisProject, siblingName)
		fmt.Printf("  Sibling bond: %s\n", siblingBond)
	} else {
		fmt.Printf("Bond incomplete: sibling %s has no bond file\n", siblingName)
		fmt.Printf("  Expected: %s\n", siblingBond)
	}

	return nil
}

func resolveBondPath() string {
	if bondPath != "" {
		return bondPath
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "docs", "bond.md")
}

// parseBondFile parses a bond markdown file into sections.
func parseBondFile(content string) *BondFile {
	bf := &BondFile{}
	lines := strings.Split(content, "\n")

	// Extract title from first # line
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			bf.Title = strings.TrimPrefix(line, "# ")
			break
		}
	}

	// Split into sections by ## headers
	sections := make(map[string][]string)
	var currentSection string
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			currentSection = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			sections[currentSection] = nil
			continue
		}
		if currentSection != "" {
			sections[currentSection] = append(sections[currentSection], line)
		}
	}

	// Parse Identity table
	if sec, ok := sections["Identity"]; ok {
		bf.Identity = parseMarkdownTable(sec)
	}

	// Parse Data flow — extract from code block
	if sec, ok := sections["Data flow"]; ok {
		inBlock := false
		var flowLines []string
		for _, line := range sec {
			if strings.HasPrefix(line, "```") {
				if inBlock {
					break
				}
				inBlock = true
				continue
			}
			if inBlock {
				flowLines = append(flowLines, line)
			}
		}
		bf.DataFlow = strings.TrimSpace(strings.Join(flowLines, "\n"))
	}

	// Parse Shared metrics table
	if sec, ok := sections["Shared metrics"]; ok {
		bf.Metrics = parseMarkdownTable(sec)
	}

	// Parse Debugger mapping table
	if sec, ok := sections["Debugger mapping"]; ok {
		bf.Debugger = parseMarkdownTable(sec)
	}

	// Parse Lineage — free text
	if sec, ok := sections["Lineage"]; ok {
		bf.Lineage = strings.TrimSpace(strings.Join(sec, "\n"))
	}

	// Parse Philosophy — bullet list
	if sec, ok := sections["Philosophy (shared)"]; ok {
		for _, line := range sec {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				bf.Philos = append(bf.Philos, strings.TrimPrefix(trimmed, "- "))
			}
		}
	}

	return bf
}

// parseMarkdownTable parses lines into rows of cells.
// Skips the separator row (---|---).
func parseMarkdownTable(lines []string) []BondRow {
	var rows []BondRow
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.Contains(trimmed, "|") {
			continue
		}
		// Skip separator rows
		cleaned := strings.ReplaceAll(trimmed, "|", "")
		cleaned = strings.ReplaceAll(cleaned, "-", "")
		cleaned = strings.ReplaceAll(cleaned, ":", "")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" {
			continue
		}

		// Split by |, trim leading/trailing empty cells
		parts := strings.Split(trimmed, "|")
		var cells []string
		for _, p := range parts {
			cell := strings.TrimSpace(p)
			cells = append(cells, cell)
		}
		// Remove leading/trailing empty cells from | boundaries
		if len(cells) > 0 && cells[0] == "" {
			cells = cells[1:]
		}
		if len(cells) > 0 && cells[len(cells)-1] == "" {
			cells = cells[:len(cells)-1]
		}
		if len(cells) > 0 {
			rows = append(rows, BondRow{Cells: cells})
		}
	}
	return rows
}
