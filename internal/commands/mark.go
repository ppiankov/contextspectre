package commands

import (
	"fmt"
	"os"

	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/spf13/cobra"
)

var markList bool

var markCmd = &cobra.Command{
	Use:   "mark <session-id-or-path> <uuid> <keep|candidate|noise|clear>",
	Short: "Set a marker on a session entry",
	Long: `Set or clear a marker on a session entry by UUID. Markers persist in a
sidecar file alongside the session JSONL.

  keep       — protect entry from cleanup operations
  candidate  — flag entry as safe to remove
  noise      — flag entry as noise
  clear      — remove the marker

Use --list to show all markers for a session:
  contextspectre mark <session-id-or-path> --list`,
	Args: cobra.RangeArgs(1, 3),
	RunE: runMark,
}

func runMark(cmd *cobra.Command, args []string) error {
	path := resolveSessionPath(args[0])
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("session not found: %s", path)
	}

	if markList {
		return runMarkList(path)
	}

	if len(args) != 3 {
		return fmt.Errorf("usage: mark <session> <uuid> <keep|candidate|noise|clear>")
	}

	uuid := args[1]
	action := args[2]

	markers, err := editor.LoadMarkers(path)
	if err != nil {
		return fmt.Errorf("load markers: %w", err)
	}

	var markerType editor.MarkerType
	switch action {
	case "keep":
		markerType = editor.MarkerKeep
	case "candidate":
		markerType = editor.MarkerCandidate
	case "noise":
		markerType = editor.MarkerNoise
	case "clear":
		markers.Clear(uuid)
		if err := editor.SaveMarkers(path, markers); err != nil {
			return fmt.Errorf("save markers: %w", err)
		}
		if isJSON() {
			return printJSON(MarkOutput{
				UUID:   uuid,
				Marker: "",
				Action: "cleared",
			})
		}
		fmt.Printf("Cleared marker on %s\n", uuid)
		return nil
	default:
		return fmt.Errorf("invalid marker type %q: use keep, candidate, noise, or clear", action)
	}

	markers.Set(uuid, markerType)
	if err := editor.SaveMarkers(path, markers); err != nil {
		return fmt.Errorf("save markers: %w", err)
	}

	if isJSON() {
		return printJSON(MarkOutput{
			UUID:   uuid,
			Marker: string(markerType),
			Action: "set",
		})
	}
	fmt.Printf("Set %s marker on %s\n", action, uuid)
	return nil
}

func runMarkList(path string) error {
	markers, err := editor.LoadMarkers(path)
	if err != nil {
		return fmt.Errorf("load markers: %w", err)
	}

	if isJSON() {
		out := MarkListOutput{
			Markers: make(map[string]string, len(markers.Markers)),
			Total:   len(markers.Markers),
		}
		for uuid, mt := range markers.Markers {
			out.Markers[uuid] = string(mt)
		}
		return printJSON(out)
	}

	if len(markers.Markers) == 0 {
		fmt.Println("No markers set.")
		return nil
	}

	fmt.Printf("Markers (%d):\n", len(markers.Markers))
	for uuid, mt := range markers.Markers {
		fmt.Printf("  %s  %s\n", uuid, mt)
	}
	return nil
}

func init() {
	markCmd.Flags().BoolVar(&markList, "list", false, "List all markers for a session")
	rootCmd.AddCommand(markCmd)
}
