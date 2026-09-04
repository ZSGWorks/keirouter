package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/stretchr/testify/require"
)

func TestProviderModelsIncludesResolvedPricing(t *testing.T) {
	s, _ := newCustomProviderTestServer(t)
	rec := httptest.NewRecorder()
	s.adminProviderModels(rec, withChiID(http.MethodGet, "/providers/openai/models", "openai"))
	require.Equal(t, http.StatusOK, rec.Code)

	var response struct {
		Models []struct {
			ID      string                     `json:"id"`
			Pricing map[string]json.RawMessage `json:"pricing"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	for _, model := range response.Models {
		if model.ID == "gpt-4o" {
			require.NotNil(t, model.Pricing)
			for _, field := range []string{
				"input_per_m", "output_per_m", "cached_input_per_m", "cache_write_per_m",
				"reasoning_per_m", "long_context_threshold", "long_input_per_m",
				"long_output_per_m", "long_cached_input_per_m", "long_cache_write_per_m",
				"source", "source_url", "estimated", "explicit_free",
			} {
				require.Contains(t, model.Pricing, field)
			}
			var inputPerM float64
			require.NoError(t, json.Unmarshal(model.Pricing["input_per_m"], &inputPerM))
			require.Positive(t, inputPerM)
			var source string
			require.NoError(t, json.Unmarshal(model.Pricing["source"], &source))
			require.Equal(t, "provider_catalog", source)
			return
		}
	}
	t.Fatal("openai/gpt-4o missing from provider model response")
}

func TestProviderAccountMetadataSpecialProviders(t *testing.T) {
	cf, ok := connectors.SpecByID("cloudflare-ai")
	require.True(t, ok)
	_, err := providerAccountMetadata(cf, providerMetadataInput{})
	require.Error(t, err)

	meta, err := providerAccountMetadata(cf, providerMetadataInput{AccountID: "acct-123"})
	require.NoError(t, err)
	require.Equal(t, "acct-123", meta["accountId"])

	azure, ok := connectors.SpecByID("azure")
	require.True(t, ok)
	meta, err = providerAccountMetadata(azure, providerMetadataInput{
		AzureEndpoint:   "https://example.openai.azure.com/",
		AzureDeployment: "prod-gpt",
		AzureAPIVersion: "2024-10-01-preview",
	})
	require.NoError(t, err)
	require.Equal(t, "https://example.openai.azure.com", meta["azure_endpoint"])
	require.Equal(t, "prod-gpt", meta["deployment"])
	require.Equal(t, "2024-10-01-preview", meta["api_version"])

	custom, ok := connectors.SpecByID("custom-openai")
	require.True(t, ok)
	_, err = providerAccountMetadata(custom, providerMetadataInput{})
	require.Error(t, err)
	meta, err = providerAccountMetadata(custom, providerMetadataInput{BaseURL: "https://llm.example.com/v1"})
	require.NoError(t, err)
	require.Equal(t, "https://llm.example.com/v1", meta["base_url"])
}

func TestAccountAuthKindNoAuthProvider(t *testing.T) {
	spec, ok := connectors.SpecByID("searxng")
	require.True(t, ok)
	require.Equal(t, store.AuthNone, accountAuthKind(spec, ""))
	require.Equal(t, store.AuthAPIKey, accountAuthKind(spec, "optional-key"))
}
