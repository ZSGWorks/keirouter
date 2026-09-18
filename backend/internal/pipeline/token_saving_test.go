package pipeline

import (
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func TestContentPartCountDetectsCompressionRemoval(t *testing.T) {
	before := []core.Message{{
		Role: core.RoleAssistant,
		Content: []core.ContentPart{
			{Type: core.PartText, Text: "text"},
			{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "call"}},
		},
	}}

	if got := contentPartCount(before); got != 2 {
		t.Fatalf("part count = %d, want 2", got)
	}
	if compressionRemovedParts(contentPartCount(before), before) {
		t.Fatal("unchanged compression must not trigger final normalization")
	}

	after := []core.Message{{
		Role:    core.RoleAssistant,
		Content: before[0].Content[:1],
	}}
	if !compressionRemovedParts(contentPartCount(before), after) {
		t.Fatal("removed part must trigger final normalization")
	}
}

func TestDrainStreamCombinesTextAndThinking(t *testing.T) {
	stream := make(chan core.StreamChunk, 4)
	stream <- core.StreamChunk{Type: core.ChunkThinking, Delta: "reason "}
	stream <- core.StreamChunk{Type: core.ChunkText, Delta: "hello "}
	stream <- core.StreamChunk{Type: core.ChunkThinking, Delta: "through"}
	stream <- core.StreamChunk{Type: core.ChunkText, Delta: "world"}
	close(stream)

	response, err := drainStream(stream, "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Message.Content) != 2 {
		t.Fatalf("content parts = %d, want 2", len(response.Message.Content))
	}
	if got := response.Message.Content[0].Text; got != "reason through" {
		t.Fatalf("thinking = %q", got)
	}
	if got := response.Message.Content[1].Text; got != "hello world" {
		t.Fatalf("text = %q", got)
	}
}
