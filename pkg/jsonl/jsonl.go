// Package jsonl exposes the contextspectre JSONL parser types and functions for
// use by external consumers (e.g. contextspectre-pro). All types are thin
// re-exports from the internal jsonl package — no logic lives here.
package jsonl

import "github.com/ppiankov/contextspectre/internal/jsonl"

// Type aliases.
type Entry = jsonl.Entry
type Message = jsonl.Message
type ContentBlock = jsonl.ContentBlock
type Usage = jsonl.Usage
type MessageType = jsonl.MessageType

// MessageType constants.
const (
	TypeUser                = jsonl.TypeUser
	TypeAssistant           = jsonl.TypeAssistant
	TypeSystem              = jsonl.TypeSystem
	TypeProgress            = jsonl.TypeProgress
	TypeFileHistorySnapshot = jsonl.TypeFileHistorySnapshot
	TypeQueueOperation      = jsonl.TypeQueueOperation
	TypeCustomTitle         = jsonl.TypeCustomTitle
	TypeAgentName           = jsonl.TypeAgentName
)

// Parse loads and parses a session JSONL file into a slice of Entry values.
var Parse = jsonl.Parse

// ParseRaw loads a session JSONL file and returns both parsed entries and raw
// line bytes. Use when you need to write modified lines back to disk.
var ParseRaw = jsonl.ParseRaw

// ParseContentBlocks decodes the raw JSON content array of a message into
// typed ContentBlock values. Used by semantic analysis to inspect tool_use
// and tool_result blocks entry-by-entry.
var ParseContentBlocks = jsonl.ParseContentBlocks
