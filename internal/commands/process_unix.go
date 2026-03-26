//go:build !windows

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// findClaudeProcess checks if a claude process is already running with --resume
// for the given session ID or slug. Returns the PID if found, 0 if not.
func findClaudeProcess(sessionID, slug string) (int, error) {
	patterns := []string{sessionID}
	if slug != "" {
		patterns = append(patterns, slug)
	}

	for _, pattern := range patterns {
		pid, err := findProcessByPattern(pattern)
		if err != nil {
			continue
		}
		if pid > 0 {
			return pid, nil
		}
	}
	return 0, nil
}

func findProcessByPattern(pattern string) (int, error) {
	// Use pgrep to find claude processes matching --resume <pattern>.
	// -f matches against full command line, -a shows the command.
	cmd := exec.Command("pgrep", "-f", fmt.Sprintf("claude.*--resume.*%s", pattern))
	out, err := cmd.Output()
	if err != nil {
		// pgrep exits 1 when no match — not an error.
		return 0, nil
	}

	myPID := os.Getpid()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		// Skip our own process and parent (contextspectre itself).
		if pid == myPID || pid == os.Getppid() {
			continue
		}
		return pid, nil
	}
	return 0, nil
}
