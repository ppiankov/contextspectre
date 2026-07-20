// Package repair exposes the contextspectre repair and cleanup API for use by
// external consumers. All types and functions are thin re-exports from internal
// packages — no logic lives here.
package repair

import "github.com/ppiankov/contextspectre/internal/editor"

// Type aliases.
type CleanAllOpts = editor.CleanAllOpts
type CleanAllResult = editor.CleanAllResult
type CleanLiveOpts = editor.CleanLiveOpts
type CleanLiveResult = editor.CleanLiveResult

// CleanAll runs the full repair pipeline on a session file at path.
// Safe to call on inactive sessions.
var CleanAll = editor.CleanAll

// CleanLive runs a race-safe cleanup pass on an active session.
// It detects concurrent writes and aborts rather than corrupt the file.
var CleanLive = editor.CleanLive
