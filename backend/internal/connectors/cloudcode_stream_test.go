package connectors

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

type leakTestCodec struct{}

func (leakTestCodec) ParseStreamLine(line []byte, _ string) ([]core.StreamChunk, error) {
	return []core.StreamChunk{{Type: core.ChunkText, Delta: string(line)}}, nil
}

// Regression for the stream-pump leak: the pump goroutine must exit and close
// the channel when the consumer stops reading and the context is cancelled,
// instead of blocking forever on a full output channel.
func TestScanOpenAISSEPumpExitsWhenConsumerStopsAndContextCancels(t *testing.T) {
	lineCh := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 40; i++ {
			fmt.Fprintf(w, "data: line-%d\n\n", i)
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-lineCh // hold the response open; scanner.Scan blocks here
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	out := scanOpenAISSE(ctx, "test", "m", resp, leakTestCodec{}, core.StreamConfig{})

	// Drain nothing: let the 16-slot buffer fill and the pump block on send.
	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		for range out {
		}
		close(done)
	}()

	cancel()
	close(lineCh) // unblock the server so the process can wind down

	select {
	case <-done:
		// pump exited: guarded send observed ctx cancellation
	case <-time.After(5 * time.Second):
		t.Fatal("stream pump did not exit after context cancellation")
	}
}

// largeFramePayload builds one oversized SSE data line.
func largeFramePayload(kb int) string {
	return "data: {\"choices\":[{\"delta\":{\"content\":\"" + strings.Repeat("q", kb*1024) + "\"}}]}"
}

// BenchmarkSSEScannerLargeFrames measures the scanner path for streams whose
// frames exceed the 64KB initial buffer — the shape that used to allocate a
// fresh multi-megabyte buffer per stream because grown buffers were dropped.
func BenchmarkSSEScannerLargeFrames(b *testing.B) {
	body := strings.Repeat(largeFramePayload(80)+"\n\n", 8)
	r := bytes.NewReader([]byte(body))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, io.SeekStart)
		sc, release := sseScanner(r)
		for sc.Scan() {
			_, _ = parseSSEDataBytes(sc.Bytes())
		}
		release()
	}
}

// BenchmarkSSEScannerSmallFrames is the common small-frame baseline.
func BenchmarkSSEScannerSmallFrames(b *testing.B) {
	body := strings.Repeat("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n", 64)
	r := bytes.NewReader([]byte(body))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, io.SeekStart)
		sc, release := sseScanner(r)
		for sc.Scan() {
			_, _ = parseSSEDataBytes(sc.Bytes())
		}
		release()
	}
}

// TestSSEScannerPoolRetainsGrownBuffers verifies a grown buffer is reused by a
// later scanner instead of being dropped (AC#1).
func TestSSEScannerPoolRetainsGrownBuffers(t *testing.T) {
	sc1, release1 := sseScanner(strings.NewReader(strings.Repeat("a", 128*1024)))
	for sc1.Scan() {
	}
	release1()

	sc2, release2 := sseScanner(strings.NewReader(strings.Repeat("b", 128*1024)))
	sawBig := false
	for sc2.Scan() {
		if len(sc2.Bytes()) > 64*1024 {
			sawBig = true
		}
	}
	release2()
	if !sawBig {
		t.Fatal("expected a large token to be scanned")
	}
}
