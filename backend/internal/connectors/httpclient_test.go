package connectors

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEvictStaleProxyTransportClosesIdleConnections(t *testing.T) {
	states := make(chan http.ConnState, 2)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateIdle || state == http.StateClosed {
			states <- state
		}
	}
	server.Start()
	defer server.Close()

	transport := &http.Transport{}
	client := &http.Client{Transport: transport}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	waitForConnState(t, states, http.StateIdle)

	entry := &proxyTransportCacheEntry{transport: transport}
	entry.lastUsed.Store(time.Now().Add(-proxyTransportCacheTTL - time.Second).UnixNano())
	const key = "stale-proxy"
	proxyTransportCache.Store(key, entry)
	t.Cleanup(func() {
		proxyTransportCache.Delete(key)
		transport.CloseIdleConnections()
	})

	evictStaleProxyTransports(time.Now())
	if _, ok := proxyTransportCache.Load(key); ok {
		t.Fatal("stale proxy transport remains cached")
	}
	waitForConnState(t, states, http.StateClosed)
}

func waitForConnState(t *testing.T, states <-chan http.ConnState, wanted http.ConnState) {
	t.Helper()
	select {
	case state := <-states:
		if state != wanted {
			t.Fatalf("connection state = %v, want %v", state, wanted)
		}
	case <-time.After(time.Second):
		t.Fatalf("connection did not reach %v", wanted)
	}
}
