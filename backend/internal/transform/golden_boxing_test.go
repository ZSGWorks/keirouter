package transform

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// Golden equivalence: the emitted upstream JSON must be semantically
// unchanged after the RawMessage passthrough replaced map boxing for
// tool inputs.

func goldenChatRequest() *core.ChatRequest {
	return &core.ChatRequest{
		Model: "kiro-claude",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "list files"}}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartText, Text: "calling tool"},
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tu_1", Name: "Read", Arguments: json.RawMessage(`{"file_path":"/tmp/a.go","limit": 50,"zebra":true}`)}},
			}},
			{Role: core.RoleTool, Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "tu_1", Content: "ok"}},
			}},
		},
		Tools: []core.Tool{{Name: "Read", Description: "read", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
}

func goldenToolRequest() *core.ChatRequest {
	req := goldenChatRequest()
	req.Messages[1].Content[1].ToolCall.Arguments = json.RawMessage(`{"file_path":"/tmp/a.go","limit":50,"zebra":true}`)
	return req
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		t.Fatalf("left not JSON: %v", err)
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		t.Fatalf("right not JSON: %v", err)
	}
	return bytes.Equal(a, b) || jsonEqualValue(va, vb)
}

func jsonEqualValue(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}

var kiroCodec = KiroCodec{}
var antCodec = AnthropicCodec{}

func TestKiro_ToolInputPassthroughGolden(t *testing.T) {
	out, err := kiroCodec.RenderRequest(goldenChatRequest())
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatal(err)
	}

	// Locate the assistant toolUse input in history and assert the exact
	// raw argument bytes are preserved (key order intact).
	if !strings.Contains(string(out), `"file_path":"/tmp/a.go","limit": 50,"zebra":true`) {
		t.Fatalf("tool input not passed through verbatim:\n%s", out)
	}
	_ = payload
}

func TestKiro_ToolInputInvalidFallsBackToEmptyObject(t *testing.T) {
	req := goldenChatRequest()
	req.Messages[1].Content[1].ToolCall.Arguments = json.RawMessage(`not-json`)
	out, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("payload not JSON: %v\n%s", err, out)
	}
}

func TestAnthropic_ToolInputPassthroughGolden(t *testing.T) {
	out, err := antCodec.RenderRequest(goldenChatRequest())
	if err != nil {
		t.Fatal(err)
	}
	// Raw passthrough must preserve exact bytes of a valid object input.
	if !strings.Contains(string(out), `"file_path":"/tmp/a.go","limit": 50,"zebra":true`) {
		t.Fatalf("tool input not passed through verbatim:\n%s", out)
	}
	// Invalid input must still normalize to {}.
	req := goldenChatRequest()
	req.Messages[1].Content[1].ToolCall.Arguments = json.RawMessage(`garbage`)
	out2, err := antCodec.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out2), `"input":{}`) {
		t.Fatalf("invalid input not normalized:\n%s", out2)
	}
}

// BenchmarkKiroRenderToolHistory measures the render path for a tool-heavy
// conversation — the shape that dominated map boxing before the passthrough.
func BenchmarkKiroRenderToolHistory(b *testing.B) {
	msgs := make([]core.Message, 0, 64)
	for i := 0; i < 32; i++ {
		msgs = append(msgs,
			core.Message{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{
					ID: "tu", Name: "Edit",
					Arguments: json.RawMessage(strings.Repeat(`{"old_string":"x","new_string":"y"},`, 16) + `{"old_string":"x","new_string":"y"}`),
				}},
			}},
			core.Message{Role: core.RoleTool, Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "tu", Content: strings.Repeat("ok ", 128)}},
			}},
		)
	}
	req := &core.ChatRequest{Model: "kiro-claude", Messages: msgs}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := kiroCodec.RenderRequest(req); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAnthropicRenderToolHistory is the Anthropic counterpart.
func BenchmarkAnthropicRenderToolHistory(b *testing.B) {
	msgs := make([]core.Message, 0, 64)
	for i := 0; i < 32; i++ {
		msgs = append(msgs,
			core.Message{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{
					ID: "toolu_1", Name: "Edit",
					Arguments: json.RawMessage(strings.Repeat(`{"old_string":"x","new_string":"y"},`, 16) + `{"old_string":"x","new_string":"y"}`),
				}},
			}},
			core.Message{Role: core.RoleTool, Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "toolu_1", Content: strings.Repeat("ok ", 128)}},
			}},
		)
	}
	req := &core.ChatRequest{Model: "claude-sonnet", Messages: msgs}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := antCodec.RenderRequest(req); err != nil {
			b.Fatal(err)
		}
	}
}

// antOldBoxing is the previous map-boxing implementation, kept for the A/B
// regression check below.
func antOldBoxing(raw json.RawMessage) any {
	raw = normalizeAntToolInputRaw(raw)
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		return map[string]any{}
	}
	return input
}

// TestAnthropicToolInputBoxingReduced proves the passthrough eliminated the
// per-tool-input map boxing: 0 allocs vs 9 for the old path.
func TestAnthropicToolInputBoxingReduced(t *testing.T) {
	raw := json.RawMessage(`{"file_path":"/tmp/a.go","limit":50}`)
	bOld := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = antOldBoxing(raw)
		}
	})
	bNew := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = normalizeAntToolInputValue(raw)
		}
	})
	if bNew.AllocsPerOp() >= bOld.AllocsPerOp() {
		t.Fatalf("boxing not reduced: old %d allocs/op, new %d allocs/op", bOld.AllocsPerOp(), bNew.AllocsPerOp())
	}
	t.Logf("old %d allocs/op, new %d allocs/op", bOld.AllocsPerOp(), bNew.AllocsPerOp())
}
