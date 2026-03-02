package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check tool health and environment",
	RunE:  runDoctor,
}

// DoctorOutput is the JSON output for the doctor command.
type DoctorOutput struct {
	Version    string           `json:"version"`
	Platform   string           `json:"platform"`
	ClaudeDir  DoctorCheck      `json:"claude_dir"`
	Sessions   DoctorCheck      `json:"sessions"`
	Companions []CompanionCheck `json:"companions"`
}

// DoctorCheck holds a single health check result.
type DoctorCheck struct {
	Status  string `json:"status"` // "ok", "warn", "error"
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// CompanionCheck holds info about a companion tool.
type CompanionCheck struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	out := DoctorOutput{
		Version:  version,
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}

	dir := resolveClaudeDir()

	// Check claude directory
	if fi, err := os.Stat(dir); err != nil {
		out.ClaudeDir = DoctorCheck{Status: "error", Message: "claude directory not found", Detail: dir}
	} else if !fi.IsDir() {
		out.ClaudeDir = DoctorCheck{Status: "error", Message: "claude path is not a directory", Detail: dir}
	} else {
		out.ClaudeDir = DoctorCheck{Status: "ok", Message: "claude directory accessible", Detail: dir}
	}

	// Check sessions
	d := &session.Discoverer{ClaudeDir: dir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		out.Sessions = DoctorCheck{Status: "error", Message: fmt.Sprintf("list sessions: %v", err)}
	} else if len(sessions) == 0 {
		out.Sessions = DoctorCheck{Status: "warn", Message: "no sessions found"}
	} else {
		out.Sessions = DoctorCheck{
			Status:  "ok",
			Message: fmt.Sprintf("%d sessions found", len(sessions)),
		}
	}

	// Check companion tools
	companions := []string{"ancc", "chainwatch"}
	for _, name := range companions {
		cc := CompanionCheck{Name: name}
		if path, err := exec.LookPath(name); err == nil {
			cc.Available = true
			cc.Path = path
		}
		out.Companions = append(out.Companions, cc)
	}

	if isJSON() {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Text output
	fmt.Printf("contextspectre doctor (%s)\n\n", out.Platform)

	printCheck("Claude directory", out.ClaudeDir)
	printCheck("Sessions", out.Sessions)

	fmt.Println()
	fmt.Println("Companion tools:")
	for _, c := range out.Companions {
		if c.Available {
			fmt.Printf("  %s: found at %s\n", c.Name, c.Path)
		} else {
			fmt.Printf("  %s: not found\n", c.Name)
		}
	}

	return nil
}

func printCheck(label string, check DoctorCheck) {
	icon := "?"
	switch check.Status {
	case "ok":
		icon = "ok"
	case "warn":
		icon = "!!"
	case "error":
		icon = "XX"
	}
	msg := check.Message
	if check.Detail != "" {
		msg += " (" + check.Detail + ")"
	}
	fmt.Printf("  [%s] %s: %s\n", icon, label, msg)
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
