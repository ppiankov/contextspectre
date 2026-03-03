package commands

import (
	"fmt"
	"os"

	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/spf13/cobra"
)

var (
	markList bool
	markCWD  bool
)

var markCmd = &cobra.Command{
	Use:   "mark [session-id-or-path] <uuid> <action>",
	Short: "Set a marker or phase on a session entry",
	Long: `Set or clear a marker or reasoning phase on a session entry by UUID.
Markers and phases persist in a sidecar file alongside the session JSONL.

Markers (intent labels):
  keep         — protect entry from cleanup operations
  candidate    — flag entry as safe to remove
  noise        — flag entry as noise

Phases (reasoning stages):
  exploratory  — trying things, investigating, reading code
  decision     — committing to an approach, choosing a design
  operational  — executing the decision, writing code, running tests

  clear        — remove the marker and phase

Use --list to show all markers and phases for a session:
  contextspectre mark <session-id-or-path> --list
  contextspectre mark --cwd --list`,
	Args: cobra.RangeArgs(0, 3),
	RunE: runMark,
}

func runMark(cmd *cobra.Command, args []string) error {
	var path, uuid, action string

	if markCWD || (len(args) == 0 && os.Getenv("CLAUDECODE") == "1") {
		// --cwd mode: args are [uuid action] or empty (for --list)
		p, err := resolveSessionArg(nil, true)
		if err != nil {
			return err
		}
		path = p
		if markList {
			return runMarkList(path)
		}
		if len(args) != 2 {
			return fmt.Errorf("usage: mark --cwd <uuid> <keep|candidate|noise|exploratory|decision|operational|clear>")
		}
		uuid = args[0]
		action = args[1]
	} else {
		// Traditional mode: args are [session uuid action] or [session] (for --list)
		if len(args) == 0 {
			return fmt.Errorf("provide a session ID or use --cwd")
		}
		p, err := resolveSessionArg(args[:1], false)
		if err != nil {
			return err
		}
		path = p
		if markList {
			return runMarkList(path)
		}
		if len(args) != 3 {
			return fmt.Errorf("usage: mark <session> <uuid> <keep|candidate|noise|exploratory|decision|operational|clear>")
		}
		uuid = args[1]
		action = args[2]
	}

	markers, err := editor.LoadMarkers(path)
	if err != nil {
		return fmt.Errorf("load markers: %w", err)
	}

	// Phase actions
	switch action {
	case "exploratory", "decision", "operational":
		var phase editor.PhaseType
		switch action {
		case "exploratory":
			phase = editor.PhaseExploratory
		case "decision":
			phase = editor.PhaseDecision
		case "operational":
			phase = editor.PhaseOperational
		}
		markers.SetPhase(uuid, phase)
		if err := editor.SaveMarkers(path, markers); err != nil {
			return fmt.Errorf("save markers: %w", err)
		}
		if isJSON() {
			return printJSON(MarkOutput{
				UUID:   uuid,
				Phase:  string(phase),
				Action: "set",
			})
		}
		fmt.Printf("Set %s phase on %s\n", action, uuid)
		return nil
	}

	// Marker actions
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
		markers.ClearPhase(uuid)
		if err := editor.SaveMarkers(path, markers); err != nil {
			return fmt.Errorf("save markers: %w", err)
		}
		if isJSON() {
			return printJSON(MarkOutput{
				UUID:   uuid,
				Action: "cleared",
			})
		}
		fmt.Printf("Cleared marker and phase on %s\n", uuid)
		return nil
	default:
		return fmt.Errorf("invalid action %q: use keep, candidate, noise, exploratory, decision, operational, or clear", action)
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
			Total:   len(markers.Markers) + len(markers.Phases),
		}
		for uuid, mt := range markers.Markers {
			out.Markers[uuid] = string(mt)
		}
		if len(markers.Phases) > 0 {
			out.Phases = make(map[string]string, len(markers.Phases))
			for uuid, pt := range markers.Phases {
				out.Phases[uuid] = string(pt)
			}
		}
		return printJSON(out)
	}

	if len(markers.Markers) == 0 && len(markers.Phases) == 0 {
		fmt.Println("No markers or phases set.")
		return nil
	}

	if len(markers.Markers) > 0 {
		fmt.Printf("Markers (%d):\n", len(markers.Markers))
		for uuid, mt := range markers.Markers {
			fmt.Printf("  %s  %s\n", uuid, mt)
		}
	}
	if len(markers.Phases) > 0 {
		fmt.Printf("Phases (%d):\n", len(markers.Phases))
		for uuid, pt := range markers.Phases {
			fmt.Printf("  %s  %s\n", uuid, pt)
		}
	}
	return nil
}

func init() {
	markCmd.Flags().BoolVar(&markList, "list", false, "List all markers for a session")
	markCmd.Flags().BoolVar(&markCWD, "cwd", false, "Use most recent session for current directory")
	rootCmd.AddCommand(markCmd)
}
