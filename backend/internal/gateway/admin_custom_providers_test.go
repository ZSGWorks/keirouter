package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/capability"
	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/crypto"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/vault"
)

// newCustomProviderTestServer wires a minimal Server with an in-memory store
// and vault, sufficient to exercise the custom-provider delete handler.
func newCustomProviderTestServer(t *testing.T) (*Server, *store.DB) {
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

	s := &Server{
		db:       db,
		accounts: db.Accounts(),
		vault:    vault.New(sealer),
		log:      slog.Default(),
	}
	return s, db
}

// withChiID builds a request whose chi URLParam "id" is set, so handlers that
// call chi.URLParam(r, "id") work without mounting a full router.
func withChiID(method, target, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(ctx)
}

// deleteCustomProvider invokes the handler and returns status + decoded body.
func deleteCustomProvider(t *testing.T, s *Server, id string) (int, map[string]any) {
	t.Helper()
	req := withChiID(http.MethodDelete, "/custom-providers/"+id, id)
	rec := httptest.NewRecorder()
	s.adminDeleteCustomProvider(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return rec.Code, out
}

func TestDeleteCustomProvider_BuiltinRejected(t *testing.T) {
	s, _ := newCustomProviderTestServer(t)

	for _, id := range []string{"custom-openai", "custom-anthropic"} {
		code, body := deleteCustomProvider(t, s, id)
		require.Equal(t, http.StatusConflict, code, "id=%s body=%v", id, body)
		require.Nil(t, body["deleted"])
	}
}

func TestDeleteCustomProvider_NotFound(t *testing.T) {
	s, _ := newCustomProviderTestServer(t)

	code, body := deleteCustomProvider(t, s, "custom-openai-missing")
	require.Equal(t, http.StatusNotFound, code, body)

	// Sanity: the missing id never accidentally matches a dynamic provider.
	_, ok := connectors.DynamicProviderByID("custom-openai-missing")
	require.False(t, ok)
}

func TestDeleteCustomProvider_SuccessDisablesAccountsAndUnregisters(t *testing.T) {
	s, db := newCustomProviderTestServer(t)
	ctx := context.Background()

	const pid = "custom-openai-testinst"
	now := time.Now()

	// Persist a custom provider row + a custom model bound to it.
	require.NoError(t, db.CustomProviders().CreateProvider(ctx, store.CustomProvider{
		ID:          pid,
		TenantID:    store.DefaultTenantID,
		DisplayName: "Test Instance",
		Alias:       pid,
		Dialect:     string(core.DialectOpenAI),
		BaseURL:     "https://example.test/v1",
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	require.NoError(t, db.CustomProviders().CreateModel(ctx, store.CustomModel{
		ID:          "cm-1",
		TenantID:    store.DefaultTenantID,
		ProviderID:  pid,
		ModelID:     "gpt-test",
		DisplayName: "gpt-test",
		Kind:        string(core.ServiceLLM),
		Source:      "manual",
	}))

	// Register it live so the registry mirrors runtime state.
	connectors.RegisterDynamicProvider(connectors.DynamicProvider{
		ID: pid, DisplayName: "Test Instance", Alias: pid,
		Dialect: core.DialectOpenAI, BaseURL: "https://example.test/v1",
	})
	t.Cleanup(func() { connectors.UnregisterDynamicProvider(pid) })

	// Create two accounts bound to the provider: one enabled, one disabled.
	require.NoError(t, db.Accounts().Create(ctx, store.Account{
		ID: "acct-enabled", TenantID: store.DefaultTenantID, Provider: pid,
		Label: "enabled", AuthKind: store.AuthAPIKey, Priority: 10, Metadata: "{}",
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, db.Accounts().Create(ctx, store.Account{
		ID: "acct-already-off", TenantID: store.DefaultTenantID, Provider: pid,
		Label: "off", AuthKind: store.AuthAPIKey, Priority: 10, Metadata: "{}",
		Disabled:  true,
		CreatedAt: now, UpdatedAt: now,
	}))

	// Precondition: dynamic provider + an enabled account exist.
	_, ok := connectors.DynamicProviderByID(pid)
	require.True(t, ok)
	enabled, err := db.Accounts().ListByProvider(ctx, store.DefaultTenantID, pid)
	require.NoError(t, err)
	require.Len(t, enabled, 1)

	code, body := deleteCustomProvider(t, s, pid)
	require.Equal(t, http.StatusOK, code, body)
	require.Equal(t, true, body["deleted"])
	require.Equal(t, pid, body["id"])
	// accounts_disabled reflects total matched rows (both accounts).
	require.Equal(t, float64(2), body["accounts_disabled"])

	// In-memory dynamic provider + its models are gone.
	_, ok = connectors.DynamicProviderByID(pid)
	require.False(t, ok)

	// Provider row + its models are gone from the DB.
	_, err = db.CustomProviders().GetProvider(ctx, pid)
	require.ErrorIs(t, err, store.ErrNotFound)
	models, err := db.CustomProviders().ListModelsByProvider(ctx, pid)
	require.NoError(t, err)
	require.Empty(t, models)

	// No enabled accounts remain bound to the deleted provider.
	enabled, err = db.Accounts().ListByProvider(ctx, store.DefaultTenantID, pid)
	require.NoError(t, err)
	require.Empty(t, enabled, "all bound accounts must be disabled after delete")
}

func TestDeleteCustomProvider_RepoReturnsNotFoundForMissing(t *testing.T) {
	s, db := newCustomProviderTestServer(t)
	ctx := context.Background()

	// DeleteProvider itself surfaces ErrNotFound for an id that was never
	// inserted (defence-in-depth behind the handler's GetProvider probe).
	err := db.CustomProviders().DeleteProvider(ctx, "custom-openai-ghost")
	require.ErrorIs(t, err, store.ErrNotFound)

	// And the handler maps it to 404 when it slips past the probe.
	code, body := deleteCustomProvider(t, s, "custom-anthropic-ghost")
	require.Equal(t, http.StatusNotFound, code, body)
}

// TestCustomModelCapabilityRoundTrip covers the user-declared capability
// override end to end: create a custom model with capability ticks on an
// Ollama provider, verify they persist through the store and resolve through
// the capability registry (dispatch/stripping source of truth).
func TestCustomModelCapabilityRoundTrip(t *testing.T) {
	s, db := newCustomProviderTestServer(t)
	ctx := context.Background()

	const providerID = "ollama-local"

	// A text-only model whose id would otherwise match the *gemma* vision
	// pattern, plus a vision model with an id no heuristic knows.
	create := func(modelID string, caps map[string]bool) store.CustomModel {
		body, err := json.Marshal(map[string]any{
			"id": modelID, "kind": "llm", "capabilities": caps,
		})
		require.NoError(t, err)
		req := withChiID(http.MethodPost, "/providers/"+providerID+"/custom-models", providerID)
		req = httptest.NewRequest(http.MethodPost, "/providers/"+providerID+"/custom-models", bytes.NewReader(body)).WithContext(req.Context())
		rec := httptest.NewRecorder()
		s.adminCreateCustomModel(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code, "create %s: %s", modelID, rec.Body.String())
		var out struct {
			Capabilities map[string]bool `json:"capabilities"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		require.Equal(t, caps, out.Capabilities, "handler should echo the persisted capabilities")
		models, gerr := db.CustomProviders().ListManualModelsByProvider(ctx, providerID)
		require.NoError(t, gerr)
		for _, cm := range models {
			if cm.ModelID == modelID {
				return cm
			}
		}
		t.Fatalf("created model %s not found", modelID)
		return store.CustomModel{}
	}

	textOnly := create("gemma3:4b-test", map[string]bool{"vision": false})
	_ = create("my-vision-finetune", map[string]bool{"vision": true, "reasoning": true})

	// Unknown capability names are rejected.
	badReq := withChiID(http.MethodPost, "/providers/"+providerID+"/custom-models", providerID)
	badReq = httptest.NewRequest(http.MethodPost, "/providers/"+providerID+"/custom-models",
		bytes.NewReader([]byte(`{"id":"bad","kind":"llm","capabilities":{"telepathy":true}}`))).WithContext(badReq.Context())
	badRec := httptest.NewRecorder()
	s.adminCreateCustomModel(badRec, badReq)
	require.Equal(t, http.StatusBadRequest, badRec.Code)

	// Registry reflects the declarations through capability.Resolve, the
	// choke point for dispatch eligibility and modality stripping.
	res := capability.Resolve(providerID, "gemma3:4b-test")
	require.Equal(t, capability.SourceUser, res.Source)
	require.False(t, res.Profile.Vision, "stated vision=false must beat the *gemma* pattern")

	vision := capability.Resolve(providerID, "my-vision-finetune")
	require.Equal(t, capability.SourceUser, vision.Source)
	require.True(t, vision.Profile.Vision)
	require.True(t, vision.Profile.Reasoning)

	// Admin payload surfaces the declaration for the dashboard.
	listReq := withChiID(http.MethodGet, "/providers/"+providerID+"/custom-models", providerID)
	listRec := httptest.NewRecorder()
	s.adminListCustomModels(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)

	// Update clears the override; heuristic resolution returns.
	patchBody, err := json.Marshal(map[string]any{"capabilities": map[string]bool{}})
	require.NoError(t, err)
	patchRctx := chi.NewRouteContext()
	patchRctx.URLParams.Add("id", providerID)
	patchRctx.URLParams.Add("modelDBID", textOnly.ID)
	patchReq := httptest.NewRequest(http.MethodPatch, "/providers/"+providerID+"/custom-models/"+textOnly.ID,
		bytes.NewReader(patchBody)).WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, patchRctx))
	patchRec := httptest.NewRecorder()
	s.adminUpdateCustomModel(patchRec, patchReq)
	require.Equal(t, http.StatusOK, patchRec.Code, patchRec.Body.String())

	after, err := db.CustomProviders().GetModel(ctx, textOnly.ID)
	require.NoError(t, err)
	require.Empty(t, after.Capabilities)
	cleared := capability.Resolve(providerID, "gemma3:4b-test")
	require.NotEqual(t, capability.SourceUser, cleared.Source, "empty override should restore heuristics")
}
