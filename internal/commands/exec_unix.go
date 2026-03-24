//go:build !windows

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// execClaude replaces the current process with claude (Unix exec).
func execClaude(args []string) error {
	binary, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}
	return syscall.Exec(binary, args, os.Environ())
}
