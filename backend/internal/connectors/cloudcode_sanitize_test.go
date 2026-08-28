package connectors

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

func TestAntigravityStripsCompetitiveSystemPrompt(t *testing.T) {
	c := NewAntigravity("antigravity", "https://example.com")
	req := &core.ChatRequest{
		Model:  "gemini-3.1-pro-high",
		System: antigravityCompetitivePrompt + "\nKeep this useful instruction.",
		Messages: []core.Message{{
			Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}},
		}},
	}

	body, err := c.wrapRequest(req, core.Credentials{AccessToken: "token"}, "")
	require.NoError(t, err)
	require.NotContains(t, string(body), antigravityCompetitivePrompt)
	require.Contains(t, string(body), "Keep this useful instruction.")

	var envelope struct {
		Request map[string]any `json:"request"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	system := envelope.Request["systemInstruction"].(map[string]any)
	parts := system["parts"].([]any)
	require.Len(t, parts, 1)
	require.Equal(t, "Keep this useful instruction.", strings.TrimSpace(parts[0].(map[string]any)["text"].(string)))
}

func TestAntigravityDropsEmptyCompetitiveSystemInstruction(t *testing.T) {
	inner := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []any{map[string]any{"text": antigravityCompetitivePrompt}},
		},
	}

	require.True(t, stripAntigravityCompetitivePrompt(inner))
	require.NotContains(t, inner, "systemInstruction")
}
