package headroom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func testRequest() *core.ChatRequest {
	return &core.ChatRequest{
		Model: "gpt-4o",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello world"}}},
		},
	}
}

// TestCompress_RetriesTransientThenSucceeds verifies that a transient 503 is
// retried and a subsequent 200 yields a successful compression.
func TestCompress_RetriesTransientThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"role":"user","content":"hi"}],"tokens_before":10,"tokens_after":2,"tokens_saved":8}`))
	}))
	defer srv.Close()

	req := testRequest()
	stats := New(nil).Compress(context.Background(), req, Config{Enabled: true, URL: srv.URL, Timeout: 2 * time.Second})

	require.True(t, stats.Compressed, "should succeed on retry")
	require.Equal(t, int32(2), atomic.LoadInt32(&calls), "should retry once after 503")
	require.Len(t, req.Messages, 1)
	require.Equal(t, "hi", req.Messages[0].Content[0].Text)
	require.Equal(t, 10, stats.TokensBefore)
	require.Equal(t, 2, stats.TokensAfter)
	require.Equal(t, 8, stats.TokensSaved)
}

// TestCompress_PersistentTransientFailsOpen verifies that when every attempt
// returns 503, Compress exhausts its retries and fails open (request unchanged,
// no panic, empty stats).
func TestCompress_PersistentTransientFailsOpen(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	req := testRequest()
	before := req.Messages[0].Content[0].Text
	stats := New(nil).Compress(context.Background(), req, Config{Enabled: true, URL: srv.URL, Timeout: 2 * time.Second})

	require.False(t, stats.Compressed, "should fail open")
	require.Equal(t, before, req.Messages[0].Content[0].Text, "messages must be unchanged")
	require.Equal(t, int32(maxCompressAttempts), atomic.LoadInt32(&calls), "should try maxCompressAttempts times")
}

// TestCompress_ConcurrencyCapSkips verifies that when all concurrency slots are
// busy, Compress skips the proxy entirely (fail-open) instead of piling on.
func TestCompress_ConcurrencyCapSkips(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"role":"user","content":"hi"}]}`))
	}))
	defer srv.Close()

	c := New(nil)
	c.sem = make(chan struct{}, 1)
	c.sem <- struct{}{} // occupy the only slot

	req := testRequest()
	stats := c.Compress(context.Background(), req, Config{Enabled: true, URL: srv.URL, Timeout: 2 * time.Second})

	require.False(t, stats.Compressed, "should skip when concurrency cap is reached")
	require.Equal(t, int32(0), atomic.LoadInt32(&calls), "proxy must not be called when capped")
}

// (e.g. 400) fails open immediately without consuming retries.
func TestCompress_NonRetryableStatusNoRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	req := testRequest()
	stats := New(nil).Compress(context.Background(), req, Config{Enabled: true, URL: srv.URL, Timeout: 2 * time.Second})

	require.False(t, stats.Compressed)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls), "non-retryable status must not be retried")
}

// TestCompress_UnusableSuccessResponseFailsOpen ensures a 2xx response cannot
// replace caller messages unless it contains a usable compression result.
func TestCompress_UnusableSuccessResponseFailsOpen(t *testing.T) {
	cases := map[string]string{
		"malformed JSON":      `{"messages":`,
		"compression skipped": `{"messages":[{"role":"user","content":"compressed"}],"compression_skipped":true}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			req := testRequest()
			before := req.Messages[0].Content[0].Text
			stats := New(nil).Compress(context.Background(), req, Config{Enabled: true, URL: srv.URL, Timeout: 2 * time.Second})

			require.False(t, stats.Compressed)
			require.Equal(t, before, req.Messages[0].Content[0].Text)
			require.Zero(t, stats.TokensSaved)
		})
	}
}

func TestProbe_SkippedResponseIsNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"role":"user","content":"ping"}],"compression_skipped":true}`))
	}))
	defer srv.Close()

	result := New(nil).Probe(context.Background(), Config{URL: srv.URL, Timeout: time.Second})

	require.True(t, result.Reachable)
	require.False(t, result.OK)
	require.Contains(t, result.Message, "skipped")
}

// TestCompress_BytesMeasurementsUnchangedBySinglePassMapping verifies that
// building the OpenAI mapping once still yields the same bytesBefore /
// bytesAfter values as the previous triple-serialization behavior: mapping the
// proxy response back through the core request and re-mapping must be
// byte-identical to the raw response messages.
func TestCompress_BytesMeasurementsUnchangedBySinglePassMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"role":"user","content":"compressed text"}],"tokens_before":100,"tokens_after":20,"tokens_saved":80}`))
	}))
	defer srv.Close()

	req := testRequest()
	stats := New(nil).Compress(context.Background(), req, Config{Enabled: true, URL: srv.URL, Timeout: 2 * time.Second})
	require.True(t, stats.Compressed)
	require.Greater(t, stats.BytesBefore, 0)
	require.Greater(t, stats.BytesAfter, 0)

	// Byte-equivalence: re-deriving the measurement from the mutated request
	// must produce exactly the same number as recorded from resp.Messages.
	require.Equal(t, jsonBytes(toOpenAIMessages(req)), stats.BytesAfter)
}

// largeTestRequest builds a conversation big enough that mapping and
// serialization dominate the compression call.
func largeTestRequest() *core.ChatRequest {
	msgs := make([]core.Message, 0, 64)
	for i := 0; i < 64; i++ {
		msgs = append(msgs, core.Message{
			Role:    core.RoleUser,
			Content: []core.ContentPart{{Type: core.PartText, Text: strings.Repeat("lorem ipsum coding-agent context ", 64)}},
		})
	}
	return &core.ChatRequest{Model: "gpt-4o", Messages: msgs}
}

// BenchmarkCompressAllocation measures allocations of the compress call path
// (mapping + serialization) against an always-successful proxy. Run with
// -benchmem to track allocs/op when tuning the serialization path.
func BenchmarkCompressAllocation(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"role":"user","content":"c"}],"tokens_before":1,"tokens_after":1,"tokens_saved":0}`))
	}))
	defer srv.Close()

	c := New(nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := largeTestRequest()
		_ = c.Compress(context.Background(), req, Config{Enabled: true, URL: srv.URL, Timeout: 2 * time.Second})
	}
}

// BenchmarkToOpenAIMessagesMapping isolates the mapping step that used to run
// three times per request and now runs once.
func BenchmarkToOpenAIMessagesMapping(b *testing.B) {
	req := largeTestRequest()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = toOpenAIMessages(req)
	}
}
