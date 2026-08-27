package connectors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

func TestCloudCodeStreamForwardsSSEKeepAliveAsPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": ping\n\n")
		fmt.Fprintf(w, "data: %s\n\n", geminiStreamChunk("OK"))
		fmt.Fprintf(w, "data: %s\n\n", geminiStreamStop())
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:    "claude-opus-4-6-thinking",
		Messages: []core.Message{{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "question"}}}},
	}
	stream, err := c.Stream(context.Background(), req, core.Credentials{AccessToken: "token"}, core.StreamConfig{})
	require.NoError(t, err)

	var sawPing, sawText, sawFinish bool
	for chunk := range stream {
		switch chunk.Type {
		case core.ChunkPing:
			sawPing = true
		case core.ChunkText:
			sawText = true
		case core.ChunkFinish:
			sawFinish = true
		}
	}
	require.True(t, sawPing)
	require.True(t, sawText)
	require.True(t, sawFinish)
}

func TestIsSSEKeepAlive(t *testing.T) {
	require.True(t, isSSEKeepAlive(": ping"))
	require.True(t, isSSEKeepAlive("  : keep-alive\r"))
	require.False(t, isSSEKeepAlive("data: {}"))
	require.False(t, isSSEKeepAlive("event: message"))
}
