package health

import (
	"testing"
	"time"
)

// TestChainStats_Counting verifies the terminal-event counting model: a
// request through a chain emits one terminal event (success or final-failure)
// plus zero or more non-terminal fallback-failure events. Only terminal
// events count as requests; FallbackTriggered on a terminal event means the
// request fell back >=1 time.
func TestChainStats_Counting(t *testing.T) {
	svc := New(Config{Enabled: true, RollingWindow: time.Hour}, nil, nil)
	defer svc.Close(time.Second)

	now := time.Now()
	// Chain "coding": 2 successful requests, 1 request that fell back twice
	// then succeeded, 1 request that failed all attempts.
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "success"})
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "success"})
	// request 3: two non-terminal fallback failures then success (fellBack=true)
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "failed", FallbackTriggered: true})
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "failed", FallbackTriggered: true})
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "success", FallbackTriggered: true})
	// request 4: one non-terminal fallback failure then final failure (fellBack=true)
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "failed", FallbackTriggered: true})
	svc.ingest(ProviderTelemetryEvent{Timestamp: now, ChainID: "coding", Status: "failed", FinalFailure: true, FallbackTriggered: true})

	stats := svc.ChainStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(stats))
	}
	st := stats[0]
	if st.ChainID != "coding" {
		t.Fatalf("chain id = %s", st.ChainID)
	}
	if st.Requests != 4 {
		t.Errorf("requests = %d, want 4", st.Requests)
	}
	if st.Successes != 3 {
		t.Errorf("successes = %d, want 3", st.Successes)
	}
	if st.FinalFailures != 1 {
		t.Errorf("final failures = %d, want 1", st.FinalFailures)
	}
	if st.Fallbacks != 2 {
		t.Errorf("fallbacks = %d, want 2", st.Fallbacks)
	}
	wantRate := 2.0 / 4.0
	if st.FallbackRate-wantRate > 1e-9 || wantRate-st.FallbackRate > 1e-9 {
		t.Errorf("fallback rate = %v, want %v", st.FallbackRate, wantRate)
	}
}

// TestChainStatsSince_Window verifies ChainStatsSince scopes aggregation to the
// requested lookback: a 20-minute-old terminal event is counted at 1h but
// excluded at 5m, and ChainStats keeps using the rolling window.
func TestChainStatsSince_Window(t *testing.T) {
	svc := New(Config{
		Enabled:          true,
		RollingWindow:    5 * time.Minute,
		MaxHistoryWindow: time.Hour,
	}, nil, nil)
	defer svc.Close(time.Second)

	now := time.Now()
	svc.ingest(ProviderTelemetryEvent{Timestamp: now.Add(-20 * time.Minute), ChainID: "coding", Status: "success"})
	svc.ingest(ProviderTelemetryEvent{Timestamp: now.Add(-time.Minute), ChainID: "coding", Status: "success"})

	recent := svc.ChainStatsSince(5 * time.Minute)
	if len(recent) != 1 {
		t.Fatalf("5m: expected 1 chain, got %d", len(recent))
	}
	if recent[0].Requests != 1 {
		t.Errorf("5m requests = %d, want 1", recent[0].Requests)
	}

	full := svc.ChainStatsSince(time.Hour)
	if len(full) != 1 {
		t.Fatalf("1h: expected 1 chain, got %d", len(full))
	}
	if full[0].Requests != 2 {
		t.Errorf("1h requests = %d, want 2", full[0].Requests)
	}

	rolling := svc.ChainStats()
	if len(rolling) != 1 {
		t.Fatalf("rolling: expected 1 chain, got %d", len(rolling))
	}
	if rolling[0].Requests != 1 {
		t.Errorf("rolling requests = %d, want 1 (rolling window is 5m)", rolling[0].Requests)
	}
}
