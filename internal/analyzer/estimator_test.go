package analyzer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ppiankov/contextspectre/internal/jsonl"
)

func TestEstimateTokens_Text(t *testing.T) {
	e := &jsonl.Entry{
		Type: jsonl.TypeUser,
		Message: &jsonl.Message{
			Content: json.RawMessage(`"hello world this is a test message"`),
		},
	}
	tokens := EstimateTokens(e)
	// 35 chars / 4 = 8 tokens
	if tokens != 8 {
		t.Errorf("expected 8 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_TextBlocks(t *testing.T) {
	e := &jsonl.Entry{
		Type: jsonl.TypeAssistant,
		Message: &jsonl.Message{
			Content: json.RawMessage(`[{"type":"text","text":"hello world"}]`),
		},
	}
	tokens := EstimateTokens(e)
	// 11 chars / 4 = 2 tokens
	if tokens != 2 {
		t.Errorf("expected 2 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_Image(t *testing.T) {
	imgData := strings.Repeat("A", 7500) // 7500 / 750 = 10 tokens
	e := &jsonl.Entry{
		Type: jsonl.TypeUser,
		Message: &jsonl.Message{
			Content: json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + imgData + `"}}]`),
		},
	}
	tokens := EstimateTokens(e)
	if tokens != 10 {
		t.Errorf("expected 10 tokens for image, got %d", tokens)
	}
}

func TestEstimateTokens_ToolUse(t *testing.T) {
	e := &jsonl.Entry{
		Type: jsonl.TypeAssistant,
		Message: &jsonl.Message{
			Content: json.RawMessage(`[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/tmp/test.go"}}]`),
		},
	}
	tokens := EstimateTokens(e)
	if tokens <= 0 {
		t.Error("expected positive tokens for tool_use")
	}
}

func TestEstimateTokens_ToolResult(t *testing.T) {
	e := &jsonl.Entry{
		Type: jsonl.TypeUser,
		Message: &jsonl.Message{
			Content: json.RawMessage(`[{"tool_use_id":"t1","type":"tool_result","content":"this is the output of the command"}]`),
		},
	}
	tokens := EstimateTokens(e)
	// "this is the output of the command" = 34 chars / 4 = 8 tokens
	if tokens != 8 {
		t.Errorf("expected 8 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_WithToolUseResult(t *testing.T) {
	e := &jsonl.Entry{
		Type: jsonl.TypeUser,
		Message: &jsonl.Message{
			Content: json.RawMessage(`"short message"`),
		},
		ToolUseResult: json.RawMessage(`{"filenames":["a.go","b.go"],"exitCode":0}`),
	}
	tokens := EstimateTokens(e)
	// Text: 13/4=3, ToolUseResult: 44/4=11, total ~14
	if tokens < 10 {
		t.Errorf("expected >10 tokens with toolUseResult, got %d", tokens)
	}
}

func TestEstimateTokens_NilMessage(t *testing.T) {
	e := &jsonl.Entry{Type: jsonl.TypeProgress}
	tokens := EstimateTokens(e)
	if tokens != 0 {
		t.Errorf("expected 0 tokens for nil message, got %d", tokens)
	}
}

func TestEstimateImageBytes(t *testing.T) {
	imgData := strings.Repeat("A", 4000) // 4000 * 3/4 = 3000 bytes
	e := &jsonl.Entry{
		Type: jsonl.TypeUser,
		Message: &jsonl.Message{
			Content: json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + imgData + `"}}]`),
		},
	}
	bytes := EstimateImageBytes(e)
	if bytes != 3000 {
		t.Errorf("expected 3000 bytes, got %d", bytes)
	}
}

func TestEstimateImageBytes_NoImage(t *testing.T) {
	e := &jsonl.Entry{
		Type:    jsonl.TypeUser,
		Message: &jsonl.Message{Content: json.RawMessage(`"just text"`)},
	}
	if EstimateImageBytes(e) != 0 {
		t.Error("expected 0 image bytes for text message")
	}
}
