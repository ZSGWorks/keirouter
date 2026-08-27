package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

type cloudCodeRequestEnvelope struct {
	Request struct {
		Contents []struct {
			Role  string           `json:"role"`
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
	} `json:"request"`
}

func decodeCloudCodeRequest(t *testing.T, body []byte) cloudCodeRequestEnvelope {
	t.Helper()
	var envelope cloudCodeRequestEnvelope
	require.NoError(t, json.Unmarshal(body, &envelope))
	return envelope
}

func decodeCloudCodeRequestFromHTTP(t *testing.T, r *http.Request) cloudCodeRequestEnvelope {
	t.Helper()
	var envelope cloudCodeRequestEnvelope
	require.NoError(t, json.NewDecoder(r.Body).Decode(&envelope))
	return envelope
}

func finalCloudCodeTurn(t *testing.T, envelope cloudCodeRequestEnvelope) (string, []map[string]any) {
	t.Helper()
	require.NotEmpty(t, envelope.Request.Contents)
	last := envelope.Request.Contents[len(envelope.Request.Contents)-1]
	return last.Role, last.Parts
}

func cloudCodeTurnHasText(parts []map[string]any, want string) bool {
	for _, part := range parts {
		if text, _ := part["text"].(string); text == want {
			return true
		}
	}
	return false
}

func textPart(text string) core.ContentPart {
	return core.ContentPart{Type: core.PartText, Text: text}
}

func TestAntigravityClaudeConversationRepairUsesRenderedPayload(t *testing.T) {
	toolCall := core.ContentPart{
		Type: core.PartToolCall,
		ToolCall: &core.ToolCall{
			ID:        "call_1",
			Name:      "lookup",
			Arguments: json.RawMessage(`{"q":"test"}`),
		},
	}
	toolResult := core.ContentPart{
		Type:       core.PartToolResult,
		ToolResult: &core.ToolResult{CallID: "call_1", Content: "result"},
	}

	tests := []struct {
		name         string
		model        string
		force        bool
		messages     []core.Message
		wantRepaired bool
		wantRole     string
		wantContinue bool
	}{
		{
			name: "assistant text", model: "claude-opus-4-6-thinking",
			messages:     []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{textPart("question")}}, {Role: core.RoleAssistant, Content: []core.ContentPart{textPart("partial")}}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
		{
			name: "assistant thinking", model: "claude-opus-4-6-thinking",
			messages:     []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{{Type: core.PartThinking, Text: "reasoning"}}}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
		{
			name: "assistant tool call", model: "claude-opus-4-6-thinking",
			messages:     []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{toolCall}}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
		{
			name: "empty user", model: "claude-opus-4-6-thinking",
			messages:     []core.Message{{Role: core.RoleUser}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
		{
			name: "whitespace user", model: "claude-opus-4-6-thinking",
			messages:     []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{textPart("  \n\t")}}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
		{
			name: "empty tool turn", model: "claude-opus-4-6-thinking",
			messages:     []core.Message{{Role: core.RoleTool}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
		{
			name: "audio only is not rendered", model: "claude-opus-4-6-thinking",
			messages:     []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartAudio, Media: &core.MediaPayload{MIMEType: "audio/wav", Data: "YWJj"}}}}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
		{
			name: "image URL only is not rendered", model: "claude-opus-4-6-thinking",
			messages:     []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartImage, Media: &core.MediaPayload{MIMEType: "image/png", URL: "https://example.com/image.png"}}}}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
		{
			name: "valid user text", model: "claude-opus-4-6-thinking",
			messages: []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{textPart("question")}}},
			wantRole: "user",
		},
		{
			name: "valid tool result", model: "claude-opus-4-6-thinking",
			messages: []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{toolCall}}, {Role: core.RoleTool, Content: []core.ContentPart{toolResult}}},
			wantRole: "user",
		},
		{
			name: "valid tool result with forced retry", model: "claude-opus-4-6-thinking", force: true,
			messages:     []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{toolCall}}, {Role: core.RoleTool, Content: []core.ContentPart{toolResult}}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
		{
			name: "inline image", model: "claude-opus-4-6-thinking",
			messages: []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartImage, Media: &core.MediaPayload{MIMEType: "image/png", Data: "YWJj"}}}}},
			wantRole: "user",
		},
		{
			name: "unknown alias without force", model: "internal-opus-alias",
			messages: []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{textPart("partial")}}},
			wantRole: "model",
		},
		{
			name: "antigravity Gemini model is untouched", model: "gemini-3.1-pro-high",
			messages: []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{textPart("partial")}}},
			wantRole: "model",
		},
		{
			name: "unknown alias with force", model: "internal-opus-alias", force: true,
			messages:     []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{textPart("partial")}}},
			wantRepaired: true, wantRole: "user", wantContinue: true,
		},
	}

	c := NewAntigravity("antigravity", "https://unused.example")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &core.ChatRequest{Model: tt.model, Messages: tt.messages}
			before, err := json.Marshal(req)
			require.NoError(t, err)

			body, repaired, err := c.wrapRequestWithConversationRepair(req, core.Credentials{AccessToken: "token"}, "", tt.force)
			require.NoError(t, err)
			require.Equal(t, tt.wantRepaired, repaired)
			role, parts := finalCloudCodeTurn(t, decodeCloudCodeRequest(t, body))
			require.Equal(t, tt.wantRole, role)
			if tt.wantContinue {
				require.Equal(t, antigravityClaudeContinuation, parts[len(parts)-1]["text"])
			}

			after, err := json.Marshal(req)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after), "repair must not mutate the caller's request")
		})
	}
}

func TestAntigravityClaudeConversationRepairLeavesEmptyRequestsAlone(t *testing.T) {
	require.Nil(t, appendAntigravityClaudeContinuation(nil))
	empty := &core.ChatRequest{}
	require.Same(t, empty, appendAntigravityClaudeContinuation(empty))
}

func TestAntigravityClaudeConversationRepairDoesNotAffectGeminiCLI(t *testing.T) {
	req := &core.ChatRequest{
		Model:    "claude-opus-4-6-thinking",
		Messages: []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{textPart("partial")}}},
	}
	c := NewGeminiCLI("gemini-cli", "https://unused.example/v1internal")
	body, repaired, err := c.wrapRequestWithConversationRepair(req, core.Credentials{AccessToken: "token"}, "", true)
	require.NoError(t, err)
	require.False(t, repaired)
	role, _ := finalCloudCodeTurn(t, decodeCloudCodeRequest(t, body))
	require.Equal(t, "model", role)
}

func TestAntigravityClaudeUnknownAliasRetriesUnaryOnce(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		role, _ := finalCloudCodeTurn(t, decodeCloudCodeRequestFromHTTP(t, r))
		if requests == 1 {
			require.Equal(t, "model", role)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"This model does not support assistant message prefill. The conversation must end with a user message."}}`)
			return
		}
		require.Equal(t, "user", role)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, cloudCodeResponse(geminiUnaryResponse("OK")))
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model: "internal-opus-alias", Stream: true,
		Messages: []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{textPart("question")}}, {Role: core.RoleAssistant, Content: []core.ContentPart{textPart("partial")}}},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "token"})
	require.NoError(t, err)
	require.Equal(t, "OK", resp.Message.TextContent())
	require.Equal(t, 2, requests)
	require.True(t, req.Stream, "Chat must not mutate the caller's request")
}

func TestAntigravityClaudePrefillRetryDoesNotRepeatOtherBadRequests(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"different invalid request"}}`)
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:    "internal-opus-alias",
		Messages: []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{textPart("partial")}}},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "token"})
	require.Error(t, err)
	require.Equal(t, 1, requests)
}

func TestAntigravityClaudeUnknownAliasRetriesStreamOnce(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		role, _ := finalCloudCodeTurn(t, decodeCloudCodeRequestFromHTTP(t, r))
		if requests == 1 {
			require.Equal(t, "model", role)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"The conversation must end with a user message."}}`)
			return
		}
		require.Equal(t, "user", role)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", geminiStreamChunk("OK"))
		fmt.Fprintf(w, "data: %s\n\n", geminiStreamStop())
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:    "internal-opus-alias",
		Messages: []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{textPart("partial")}}},
	}
	stream, err := c.Stream(context.Background(), req, core.Credentials{AccessToken: "token"}, core.StreamConfig{})
	require.NoError(t, err)
	var output strings.Builder
	for chunk := range stream {
		if chunk.Type == core.ChunkText {
			output.WriteString(chunk.Delta)
		}
		require.NotEqual(t, core.ChunkError, chunk.Type)
	}
	require.Equal(t, "OK", output.String())
	require.Equal(t, 2, requests)
	require.False(t, req.Stream, "Stream must not mutate the caller's request")
}

func TestAntigravityClaudePrefillStreamRetryForcesToolResultContinuation(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		role, parts := finalCloudCodeTurn(t, decodeCloudCodeRequestFromHTTP(t, r))
		require.Equal(t, "user", role)
		if requests == 1 {
			require.False(t, cloudCodeTurnHasText(parts, antigravityClaudeContinuation))
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"This model does not support assistant message prefill. The conversation must end with a user message."}}`)
			return
		}
		require.True(t, cloudCodeTurnHasText(parts, antigravityClaudeContinuation))
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", geminiStreamChunk("OK"))
		fmt.Fprintf(w, "data: %s\n\n", geminiStreamStop())
	}))
	defer srv.Close()

	toolCall := core.ContentPart{
		Type: core.PartToolCall,
		ToolCall: &core.ToolCall{
			ID:        "call_1",
			Name:      "lookup",
			Arguments: json.RawMessage(`{"q":"test"}`),
		},
	}
	toolResult := core.ContentPart{
		Type:       core.PartToolResult,
		ToolResult: &core.ToolResult{CallID: "call_1", Content: "result"},
	}
	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model: "claude-opus-4-6-thinking",
		Messages: []core.Message{
			{Role: core.RoleAssistant, Content: []core.ContentPart{toolCall}},
			{Role: core.RoleTool, Content: []core.ContentPart{toolResult}},
		},
	}
	stream, err := c.Stream(context.Background(), req, core.Credentials{AccessToken: "token"}, core.StreamConfig{})
	require.NoError(t, err)
	var output strings.Builder
	for chunk := range stream {
		if chunk.Type == core.ChunkText {
			output.WriteString(chunk.Delta)
		}
		require.NotEqual(t, core.ChunkError, chunk.Type)
	}
	require.Equal(t, "OK", output.String())
	require.Equal(t, 2, requests)
}

func TestAntigravityClaudePrefillStreamRetryIsBounded(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"This model does not support assistant message prefill."}}`)
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:    "internal-opus-alias",
		Messages: []core.Message{{Role: core.RoleAssistant, Content: []core.ContentPart{textPart("partial")}}},
	}
	_, err := c.Stream(context.Background(), req, core.Credentials{AccessToken: "token"}, core.StreamConfig{})
	require.Error(t, err)
	require.Equal(t, 2, requests)
}
