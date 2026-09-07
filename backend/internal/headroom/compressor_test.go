package headroom

import (
	"context"
	"net/http"
	"net/http/httptest"
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
