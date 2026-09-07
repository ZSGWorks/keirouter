package gateway

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/stretchr/testify/require"
)

func TestModelCacheGetSetExpire(t *testing.T) {
	cache := newModelCache()
	cache.ttl = 50 * time.Millisecond

	_, ok := cache.get("openai")
	require.False(t, ok)

	models := []providerModelInfo{{ID: "gpt-test", Name: "GPT Test"}}
	cache.set("openai", models)

	got, ok := cache.get("openai")
	require.True(t, ok)
	require.Equal(t, models, got)

	time.Sleep(60 * time.Millisecond)
	_, ok = cache.get("openai")
	require.False(t, ok)
}

func TestModelCacheInvalidate(t *testing.T) {
	cache := newModelCache()
	cache.set("openai", []providerModelInfo{{ID: "gpt-test"}})
	cache.invalidate("openai")
	_, ok := cache.get("openai")
	require.False(t, ok)

	// Invalidate is a no-op on unknown providers.
	cache.invalidate("unknown")
}

func TestModelCacheNilSafe(t *testing.T) {
	var cache *modelCache
	_, ok := cache.get("openai")
	require.False(t, ok)
	cache.set("openai", nil)
	cache.invalidate("openai")
}

// seedConnectedAccount creates an enabled account so the provider counts as
// connected for the admin model handlers.
func seedConnectedAccount(t *testing.T, s *Server, provider string) store.Account {
	t.Helper()
	acc := store.Account{
		ID:        "acc-" + provider,
		TenantID:  adminTenant,
		Provider:  provider,
		Label:     provider,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, s.accounts.Create(context.Background(), acc))
	t.Cleanup(func() { _ = s.accounts.Delete(context.Background(), acc.ID) })
	return acc
}

// TestProviderModelsServesFromCache verifies a connected provider's model
// listing is served from the cache once warmed, bypassing discovery.
func TestProviderModelsServesFromCache(t *testing.T) {
	s, _ := newCustomProviderTestServer(t)
	seedConnectedAccount(t, s, "openai")

	cached := []providerModelInfo{{ID: "cached-only-model", Name: "From Cache", Kind: "llm"}}
	s.providerModelCache().set("openai", cached)

	rec := httptest.NewRecorder()
	s.adminProviderModels(rec, withChiID("GET", "/providers/openai/models", "openai"))
	require.Equal(t, 200, rec.Code)

	var response struct {
		Models []providerModelInfo `json:"models"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response.Models, 1)
	require.Equal(t, "cached-only-model", response.Models[0].ID)
}

// TestProviderModelsUnconnectedBypassesCache verifies unconnected providers
// never serve stale cache entries.
func TestProviderModelsUnconnectedBypassesCache(t *testing.T) {
	s, _ := newCustomProviderTestServer(t)

	s.providerModelCache().set("openai", []providerModelInfo{{ID: "stale-entry", Name: "Stale"}})

	rec := httptest.NewRecorder()
	s.adminProviderModels(rec, withChiID("GET", "/providers/openai/models", "openai"))
	require.Equal(t, 200, rec.Code)

	var response struct {
		Models []providerModelInfo `json:"models"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	for _, model := range response.Models {
		require.NotEqual(t, "stale-entry", model.ID)
	}
}

// TestAdminListProvidersReportsConnected verifies the connected flag appears
// on the provider listing.
func TestAdminListProvidersReportsConnected(t *testing.T) {
	s, _ := newCustomProviderTestServer(t)
	seedConnectedAccount(t, s, "openai")

	rec := httptest.NewRecorder()
	s.adminListProviders(rec, httptest.NewRequest("GET", "/providers", nil))
	require.Equal(t, 200, rec.Code)

	var response struct {
		Providers []struct {
			ID        string `json:"id"`
			Connected bool   `json:"connected"`
		} `json:"providers"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	byID := map[string]bool{}
	for _, p := range response.Providers {
		byID[p.ID] = p.Connected
	}
	require.True(t, byID["openai"])
	require.False(t, byID["ollama"])
}
