package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/crypto"
	"github.com/mydisha/keirouter/backend/internal/identity"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/vault"
)

type testLiveModelSource struct{}

func (testLiveModelSource) ListModels(context.Context, core.Credentials) ([]connectors.ModelSpec, error) {
	return []connectors.ModelSpec{{ID: "live-local-model", Name: "Live Local Model", Kind: core.ServiceLLM}}, nil
}

type failingLiveModelSource struct{}

func (failingLiveModelSource) ListModels(context.Context, core.Credentials) ([]connectors.ModelSpec, error) {
	return nil, errors.New("upstream unavailable")
}

type blockingLiveModelSource struct {
	started chan struct{}
	release <-chan struct{}
	active  atomic.Int32
	max     atomic.Int32
}

type modelPresenceExpectation struct {
	paths []string
	ids   []string
}

func (s *blockingLiveModelSource) ListModels(context.Context, core.Credentials) ([]connectors.ModelSpec, error) {
	active := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		max := s.max.Load()
		if active <= max || s.max.CompareAndSwap(max, active) {
			break
		}
	}
	s.started <- struct{}{}
	<-s.release
	return nil, nil
}

func TestListModelsOnlyShowsConnectedProviders(t *testing.T) {
	gw, apiKey := newModelDiscoveryTestGateway(t, []store.Account{
		modelDiscoveryAccount("acc-openai", "openai", false, false),
		modelDiscoveryAccount("acc-anthropic-disabled", "anthropic", true, false),
		modelDiscoveryAccount("acc-gemini-reconnect", "gemini", false, true),
	})

	body := getAuthedJSON(t, gw, apiKey, "/v1/models")
	models := modelIDsFromResponse(t, body)

	require.NotEmpty(t, models)
	require.Contains(t, models, "openai/gpt-4o")
	require.NotContains(t, models, "anthropic/claude-sonnet-4-20250514")
	require.NotContains(t, models, "gemini/gemini-2.5-pro")
	for _, id := range models {
		if strings.Contains(id, "/") {
			require.Truef(t, strings.HasPrefix(id, "openai/"), "unexpected unconnected provider model %q", id)
		}
	}
}

func TestListModelsByKindOnlyShowsConnectedProviders(t *testing.T) {
	gw, apiKey := newModelDiscoveryTestGateway(t, []store.Account{
		modelDiscoveryAccount("acc-openai", "openai", false, false),
	})

	body := getAuthedJSON(t, gw, apiKey, "/v1/models/embedding")
	models := modelIDsFromResponse(t, body)

	require.Contains(t, models, "openai/text-embedding-3-small")
	for _, id := range models {
		require.Truef(t, strings.HasPrefix(id, "openai/"), "unexpected unconnected provider model %q", id)
	}
}

func TestModelInfoOnlyShowsConnectedProviders(t *testing.T) {
	gw, apiKey := newModelDiscoveryTestGateway(t, []store.Account{
		modelDiscoveryAccount("acc-openai", "openai", false, false),
	})

	body := getAuthedJSON(t, gw, apiKey, "/v1/models/info?id=openai/gpt-4o")
	require.Equal(t, "openai", body["provider"])
	require.Equal(t, "gpt-4o", body["model"])

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models/info?id=anthropic/claude-sonnet-4-20250514", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	gw.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

func TestListModelsStillShowsChains(t *testing.T) {
	gw, apiKey := newModelDiscoveryTestGateway(t, nil)
	require.NoError(t, gw.chains.Create(context.Background(), store.Chain{
		ID:       "chain-fast",
		TenantID: store.DefaultTenantID,
		Name:     "fast",
		Strategy: "fallback",
		Steps: []store.ChainStep{{
			Position: 0,
			Provider: "openai",
			Model:    "gpt-4o",
		}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	body := getAuthedJSON(t, gw, apiKey, "/v1/models")
	models := modelIDsFromResponse(t, body)

	require.Equal(t, []string{"fast"}, models)
}

func TestListModelsHandlesLiveOllamaDiscovery(t *testing.T) {
	tests := []struct {
		name       string
		source     connectors.LiveModelSource
		expect     modelPresenceExpectation
		excludeIDs map[string][]string
	}{
		{
			name:   "includes live models",
			source: testLiveModelSource{},
			expect: modelPresenceExpectation{
				paths: []string{"/v1/models", "/v1/models/llm"},
				ids:   []string{"ollama-local/live-local-model", "ollama-local/llama3.2"},
			},
			excludeIDs: map[string][]string{
				"/v1/models/embedding": {"ollama-local/live-local-model"},
			},
		},
		{
			name:   "keeps static models after live failure",
			source: failingLiveModelSource{},
			expect: modelPresenceExpectation{
				paths: []string{"/v1/models"},
				ids:   []string{"ollama-local/llama3.2"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connectors.DefaultRegistry()
			connectors.RegisterLiveModelSource("ollama-local", test.source)

			gw, apiKey := newModelDiscoveryTestGateway(t, []store.Account{
				modelDiscoveryAccount("acc-ollama-local", "ollama-local", false, false),
			})
			requireModelsPresent(t, gw, apiKey, test.expect)
			requireModelsExcluded(t, gw, apiKey, test.excludeIDs)
		})
	}
}

func TestFetchLiveModelsLimitsConcurrentProbes(t *testing.T) {
	gw, source, release := newLiveProbeTestGateway(t)
	done := make(chan struct{})
	go func() {
		gw.fetchLiveModels(context.Background(), store.DefaultTenantID)
		close(done)
	}()

	waitForLiveProbes(t, source.started, 4)
	requireNoAdditionalLiveProbe(t, source.started)
	close(release)
	waitForLiveModelFetch(t, done)
	require.LessOrEqual(t, source.max.Load(), int32(4))
}

func TestFetchLiveModelsReturnsAfterCancellation(t *testing.T) {
	connectors.DefaultRegistry()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	source := &blockingLiveModelSource{started: started, release: release}
	connectors.RegisterLiveModelSource("ollama-local", source)
	gw, _ := newModelDiscoveryTestGateway(t, []store.Account{
		modelDiscoveryAccount("acc-ollama-local", "ollama-local", false, false),
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		gw.fetchLiveModels(ctx, store.DefaultTenantID)
		close(done)
	}()
	waitForLiveProbes(t, started, 1)
	cancel()
	waitForLiveModelFetch(t, done)
}

func requireModelsPresent(t *testing.T, gw *Server, apiKey string, expectation modelPresenceExpectation) {
	t.Helper()
	for _, path := range expectation.paths {
		models := modelIDsFromResponse(t, getAuthedJSON(t, gw, apiKey, path))
		for _, id := range expectation.ids {
			require.Contains(t, models, id)
		}
	}
}

func requireModelsExcluded(t *testing.T, gw *Server, apiKey string, paths map[string][]string) {
	t.Helper()
	for path, ids := range paths {
		models := modelIDsFromResponse(t, getAuthedJSON(t, gw, apiKey, path))
		for _, id := range ids {
			require.NotContains(t, models, id)
		}
	}
}

func newLiveProbeTestGateway(t *testing.T) (*Server, *blockingLiveModelSource, chan struct{}) {
	t.Helper()
	connectors.DefaultRegistry()
	started := make(chan struct{}, 5)
	release := make(chan struct{})
	source := &blockingLiveModelSource{started: started, release: release}
	accounts := make([]store.Account, 0, 5)
	seen := map[string]struct{}{}
	for _, spec := range connectors.Catalog() {
		if _, ok := seen[spec.ID]; ok {
			continue
		}
		seen[spec.ID] = struct{}{}
		accounts = append(accounts, modelDiscoveryAccount("acc-"+spec.ID, spec.ID, false, false))
		connectors.RegisterLiveModelSource(spec.ID, source)
		if len(accounts) == 5 {
			break
		}
	}
	require.Len(t, accounts, 5)

	gw, _ := newModelDiscoveryTestGateway(t, accounts)
	return gw, source, release
}

func waitForLiveProbes(t *testing.T, started <-chan struct{}, count int) {
	t.Helper()
	for range count {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for a live model probe")
		}
	}
}

func requireNoAdditionalLiveProbe(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
		t.Fatal("started more than four live model probes")
	case <-time.After(100 * time.Millisecond):
	}
}

func waitForLiveModelFetch(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("live model probes did not finish")
	}
}

func newModelDiscoveryTestGateway(t *testing.T, accounts []store.Account) (*Server, string) {
	t.Helper()
	ctx := context.Background()

	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Tenants().EnsureDefault(ctx))
	t.Cleanup(func() { _ = db.Close() })

	mk, err := crypto.GenerateMasterKey()
	require.NoError(t, err)
	sealer, err := crypto.NewSealer(mk)
	require.NoError(t, err)
	v := vault.New(sealer)

	for _, acc := range accounts {
		require.NoError(t, v.Seal(&acc, vault.NewSecret{APIKey: "sk-test"}))
		require.NoError(t, db.Accounts().Create(ctx, acc))
	}

	idSvc := identity.New(db.APIKeys())
	issued, err := idSvc.Create(ctx, store.DefaultTenantID, "", "test-key")
	require.NoError(t, err)

	gw := New(Deps{
		Config:   config.Default(),
		Identity: idSvc,
		Chains:   db.Chains(),
		Accounts: db.Accounts(),
		Vault:    v,
	})
	return gw, issued.Plaintext
}

func modelDiscoveryAccount(id, provider string, disabled, needsReconnect bool) store.Account {
	now := time.Now()
	return store.Account{
		ID:             id,
		TenantID:       store.DefaultTenantID,
		Provider:       provider,
		Label:          id,
		AuthKind:       store.AuthAPIKey,
		Priority:       10,
		Disabled:       disabled,
		NeedsReconnect: needsReconnect,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func getAuthedJSON(t *testing.T, gw *Server, apiKey, path string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	gw.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func modelIDsFromResponse(t *testing.T, body map[string]any) []string {
	t.Helper()
	items, ok := body["data"].([]any)
	require.True(t, ok)
	out := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		require.True(t, ok)
		id, ok := m["id"].(string)
		require.True(t, ok)
		out = append(out, id)
	}
	return out
}
