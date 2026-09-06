package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/oauth"
	"github.com/mydisha/keirouter/backend/internal/transform"
)

const sensitiveUpstreamDetail = "https://provider.example/v1?access_token=secret-token"

func TestSanitizeProviderErrorRedactsUpstreamDetails(t *testing.T) {
	message := sanitizeProviderError(nil, errors.New("upstream failed: "+sensitiveUpstreamDetail), "test provider failure")

	require.Equal(t, "an internal error occurred", message)
	require.NotContains(t, message, sensitiveUpstreamDetail)
	require.NotContains(t, message, "secret-token")
}

func TestOAuthCallbackRedactsProviderErrorDetails(t *testing.T) {
	gw := New(Deps{Config: config.Default(), Codecs: transform.DefaultRegistry()})
	gw.oauthSessions.Put("test-state", &oauth.Session{Provider: "codex"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=test-state&error=access_denied&error_description="+sensitiveUpstreamDetail, nil)

	gw.oauthCallback(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "OAuth authorization was denied")
	require.NotContains(t, rec.Body.String(), sensitiveUpstreamDetail)
	require.NotContains(t, rec.Body.String(), "secret-token")
}

func TestOAuthCallbackStatusRedactsStoredProviderErrorDetails(t *testing.T) {
	gw := New(Deps{Config: config.Default(), Codecs: transform.DefaultRegistry()})
	recordOAuthResult("redaction-state", "codex", errors.New("token exchange failed: "+sensitiveUpstreamDetail))
	t.Cleanup(func() {
		oauthResults.Lock()
		delete(oauthResults.m, "redaction-state")
		oauthResults.Unlock()
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth/codex/callback-status?state=redaction-state", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("provider", "codex")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	gw.oauthCallbackStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "OAuth provider request failed")
	require.NotContains(t, rec.Body.String(), sensitiveUpstreamDetail)
	require.NotContains(t, rec.Body.String(), "secret-token")
}
