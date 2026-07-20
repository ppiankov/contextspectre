// Package session exposes contextspectre's session discovery types for use by
// external consumers. All types are thin re-exports from the internal session
// package — no logic lives here.
package session

import "github.com/ppiankov/contextspectre/internal/session"

// Type aliases.
type Info = session.Info
type QuickStats = session.QuickStats
type Discoverer = session.Discoverer

// DefaultClaudeDir returns the default ~/.claude directory path, with WSL2
// detection for Windows-hosted Claude installations.
var DefaultClaudeDir = session.DefaultClaudeDir
