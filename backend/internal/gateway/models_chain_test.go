package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/store"
)

func newChainModelTestGateway(t *testing.T) (*Server, string, context.Context) {
	t.Helper()
	gw, apiKey := newModelDiscoveryTestGateway(t, nil)
	return gw, apiKey, context.Background()
}

func createTestChain(t *testing.T, gw *Server, ctx context.Context, name string, steps []store.ChainStep) {
	t.Helper()
	now := time.Now()
	chain := store.Chain{
		ID:        "chain-" + name,
		TenantID:  store.DefaultTenantID,
		Name:      name,
		Strategy:  "fallback",
		Steps:     steps,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for i := range chain.Steps {
		chain.Steps[i].ChainID = chain.ID
		chain.Steps[i].Position = i
		chain.Steps[i].CreatedAt = now
		chain.Steps[i].ID = chain.ID + "-step-" + chain.Steps[i].Model
	}
	require.NoError(t, gw.chains.Create(ctx, chain))
}

func fetchChainModelEntry(t *testing.T, gw *Server, ctx context.Context, apiKey, chainName string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	gw.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	for _, entry := range body.Data {
		if entry["id"] == chainName {
			return entry
		}
	}
	t.Fatalf("chain model %q not found in /v1/models response", chainName)
	return nil
}

func TestChainModelAdvertisesVisionFromStep(t *testing.T) {
	gw, apiKey, ctx := newChainModelTestGateway(t)
	createTestChain(t, gw, ctx, "vision-chain", []store.ChainStep{
		{Provider: "glm", Model: "glm-5"},
		{Provider: "glm", Model: "glm-5.3-flash"},
	})

	entry := fetchChainModelEntry(t, gw, ctx, apiKey, "vision-chain")
	caps, ok := entry["capabilities"].(map[string]any)
	require.True(t, ok, "chain entry must carry capabilities: %v", entry)
	require.Equal(t, true, caps["vision"], "chain containing glm-5.3-flash must advertise vision")
	require.Equal(t, "chain", entry["capability_source"])

	inMods, ok := entry["input_modalities"].([]any)
	require.True(t, ok, "chain entry must carry input_modalities: %v", entry)
	require.Contains(t, inMods, "image")
	require.Contains(t, inMods, "text")
}

func TestChainModelTextOnlyNoVision(t *testing.T) {
	gw, apiKey, ctx := newChainModelTestGateway(t)
	createTestChain(t, gw, ctx, "text-chain", []store.ChainStep{
		{Provider: "glm", Model: "glm-5"},
		{Provider: "deepseek", Model: "deepseek-chat"},
	})

	entry := fetchChainModelEntry(t, gw, ctx, apiKey, "text-chain")
	caps, ok := entry["capabilities"].(map[string]any)
	require.True(t, ok, "chain entry must carry capabilities: %v", entry)
	require.Equal(t, false, caps["vision"], "text-only chain must not advertise vision")

	inMods, ok := entry["input_modalities"].([]any)
	require.True(t, ok)
	require.NotContains(t, inMods, "image")
}

func TestChainModelLimitsAreMaxAcrossSteps(t *testing.T) {
	gw, apiKey, ctx := newChainModelTestGateway(t)
	createTestChain(t, gw, ctx, "limits-chain", []store.ChainStep{
		{Provider: "glm", Model: "glm-4.6v"},
		{Provider: "glm", Model: "glm-5"},
	})

	entry := fetchChainModelEntry(t, gw, ctx, apiKey, "limits-chain")
	caps := entry["capabilities"].(map[string]any)
	require.Equal(t, float64(200000), caps["context_window"])
	require.Equal(t, float64(128000), caps["max_output"])
}
