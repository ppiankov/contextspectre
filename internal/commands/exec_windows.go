//go:build windows

package commands

import (
	"fmt"
	"os"
	"os/exec"
)

// execClaude launches claude as a child process (Windows cannot exec-replace).
func execClaude(args []string) error {
	binary, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}
	cmd := exec.Command(binary, args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
