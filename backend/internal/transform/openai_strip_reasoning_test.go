package transform

import (
	"encoding/json"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

// TestOpenAI_RenderRequest_StripsReasoningForGroq verifies that non-reasoning
// providers which reject echoed reasoning_content (groq) receive assistant
// history with real chain-of-thought blocks stripped.
func TestOpenAI_RenderRequest_StripsReasoningForGroq(t *testing.T) {
	req := &core.ChatRequest{
		Model: "qwen/qwen3.6-27b",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartThinking, Text: "a genuine multi-sentence deliberation block"},
				{Type: core.PartText, Text: "hello"},
			}},
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "continue"}}},
		},
	}
	body, err := OpenAICodec{}.RenderRequestForProvider(req, "groq")
	require.NoError(t, err)

	var got oaiRequest
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "assistant", got.Messages[1].Role)
	require.Empty(t, got.Messages[1].ReasoningContent)
}

// TestOpenAI_RenderRequest_KeepsShortPlaceholderForGroq verifies the minimal
// validation placeholder survives stripping so upstreams that require the
// field's presence keep working.
func TestOpenAI_RenderRequest_KeepsShortPlaceholderForGroq(t *testing.T) {
	req := &core.ChatRequest{
		Model: "qwen/qwen3.6-27b",
		Messages: []core.Message{
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartThinking, Text: " "},
				{Type: core.PartText, Text: "hello"},
			}},
		},
	}
	body, err := OpenAICodec{}.RenderRequestForProvider(req, "groq")
	require.NoError(t, err)

	var got oaiRequest
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, reasoningPlaceholder, got.Messages[0].ReasoningContent)
}

func TestStripReasoningFor(t *testing.T) {
	require.True(t, stripReasoningFor("kimchi"))
	require.True(t, stripReasoningFor("groq"))
	require.False(t, stripReasoningFor("deepseek"))
	require.False(t, stripReasoningFor("openai"))
}
