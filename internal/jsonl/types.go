package jsonl

import (
	"encoding/json"
	"fmt"
	"time"
)

// MessageType enumerates JSONL line types.
type MessageType string

const (
	TypeUser                MessageType = "user"
	TypeAssistant           MessageType = "assistant"
	TypeProgress            MessageType = "progress"
	TypeFileHistorySnapshot MessageType = "file-history-snapshot"
	TypeSystem              MessageType = "system"
	TypeQueueOperation      MessageType = "queue-operation"
)

// Entry is the top-level structure of every JSONL line.
// Large fields use json.RawMessage to avoid unnecessary allocation.
type Entry struct {
	Type             MessageType     `json:"type"`
	UUID             string          `json:"uuid,omitempty"`
	ParentUUID       string          `json:"parentUuid,omitempty"`
	Timestamp        time.Time       `json:"timestamp"`
	SessionID        string          `json:"sessionId,omitempty"`
	CWD              string          `json:"cwd,omitempty"`
	Version          string          `json:"version,omitempty"`
	GitBranch        string          `json:"gitBranch,omitempty"`
	IsSidechain      bool            `json:"isSidechain,omitempty"`
	UserType         string          `json:"userType,omitempty"`
	Slug             string          `json:"slug,omitempty"`
	PermissionMode   string          `json:"permissionMode,omitempty"`
	RequestID        string          `json:"requestId,omitempty"`
	Message          *Message        `json:"message,omitempty"`
	Data             json.RawMessage `json:"data,omitempty"`
	ToolUseID        string          `json:"toolUseID,omitempty"`
	ParentToolUseID  string          `json:"parentToolUseID,omitempty"`
	MessageID        string          `json:"messageId,omitempty"`
	Snapshot         json.RawMessage `json:"snapshot,omitempty"`
	ToolUseResult    json.RawMessage `json:"toolUseResult,omitempty"`
	IsSnapshotUpdate bool            `json:"isSnapshotUpdate,omitempty"`

	// Computed fields (not serialized to JSON)
	LineNumber int `json:"-"`
	RawSize    int `json:"-"`
}

// Message holds the role+content from user/assistant messages.
type Message struct {
	Role       string          `json:"role,omitempty"`
	Content    json.RawMessage `json:"content"`
	Model      string          `json:"model,omitempty"`
	ID         string          `json:"id,omitempty"`
	Type       string          `json:"type,omitempty"`
	Usage      *Usage          `json:"usage,omitempty"`
	StopReason *string         `json:"stop_reason,omitempty"`
}

// Usage holds token counts from assistant messages.
type Usage struct {
	InputTokens              int    `json:"input_tokens"`
	OutputTokens             int    `json:"output_tokens"`
	CacheCreationInputTokens int    `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int    `json:"cache_read_input_tokens"`
	ServiceTier              string `json:"service_tier,omitempty"`
}

// TotalContextTokens returns the total context window consumption
// at the time this assistant message was generated.
func (u *Usage) TotalContextTokens() int {
	if u == nil {
		return 0
	}
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

// ContentBlock represents a single block within a message content array.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Name      string          `json:"name,omitempty"`
	ID        string          `json:"id,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Source    *ImageSource    `json:"source,omitempty"`
	Caller    json.RawMessage `json:"caller,omitempty"`
}

// ImageSource holds base64 image data.
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// ParseContentBlocks parses message content into individual blocks.
// Content can be a JSON string (returns one text block) or an array of blocks.
func ParseContentBlocks(raw json.RawMessage) ([]ContentBlock, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Try string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []ContentBlock{{Type: "text", Text: s}}, nil
	}

	// Try array of blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("parse content blocks: %w", err)
	}
	return blocks, nil
}

// ContentPreview extracts a short text preview from message content.
func (e *Entry) ContentPreview(maxLen int) string {
	if e.Message == nil {
		return typeLabel(e)
	}

	blocks, err := ParseContentBlocks(e.Message.Content)
	if err != nil || len(blocks) == 0 {
		return typeLabel(e)
	}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				return truncate(b.Text, maxLen)
			}
		case "tool_use":
			return fmt.Sprintf("[tool: %s]", b.Name)
		case "tool_result":
			var s string
			if json.Unmarshal(b.Content, &s) == nil && s != "" {
				return truncate(s, maxLen)
			}
			return fmt.Sprintf("[tool_result: %s]", b.ToolUseID)
		case "image":
			size := 0
			if b.Source != nil {
				size = len(b.Source.Data) * 3 / 4 / 1024 // approx KB
			}
			return fmt.Sprintf("[image %d KB]", size)
		}
	}
	return typeLabel(e)
}

// IsConversational returns true for user/assistant types
// (messages that consume context window).
func (e *Entry) IsConversational() bool {
	return e.Type == TypeUser || e.Type == TypeAssistant
}

// HasImages returns true if the message content contains base64 images.
func (e *Entry) HasImages() bool {
	if e.Message == nil {
		return false
	}
	blocks, err := ParseContentBlocks(e.Message.Content)
	if err != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "image" && b.Source != nil && len(b.Source.Data) > 100 {
			return true
		}
	}
	return false
}

// ToolUseIDs returns tool_use IDs from assistant message content blocks.
func (e *Entry) ToolUseIDs() []string {
	if e.Message == nil || e.Type != TypeAssistant {
		return nil
	}
	blocks, err := ParseContentBlocks(e.Message.Content)
	if err != nil {
		return nil
	}
	var ids []string
	for _, b := range blocks {
		if b.Type == "tool_use" && b.ID != "" {
			ids = append(ids, b.ID)
		}
	}
	return ids
}

func typeLabel(e *Entry) string {
	switch e.Type {
	case TypeProgress:
		return "[progress]"
	case TypeFileHistorySnapshot:
		return "[file-history-snapshot]"
	case TypeSystem:
		return "[system]"
	case TypeQueueOperation:
		return "[queue-operation]"
	default:
		return string(e.Type)
	}
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 80
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
