package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/health"
	"github.com/mydisha/keirouter/backend/internal/store"
)

// newChainTestServer opens an in-memory store and wires a gateway Server with
// the given provider-health telemetry service.
func newChainTestServer(t *testing.T, providerHealth *health.Service) (*Server, *store.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Tenants().EnsureDefault(ctx))
	t.Cleanup(func() { _ = db.Close() })
	return &Server{
		db:             db,
		chains:         db.Chains(),
		providerHealth: providerHealth,
	}, db
}

// TestAdminHealthChainsWindow asserts the chains health endpoint applies the
// requested range (clamped to the telemetry service's retained history window)
// and reports that applied span truthfully.
func TestAdminHealthChainsWindow(t *testing.T) {
	s, _ := newChainTestServer(t, health.New(health.Config{Enabled: true, RollingWindow: time.Hour}, nil, nil))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/chains?range=24h", nil)
	s.adminHealthChains(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp struct {
		Chains []map[string]any `json:"chains"`
		Window struct {
			Kind            string `json:"kind"`
			DurationSeconds int64  `json:"duration_seconds"`
			RequestedRange  string `json:"requested_range"`
			Since           string `json:"since"`
		} `json:"window"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "in_memory_history", resp.Window.Kind)
	require.Equal(t, int64(86400), resp.Window.DurationSeconds, "window must reflect the applied range")
	require.Equal(t, "24h", resp.Window.RequestedRange)
	require.NotEmpty(t, resp.Window.Since, "since must be present when duration > 0")
}

// TestAdminHealthChainsWindow_ClampsToRetainedHistory asserts that ranges
// beyond the retained history window are clamped and reported as such.
func TestAdminHealthChainsWindow_ClampsToRetainedHistory(t *testing.T) {
	s, _ := newChainTestServer(t, health.New(health.Config{
		Enabled:          true,
		RollingWindow:    15 * time.Minute,
		MaxHistoryWindow: time.Hour,
	}, nil, nil))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/chains?range=7d", nil)
	s.adminHealthChains(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp struct {
		Window struct {
			Kind            string `json:"kind"`
			DurationSeconds int64  `json:"duration_seconds"`
			RequestedRange  string `json:"requested_range"`
		} `json:"window"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "in_memory_history", resp.Window.Kind)
	require.Equal(t, int64(3600), resp.Window.DurationSeconds, "7d must clamp to the retained history window")
	require.Equal(t, "7d", resp.Window.RequestedRange)
}

// TestAdminHealthChainsWindow_NilProviderHealth asserts that when the health
// telemetry service is unavailable, the window is still present with a zero
// duration so dashboards can render a disabled label instead of "24h".
func TestAdminHealthChainsWindow_NilProviderHealth(t *testing.T) {
	s, _ := newChainTestServer(t, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/chains?range=24h", nil)
	s.adminHealthChains(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp struct {
		Window struct {
			Kind            string `json:"kind"`
			DurationSeconds int64  `json:"duration_seconds"`
		} `json:"window"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "unavailable", resp.Window.Kind)
	require.Zero(t, resp.Window.DurationSeconds, "window duration must be zero when provider health is disabled")
}

// TestAdminHealthChainsRangeChangesCounts asserts that the ?range= selector
// actually changes the reported chain numbers: a 20-minute-old terminal event
// is visible at 1h but not at 5m.
func TestAdminHealthChainsRangeChangesCounts(t *testing.T) {
	ctx := context.Background()
	svc := health.New(health.Config{
		Enabled:          true,
		RollingWindow:    15 * time.Minute,
		MaxHistoryWindow: 24 * time.Hour,
	}, nil, nil)
	svc.Start(ctx)
	t.Cleanup(func() { svc.Close(time.Second) })

	s, db := newChainTestServer(t, svc)
	require.NoError(t, db.Chains().Create(ctx, store.Chain{
		ID: "chain-1", TenantID: adminTenant, Name: "primary", Strategy: "failover",
		Steps: []store.ChainStep{{ID: "s1", ChainID: "chain-1", Position: 0, Provider: "openai", Model: "gpt-4o"}},
	}))

	svc.Record(health.ProviderTelemetryEvent{
		Timestamp: time.Now().Add(-20 * time.Minute),
		ChainID:   "chain-1",
		Status:    "success",
	})

	requests := func(rangeVal string) int64 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/health/chains?range="+rangeVal, nil)
		s.adminHealthChains(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
		var resp struct {
			Chains []struct {
				ChainID  string `json:"chain_id"`
				Requests int64  `json:"requests"`
			} `json:"chains"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		for _, c := range resp.Chains {
			if c.ChainID == "chain-1" {
				return c.Requests
			}
		}
		return 0
	}

	require.Eventually(t, func() bool { return requests("1h") == 1 }, 2*time.Second, 20*time.Millisecond,
		"20m-old event must appear in the 1h window once recorded")
	require.Equal(t, int64(0), requests("5m"), "20m-old event must not appear in the 5m window")
}
