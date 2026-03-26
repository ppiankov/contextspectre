//go:build windows

package commands

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// findClaudeProcess checks if a claude process is already running with --resume
// for the given session ID or slug. Returns the PID if found, 0 if not.
// On Windows this uses wmic/tasklist — best effort.
func findClaudeProcess(sessionID, slug string) (int, error) {
	patterns := []string{sessionID}
	if slug != "" {
		patterns = append(patterns, slug)
	}

	// Try wmic to get command lines of node processes (claude runs on node).
	cmd := exec.Command("wmic", "process", "where", "name like '%node%' or name like '%claude%'",
		"get", "ProcessId,CommandLine", "/format:list")
	out, err := cmd.Output()
	if err != nil {
		// wmic may not be available — best effort.
		return 0, nil
	}

	myPID := os.Getpid()
	lines := strings.Split(string(out), "\n")
	var currentCmdLine string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CommandLine=") {
			currentCmdLine = line
		} else if strings.HasPrefix(line, "ProcessId=") {
			pidStr := strings.TrimPrefix(line, "ProcessId=")
			pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
			if err != nil || pid == myPID || pid == os.Getppid() {
				currentCmdLine = ""
				continue
			}
			for _, pattern := range patterns {
				if strings.Contains(currentCmdLine, "--resume") &&
					strings.Contains(currentCmdLine, pattern) {
					return pid, nil
				}
			}
			currentCmdLine = ""
		}
	}
	return 0, nil
}
