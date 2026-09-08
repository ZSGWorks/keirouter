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

// TestAdminHealthChainsWindow asserts the chains health endpoint publishes
// the real rolling window rather than implying the requested range was
// applied. There is no persisted per-chain telemetry, so ChainStats always
// reflects RollingWindow; the window descriptor must say so truthfully.
func TestAdminHealthChainsWindow(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Tenants().EnsureDefault(ctx))
	t.Cleanup(func() { _ = db.Close() })

	s := &Server{
		db:             db,
		chains:         db.Chains(),
		providerHealth: health.New(health.Config{Enabled: true, RollingWindow: time.Hour}, nil, nil),
	}

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
	require.Equal(t, "rolling_current", resp.Window.Kind)
	require.Equal(t, int64(3600), resp.Window.DurationSeconds, "window must reflect configured RollingWindow, not requested range")
	require.Equal(t, "24h", resp.Window.RequestedRange)
	require.NotEmpty(t, resp.Window.Since, "since must be present when duration > 0")
}

// TestAdminHealthChainsWindow_NilProviderHealth asserts that when the health
// telemetry service is unavailable, the window is still present with a zero
// duration so dashboards can render a disabled label instead of "24h".
func TestAdminHealthChainsWindow_NilProviderHealth(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Tenants().EnsureDefault(ctx))
	t.Cleanup(func() { _ = db.Close() })

	s := &Server{
		db:     db,
		chains: db.Chains(),
	}

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
	require.Equal(t, "rolling_current", resp.Window.Kind)
	require.Zero(t, resp.Window.DurationSeconds, "window duration must be zero when provider health is disabled")
}
