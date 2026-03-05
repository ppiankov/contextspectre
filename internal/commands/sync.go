package commands

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var (
	syncTarget  string
	syncVector  bool
	syncProject string
	syncCWD     bool
	syncApply   bool
	syncDryRun  bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [distill-output.md]",
	Short: "Merge distilled/vector decisions into CLAUDE.md",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSync,
}

type syncSection struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

type syncAction struct {
	Heading string `json:"heading"`
	Action  string `json:"action"`
	Source  string `json:"source"`
	Diff    string `json:"diff,omitempty"`
}

type syncOutput struct {
	TargetPath string       `json:"target_path"`
	SourcePath string       `json:"source_path"`
	Apply      bool         `json:"apply"`
	DryRun     bool         `json:"dry_run"`
	Actions    []syncAction `json:"actions"`
}

func runSync(cmd *cobra.Command, args []string) error {
	if syncApply {
		syncDryRun = false
	}
	if !syncApply && !syncDryRun {
		return fmt.Errorf("refusing to write without --apply")
	}

	sourcePath, cleanup, err := resolveSyncSource(args)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	sourceSections, _, err := editor.ParseMarkdownSections(sourcePath)
	if err != nil {
		return fmt.Errorf("parse source sections: %w", err)
	}
	if len(sourceSections) == 0 {
		return fmt.Errorf("no sections found in source")
	}

	targetExists := true
	if _, err := os.Stat(syncTarget); err != nil {
		if os.IsNotExist(err) {
			targetExists = false
		} else {
			return fmt.Errorf("stat target: %w", err)
		}
	}

	targetSections := []editor.UniteSection{}
	preamble := "# CLAUDE.md\n\n"
	if targetExists {
		targetSections, _, err = editor.ParseMarkdownSections(syncTarget)
		if err != nil {
			return fmt.Errorf("parse target sections: %w", err)
		}
		preamble, err = readSyncPreamble(syncTarget)
		if err != nil {
			return err
		}
	}

	mergedSections, actions, err := mergeSyncSections(targetSections, sourceSections, syncApply, !isJSON())
	if err != nil {
		return err
	}

	rendered := renderSyncDocument(preamble, mergedSections)

	if isJSON() {
		out := syncOutput{
			TargetPath: syncTarget,
			SourcePath: sourcePath,
			Apply:      syncApply,
			DryRun:     syncDryRun,
			Actions:    actions,
		}
		if syncApply {
			if err := writeFileAtomic(syncTarget, []byte(rendered), 0o644); err != nil {
				return err
			}
		}
		return printJSON(out)
	}

	for _, a := range actions {
		fmt.Printf("[%s] %s (%s)\n", strings.ToUpper(a.Action), a.Heading, a.Source)
		if strings.TrimSpace(a.Diff) != "" {
			fmt.Println(a.Diff)
		}
	}

	if syncDryRun {
		fmt.Println("\nDry run complete. Use --apply to write changes.")
		return nil
	}

	if err := writeFileAtomic(syncTarget, []byte(rendered), 0o644); err != nil {
		return err
	}
	fmt.Printf("Synced %d section(s) into %s\n", len(actions), syncTarget)
	return nil
}

func resolveSyncSource(args []string) (string, func(), error) {
	if syncVector {
		if len(args) > 0 {
			return "", nil, fmt.Errorf("do not provide source file with --vector")
		}
		return buildSyncVectorSource()
	}
	if len(args) != 1 {
		return "", nil, fmt.Errorf("provide source markdown file or use --vector")
	}
	if _, err := os.Stat(args[0]); err != nil {
		return "", nil, fmt.Errorf("source not found: %s", args[0])
	}
	return args[0], nil, nil
}

func buildSyncVectorSource() (string, func(), error) {
	if syncProject == "" && !syncCWD {
		return "", nil, fmt.Errorf("--vector requires --project or --cwd")
	}

	dir := resolveClaudeDir()
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		return "", nil, fmt.Errorf("list sessions: %w", err)
	}

	filtered := filterDistillSessions(sessions, syncProject, syncCWD)
	if len(filtered) == 0 {
		return "", nil, fmt.Errorf("no matching sessions found")
	}

	projectName := filtered[0].ProjectName
	if projectName == "" {
		projectName = "unknown"
	}

	var inputs []analyzer.TopicSessionInput
	var markerInputs []editor.VectorMarkerInput
	for _, si := range filtered {
		entries, err := jsonl.Parse(si.FullPath)
		if err != nil {
			continue
		}
		inputs = append(inputs, analyzer.TopicSessionInput{
			Entries: entries,
			Info: analyzer.SessionInfoLite{
				SessionID: si.SessionID,
				Slug:      si.Slug,
				Created:   si.Created,
				Modified:  si.Modified,
			},
		})
		mf, err := editor.LoadMarkers(si.FullPath)
		if err == nil && len(mf.CommitPoints) > 0 {
			markerInputs = append(markerInputs, editor.VectorMarkerInput{
				SessionLabel: si.DisplayName(),
				Markers:      mf,
			})
		}
	}
	ts := analyzer.CollectTopics(inputs)
	ts.ProjectName = projectName
	snap := editor.CollectVector(ts, markerInputs)

	tmpFile, err := os.CreateTemp("", "contextspectre-sync-vector-*.md")
	if err != nil {
		return "", nil, err
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	if err := editor.RenderVector(snap, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", nil, err
	}
	return tmpPath, func() { _ = os.Remove(tmpPath) }, nil
}

func mergeSyncSections(target, source []editor.UniteSection, apply, interactive bool) ([]syncSection, []syncAction, error) {
	targetOrder := make([]string, 0, len(target))
	targetMap := make(map[string]string, len(target))
	for _, s := range target {
		targetOrder = append(targetOrder, s.Heading)
		targetMap[s.Heading] = renderSyncSectionBody(s)
	}

	targetHeadings := append([]string(nil), targetOrder...)
	actions := make([]syncAction, 0, len(source))
	var appended []syncSection
	reader := bufio.NewReader(os.Stdin)

	for _, src := range source {
		srcBody := renderSyncSectionBody(src)
		match, matched := findSyncHeadingMatch(src.Heading, targetHeadings)
		if !matched {
			appended = append(appended, syncSection{Heading: src.Heading, Body: srcBody})
			actions = append(actions, syncAction{
				Heading: src.Heading,
				Action:  "append",
				Source:  "new",
				Diff:    renderSyncDiff("", srcBody),
			})
			continue
		}

		oldBody := targetMap[match]
		action := "replace"
		if apply && interactive {
			fmt.Printf("\nSection: %s\n", match)
			fmt.Println(renderSyncDiff(oldBody, srcBody))
			fmt.Print("Choose action [r]eplace/[a]ppend/[s]kip (default: r): ")
			choice, _ := reader.ReadString('\n')
			switch strings.ToLower(strings.TrimSpace(choice)) {
			case "a", "append":
				action = "append"
			case "s", "skip":
				action = "skip"
			default:
				action = "replace"
			}
		}

		switch action {
		case "replace":
			targetMap[match] = srcBody
		case "append":
			combined := strings.TrimSpace(oldBody)
			if combined != "" {
				combined += "\n\n"
			}
			combined += strings.TrimSpace(srcBody)
			targetMap[match] = combined
		case "skip":
			// keep existing target section
		}

		actions = append(actions, syncAction{
			Heading: match,
			Action:  action,
			Source:  "matched",
			Diff:    renderSyncDiff(oldBody, srcBody),
		})
	}

	merged := make([]syncSection, 0, len(targetOrder)+len(appended))
	for _, heading := range targetOrder {
		merged = append(merged, syncSection{
			Heading: heading,
			Body:    targetMap[heading],
		})
	}
	merged = append(merged, appended...)

	return merged, actions, nil
}

func renderSyncSectionBody(s editor.UniteSection) string {
	var b strings.Builder
	for _, ml := range s.MetadataLines {
		b.WriteString(ml)
		b.WriteByte('\n')
	}
	content := strings.TrimRight(s.Content, "\n")
	if content != "" {
		if len(s.MetadataLines) > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(content)
	}
	return strings.TrimSpace(b.String())
}

func findSyncHeadingMatch(source string, targets []string) (string, bool) {
	for _, t := range targets {
		if t == source {
			return t, true
		}
	}
	normalizedSource := normalizeSyncHeading(source)
	for _, t := range targets {
		if normalizeSyncHeading(t) == normalizedSource {
			return t, true
		}
	}
	return "", false
}

func normalizeSyncHeading(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune(' ')
		}
	}
	fields := strings.Fields(b.String())
	return strings.Join(fields, " ")
}

func renderSyncDiff(oldBody, newBody string) string {
	oldLines := splitSyncLines(oldBody)
	newLines := splitSyncLines(newBody)

	if strings.TrimSpace(oldBody) == strings.TrimSpace(newBody) {
		return "  (no changes)"
	}

	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}
	var out []string
	for i := 0; i < maxLen; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		if oldLine == newLine {
			continue
		}
		if oldLine != "" {
			out = append(out, "- "+oldLine)
		}
		if newLine != "" {
			out = append(out, "+ "+newLine)
		}
		if len(out) >= 12 {
			out = append(out, "... (diff truncated)")
			break
		}
	}
	if len(out) == 0 {
		return "  (content reordered)"
	}
	return strings.Join(out, "\n")
}

func splitSyncLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	raw := strings.Split(strings.TrimSpace(s), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		out = append(out, strings.TrimRight(line, " \t"))
	}
	return out
}

func readSyncPreamble(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read target: %w", err)
	}
	text := string(data)
	lines := strings.Split(text, "\n")
	var pre []string
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			break
		}
		pre = append(pre, line)
	}
	preamble := strings.TrimRight(strings.Join(pre, "\n"), "\n")
	if strings.TrimSpace(preamble) == "" {
		preamble = "# CLAUDE.md"
	}
	return preamble + "\n\n", nil
}

func renderSyncDocument(preamble string, sections []syncSection) string {
	var b strings.Builder
	if strings.TrimSpace(preamble) == "" {
		preamble = "# CLAUDE.md\n\n"
	}
	b.WriteString(strings.TrimRight(preamble, "\n"))
	b.WriteString("\n\n")
	for i, s := range sections {
		b.WriteString("## ")
		b.WriteString(s.Heading)
		b.WriteString("\n\n")
		body := strings.TrimSpace(s.Body)
		if body != "" {
			b.WriteString(body)
			b.WriteString("\n")
		}
		if i < len(sections)-1 {
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmpName := fmt.Sprintf(".%s.tmp-%d", filepath.Base(path), time.Now().UnixNano())
	tmpPath := filepath.Join(dir, tmpName)
	if err := os.WriteFile(tmpPath, data, mode); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func init() {
	syncCmd.Flags().StringVar(&syncTarget, "target", "CLAUDE.md", "Target markdown file")
	syncCmd.Flags().BoolVar(&syncVector, "vector", false, "Use vector extraction as source")
	syncCmd.Flags().StringVar(&syncProject, "project", "", "Project alias/name for --vector mode")
	syncCmd.Flags().BoolVar(&syncCWD, "cwd", false, "Use current working directory sessions for --vector mode")
	syncCmd.Flags().BoolVar(&syncApply, "apply", false, "Write merged changes to target")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", true, "Preview merge without writing")
	rootCmd.AddCommand(syncCmd)
}
