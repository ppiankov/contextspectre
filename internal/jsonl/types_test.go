package jsonl

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUsage_TotalContextTokens(t *testing.T) {
	tests := []struct {
		name  string
		usage *Usage
		want  int
	}{
		{"nil usage", nil, 0},
		{"zero values", &Usage{}, 0},
		{"input only", &Usage{InputTokens: 100}, 100},
		{"all fields", &Usage{
			InputTokens:              100,
			CacheCreationInputTokens: 5000,
			CacheReadInputTokens:     2000,
			OutputTokens:             50,
		}, 7100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.usage.TotalContextTokens()
			if got != tt.want {
				t.Errorf("TotalContextTokens() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseContentBlocks_String(t *testing.T) {
	raw := json.RawMessage(`"hello world"`)
	blocks, err := ParseContentBlocks(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "hello world" {
		t.Errorf("expected text block with 'hello world', got %+v", blocks[0])
	}
}

func TestParseContentBlocks_Array(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"hi"},{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"/tmp/f"}}]`)
	blocks, err := ParseContentBlocks(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "hi" {
		t.Errorf("block 0: expected text 'hi', got %+v", blocks[0])
	}
	if blocks[1].Type != "tool_use" || blocks[1].Name != "Read" {
		t.Errorf("block 1: expected tool_use Read, got %+v", blocks[1])
	}
}

func TestParseContentBlocks_Empty(t *testing.T) {
	blocks, err := ParseContentBlocks(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestParseContentBlocks_Image(t *testing.T) {
	raw := json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0K..."}}]`)
	blocks, err := ParseContentBlocks(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Source == nil {
		t.Fatal("expected image source")
	}
	if blocks[0].Source.MediaType != "image/png" {
		t.Errorf("expected image/png, got %s", blocks[0].Source.MediaType)
	}
}

func TestEntry_ContentPreview(t *testing.T) {
	tests := []struct {
		name   string
		entry  Entry
		maxLen int
		want   string
	}{
		{
			name:  "nil message",
			entry: Entry{Type: TypeProgress},
			want:  "[progress]",
		},
		{
			name: "text content string",
			entry: Entry{
				Type:    TypeUser,
				Message: &Message{Content: json.RawMessage(`"hello world"`)},
			},
			want: "hello world",
		},
		{
			name: "text content truncated",
			entry: Entry{
				Type:    TypeUser,
				Message: &Message{Content: json.RawMessage(`"this is a very long message that should be truncated"`)},
			},
			maxLen: 20,
			want:   "this is a very lo...",
		},
		{
			name: "tool use",
			entry: Entry{
				Type:    TypeAssistant,
				Message: &Message{Content: json.RawMessage(`[{"type":"tool_use","id":"t1","name":"Bash","input":{}}]`)},
			},
			want: "[tool: Bash]",
		},
		{
			name: "image block",
			entry: Entry{
				Type:    TypeUser,
				Message: &Message{Content: json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + strings.Repeat("A", 3072) + `"}}]`)},
			},
			want: "[image 2 KB]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entry.ContentPreview(tt.maxLen)
			if got != tt.want {
				t.Errorf("ContentPreview() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEntry_IsConversational(t *testing.T) {
	tests := []struct {
		typ  MessageType
		want bool
	}{
		{TypeUser, true},
		{TypeAssistant, true},
		{TypeProgress, false},
		{TypeFileHistorySnapshot, false},
		{TypeSystem, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.typ), func(t *testing.T) {
			e := Entry{Type: tt.typ}
			if got := e.IsConversational(); got != tt.want {
				t.Errorf("IsConversational() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntry_HasImages(t *testing.T) {
	noImg := Entry{
		Type:    TypeUser,
		Message: &Message{Content: json.RawMessage(`"just text"`)},
	}
	if noImg.HasImages() {
		t.Error("expected no images for text content")
	}

	// Image with short data (< 100 bytes) should not count
	shortImg := Entry{
		Type:    TypeUser,
		Message: &Message{Content: json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"short"}}]`)},
	}
	if shortImg.HasImages() {
		t.Error("expected no images for short data")
	}

	// Real image data
	imgData := strings.Repeat("A", 200)
	withImg := Entry{
		Type:    TypeUser,
		Message: &Message{Content: json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + imgData + `"}}]`)},
	}
	if !withImg.HasImages() {
		t.Error("expected images for real image data")
	}
}

func TestEntry_ToolUseIDs(t *testing.T) {
	e := Entry{
		Type:    TypeAssistant,
		Message: &Message{Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_1","name":"Read","input":{}},{"type":"text","text":"hi"},{"type":"tool_use","id":"toolu_2","name":"Bash","input":{}}]`)},
	}
	ids := e.ToolUseIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 tool use IDs, got %d", len(ids))
	}
	if ids[0] != "toolu_1" || ids[1] != "toolu_2" {
		t.Errorf("unexpected IDs: %v", ids)
	}

	// Non-assistant type returns nil
	e2 := Entry{Type: TypeUser, Message: &Message{Content: json.RawMessage(`"text"`)}}
	if ids := e2.ToolUseIDs(); ids != nil {
		t.Errorf("expected nil for user type, got %v", ids)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 80, "short"},
		{"hello world", 8, "hello..."},
		{"exact", 5, "exact"},
		{"", 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestEntry_RoundTrip(t *testing.T) {
	e := Entry{
		Type:       TypeUser,
		UUID:       "test-uuid",
		ParentUUID: "parent-uuid",
		SessionID:  "sess-1",
		Message: &Message{
			Role:    "user",
			Content: json.RawMessage(`"hello"`),
		},
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var e2 Entry
	if err := json.Unmarshal(data, &e2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if e2.UUID != e.UUID || e2.ParentUUID != e.ParentUUID || e2.Type != e.Type {
		t.Errorf("round-trip mismatch: got %+v", e2)
	}
}
