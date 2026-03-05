package commands

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/spf13/cobra"
)

var marksCWD bool

var marksCmd = &cobra.Command{
	Use:   "marks [session-id-or-path]",
	Short: "List all marks, bookmarks, and commit points for a session",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runMarks,
}

type markRow struct {
	UUID       string
	Type       string
	Label      string
	EntryIndex int
	LineNumber int
	Epoch      int
}

func runMarks(cmd *cobra.Command, args []string) error {
	path, err := resolveSessionArg(args, marksCWD)
	if err != nil {
		return err
	}

	entries, err := jsonl.Parse(path)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	markers, err := editor.LoadMarkers(path)
	if err != nil {
		return fmt.Errorf("load markers: %w", err)
	}

	stats := analyzer.Analyze(entries)
	indexByUUID := make(map[string]int, len(entries))
	lineByUUID := make(map[string]int, len(entries))
	for i, e := range entries {
		if e.UUID == "" {
			continue
		}
		indexByUUID[e.UUID] = i
		lineByUUID[e.UUID] = e.LineNumber
	}

	rows := []markRow{}
	for uuid, mt := range markers.Markers {
		rows = append(rows, makeMarkRow(uuid, string(mt), "", indexByUUID, lineByUUID, stats.Compactions))
	}
	for uuid, bm := range markers.Bookmarks {
		rows = append(rows, makeMarkRow(uuid, string(bm.Type), bm.Label, indexByUUID, lineByUUID, stats.Compactions))
	}
	for _, cp := range markers.CommitPoints {
		rows = append(rows, makeMarkRow(cp.UUID, "commit", cp.Goal, indexByUUID, lineByUUID, stats.Compactions))
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].EntryIndex == -1 && rows[j].EntryIndex != -1 {
			return false
		}
		if rows[j].EntryIndex == -1 && rows[i].EntryIndex != -1 {
			return true
		}
		if rows[i].EntryIndex != rows[j].EntryIndex {
			return rows[i].EntryIndex < rows[j].EntryIndex
		}
		return rows[i].Type < rows[j].Type
	})

	if isJSON() {
		out := MarksOutputJSON{
			SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl"),
			Total:     len(rows),
			Marks:     make([]MarkEntryJSON, 0, len(rows)),
		}
		for _, r := range rows {
			out.Marks = append(out.Marks, MarkEntryJSON(r))
		}
		return printJSON(out)
	}

	if len(rows) == 0 {
		fmt.Println("No marks found.")
		return nil
	}

	fmt.Printf("Marks for %s\n", filepath.Base(path))
	fmt.Println("  TYPE         EPOCH  INDEX  UUID        LABEL")
	for _, r := range rows {
		shortUUID := r.UUID
		if len(shortUUID) > 8 {
			shortUUID = shortUUID[:8]
		}
		label := r.Label
		if label == "" {
			label = "—"
		}
		fmt.Printf("  %-12s %-5d  %-5d  %-10s %s\n",
			r.Type, r.Epoch, r.EntryIndex, shortUUID, label)
	}
	return nil
}

func makeMarkRow(
	uuid, typ, label string,
	indexByUUID map[string]int,
	lineByUUID map[string]int,
	compactions []analyzer.CompactionEvent,
) markRow {
	idx, ok := indexByUUID[uuid]
	if !ok {
		idx = -1
	}
	line := lineByUUID[uuid]
	return markRow{
		UUID:       uuid,
		Type:       typ,
		Label:      label,
		EntryIndex: idx,
		LineNumber: line,
		Epoch:      epochForEntry(idx, compactions),
	}
}

func epochForEntry(idx int, compactions []analyzer.CompactionEvent) int {
	if idx < 0 || len(compactions) == 0 {
		return 0
	}
	epoch := 0
	for _, c := range compactions {
		if idx >= c.LineIndex {
			epoch++
		}
	}
	return epoch
}

func init() {
	marksCmd.Flags().BoolVar(&marksCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(marksCmd)
}
