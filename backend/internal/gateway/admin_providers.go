package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mydisha/keirouter/backend/internal/capability"
	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/store"
)

// mountAdmin registers the dashboard admin endpoints on the given router. These
// manage API keys, provider accounts, routing chains, budgets, and usage.

func (s *Server) adminListProviders(w http.ResponseWriter, r *http.Request) {
	// Optional ?kind= filter restricts to providers serving a service kind.
	kindFilter := core.ServiceKind(r.URL.Query().Get("kind"))

	specs := connectors.Catalog()
	connected := s.connectedProviderSet(r.Context(), adminTenant)
	out := make([]map[string]any, 0, len(specs))
	for _, p := range specs {
		if kindFilter != "" && !core.HasServiceKind(p.ServiceKinds, kindFilter) {
			continue
		}
		kinds := p.ServiceKinds
		if len(kinds) == 0 {
			kinds = []core.ServiceKind{core.ServiceLLM}
		}
		entry := map[string]any{
			"id":            p.ID,
			"display_name":  p.DisplayName,
			"alias":         p.Alias,
			"dialect":       p.Dialect,
			"auth_kind":     p.AuthKind,
			"auth_modes":    p.AuthModesOf(),
			"service_kinds": kinds,
			"color":         p.Color,
			"website":       p.Website,
			"api_key_url":   p.APIKeyURL,
			"icon":          "/providers/" + providerIconID(p.ID) + ".png",
			"deprecated":    p.Deprecated,
			"hidden":        p.Hidden,
			"pinned":        p.Pinned,
			"notice":        p.Notice,
			"drivable":      connectors.DrivableDialect(p.Dialect) || webProvider(p.ID),
			"connected":     connected[p.ID],
			"input_per_m":   p.InputPerM,
			"output_per_m":  p.OutputPerM,
		}
		// Custom (user-defined) provider instances expose their configured base
		// URL so the dashboard can surface it on the provider detail page.
		if p.Custom {
			entry["custom"] = true
			entry["base_url"] = p.BaseURL
		}
		if len(p.Regions) > 0 {
			regions := make([]map[string]string, 0, len(p.Regions))
			for _, r := range p.Regions {
				regions = append(regions, map[string]string{
					"id":       r.ID,
					"label":    r.Label,
					"base_url": r.BaseURL,
				})
			}
			entry["regions"] = regions
			entry["default_region"] = p.DefaultRegion
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// providerIconID returns the provider ID used for the icon filename. Dynamic
// (user-created) custom providers inherit the logo of their base type so they
// show the OpenAI/Anthropic icon instead of a broken image or initials.
func providerIconID(id string) string {
	if strings.HasPrefix(id, connectors.CustomOpenAIPrefix) {
		return "custom-openai"
	}
	if strings.HasPrefix(id, connectors.CustomAnthropicPrefix) {
		return "custom-anthropic"
	}
	return id
}

// webProvider reports whether a provider is served by the web search/fetch
// connector (so it is routable even though its dialect is the generic openai).
func webProvider(id string) bool {
	switch id {
	case "tavily", "exa", "serper", "brave-search", "searxng", "firecrawl", "jina-reader":
		return true
	default:
		return false
	}
}

type providerModelPricing struct {
	InputPerM            float64 `json:"input_per_m"`
	OutputPerM           float64 `json:"output_per_m"`
	CachedInputPerM      float64 `json:"cached_input_per_m"`
	CacheWritePerM       float64 `json:"cache_write_per_m"`
	ReasoningPerM        float64 `json:"reasoning_per_m"`
	LongContextThreshold int     `json:"long_context_threshold"`
	LongInputPerM        float64 `json:"long_input_per_m"`
	LongOutputPerM       float64 `json:"long_output_per_m"`
	LongCachedInputPerM  float64 `json:"long_cached_input_per_m"`
	LongCacheWritePerM   float64 `json:"long_cache_write_per_m"`
	Source               string  `json:"source"`
	SourceURL            string  `json:"source_url"`
	Estimated            bool    `json:"estimated"`
	ExplicitFree         bool    `json:"explicit_free"`
}

type providerModelInfo struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	Kind             string                      `json:"kind"`
	Kinds            []string                    `json:"kinds,omitempty"`
	Capabilities     modelCapabilities           `json:"capabilities"`
	CapabilitySource capability.CapabilitySource `json:"capability_source"`
	Custom           bool                        `json:"custom,omitempty"`
	DBID             string                      `json:"db_id,omitempty"`
	Discovered       bool                        `json:"discovered,omitempty"`
	Pricing          *providerModelPricing       `json:"pricing,omitempty"`
}

type providerModelResponseOptions struct {
	ProviderID string
	KindFilter core.ServiceKind
	Price      connectors.ModelPrice
	PriceOK    bool
	Discovered bool
}

func providerModelPrice(price connectors.ModelPrice, ok bool) *providerModelPricing {
	if !ok {
		return nil
	}
	if price.Source == "" {
		price.Source = "provider_catalog"
	}
	return &providerModelPricing{
		InputPerM: price.InputPerM, OutputPerM: price.OutputPerM,
		CachedInputPerM: price.CachedInputPerM, CacheWritePerM: price.CacheWritePerM,
		ReasoningPerM: price.ReasoningPerM, LongContextThreshold: price.LongContextThreshold,
		LongInputPerM: price.LongInputPerM, LongOutputPerM: price.LongOutputPerM,
		LongCachedInputPerM: price.LongCachedInputPerM, LongCacheWritePerM: price.LongCacheWritePerM,
		Source: price.Source, SourceURL: price.SourceURL, Estimated: price.Estimated, ExplicitFree: price.ExplicitFree,
	}
}

func providerModelKind(kind core.ServiceKind) core.ServiceKind {
	if kind == "" {
		return core.ServiceLLM
	}
	return kind
}

func providerModelResponseKind(model connectors.ModelSpec, kindFilter core.ServiceKind) core.ServiceKind {
	if kindFilter != "" {
		return kindFilter
	}
	return providerModelKind(model.Kind)
}

func providerModelInfoFor(model connectors.ModelSpec, options providerModelResponseOptions) providerModelInfo {
	caps, source := capabilityPayloadForModel(options.ProviderID, model, options.KindFilter)
	return providerModelInfo{
		ID: model.ID, Name: model.Name, Kind: string(providerModelResponseKind(model, options.KindFilter)),
		Kinds: modelKindNames(model), Capabilities: caps, CapabilitySource: source,
		Discovered: options.Discovered, Pricing: providerModelPrice(options.Price, options.PriceOK),
	}
}

func (s *Server) providerCustomModels(ctx context.Context, providerID string) map[string]store.CustomModel {
	customByID := map[string]store.CustomModel{}
	if cms, err := s.db.CustomProviders().ListModelsByProvider(ctx, providerID); err == nil {
		for _, cm := range cms {
			customByID[cm.ModelID] = cm
		}
	}
	return customByID
}

func (s *Server) catalogProviderModels(ctx context.Context, providerID string, kindFilter core.ServiceKind) ([]providerModelInfo, map[string]bool) {
	customByID := s.providerCustomModels(ctx, providerID)
	static, staticPrices := connectors.ModelsAndDisplayPricesForProvider(providerID)
	seen := map[string]bool{}
	out := make([]providerModelInfo, 0, len(static))
	for _, m := range static {
		if kindFilter != "" && !m.SupportsKind(kindFilter) {
			continue
		}
		price, ok := staticPrices[m.ID]
		mi := providerModelInfoFor(m, providerModelResponseOptions{
			ProviderID: providerID, KindFilter: kindFilter, Price: price, PriceOK: ok,
		})
		if cm, ok := customByID[m.ID]; ok {
			mi.Custom = true
			mi.DBID = cm.ID
		}
		out = append(out, mi)
		seen[m.ID] = true
	}
	return out, seen
}

func (s *Server) appendLiveProviderModels(ctx context.Context, providerID string, kindFilter core.ServiceKind, source connectors.LiveModelSource, creds core.Credentials, out *[]providerModelInfo, seen map[string]bool) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	models, err := source.ListModels(ctx, creds)
	if err != nil || len(models) == 0 {
		return false
	}
	added := false
	for _, model := range models {
		if kindFilter != "" && !model.SupportsKind(kindFilter) {
			continue
		}
		if seen[model.ID] {
			continue
		}
		price, ok := connectors.ModelDisplayPriceByProviderModel(providerID, model.ID)
		*out = append(*out, providerModelInfoFor(model, providerModelResponseOptions{
			ProviderID: providerID, KindFilter: kindFilter, Price: price, PriceOK: ok, Discovered: true,
		}))
		seen[model.ID] = true
		added = true
	}
	return added
}

func (s *Server) discoverProviderModels(ctx context.Context, providerID string, kindFilter core.ServiceKind, out *[]providerModelInfo, seen map[string]bool) {
	source := connectors.GetLiveModelSource(providerID)
	if source == nil {
		return
	}
	if !s.discoverProviderModelsWithCredentials(ctx, providerID, kindFilter, source, out, seen) && len(*out) == 0 {
		s.appendLiveProviderModels(ctx, providerID, kindFilter, source, core.Credentials{}, out, seen)
	}
}

func (s *Server) discoverProviderModelsWithCredentials(ctx context.Context, providerID string, kindFilter core.ServiceKind, source connectors.LiveModelSource, out *[]providerModelInfo, seen map[string]bool) bool {
	if s.accounts == nil || s.vault == nil {
		return false
	}
	accounts, err := s.accounts.ListByProvider(ctx, adminTenant, providerID)
	if err != nil {
		return false
	}
	for _, account := range accounts {
		if account.Disabled {
			continue
		}
		creds, err := s.vault.Open(account)
		if err != nil {
			continue
		}
		if s.appendLiveProviderModels(ctx, providerID, kindFilter, source, creds, out, seen) {
			return true
		}
	}
	return false
}

// adminProviderModels returns the model list for a specific provider. It
// includes hardcoded non-LLM models plus dynamically discovered LLM models
// (models.dev snapshot and custom models) and, when a connected account
// exists, live models from the upstream (e.g. Kiro's ListAvailableModels).
func (s *Server) adminProviderModels(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "id")
	if _, ok := connectors.SpecByID(providerID); !ok {
		writeError(w, http.StatusNotFound, "unknown provider: "+providerID)
		return
	}
	kindFilter := core.ServiceKind(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind"))))
	if kindFilter != "" && !core.ValidServiceKind(kindFilter) {
		writeError(w, http.StatusBadRequest, "unknown model kind: "+string(kindFilter))
		return
	}

	// Connected providers are served through the 12h model cache so repeated
	// dashboard reads do not re-probe upstreams. Unconnected providers keep
	// the uncached path. Connection state is resolved once and reused for
	// both the read and the store below.
	connected := s.isConnectedProvider(r.Context(), providerID)
	if models, ok := s.cachedProviderModels(connected, providerID, kindFilter); ok {
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
		return
	}
	if connected && kindFilter == "" {
		cache := s.providerModelCache()
		models, _ := cache.refresh(r.Context(), providerID, func(ctx context.Context) ([]providerModelInfo, bool) {
			out, seen := s.catalogProviderModels(ctx, providerID, kindFilter)
			s.discoverProviderModels(ctx, providerID, kindFilter, &out, seen)
			return out, ctx.Err() == nil
		})
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
		return
	}

	out, seen := s.catalogProviderModels(r.Context(), providerID, kindFilter)
	s.discoverProviderModels(r.Context(), providerID, kindFilter, &out, seen)
	s.storeProviderModelCache(connected, providerID, kindFilter, out)
	writeJSON(w, http.StatusOK, map[string]any{"models": out})
}

// ---- API keys ---------------------------------------------------------------

const disabledModelsPrefix = "disabled_models_" // + provider alias

func (s *Server) loadDisabledModels(ctx context.Context, provider string) []string {
	if s.settings == nil {
		return nil
	}
	raw, err := s.settings.Get(ctx, disabledModelsPrefix+provider)
	if err != nil || raw == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	return ids
}

func (s *Server) saveDisabledModels(ctx context.Context, provider string, ids []string) error {
	if s.settings == nil {
		return nil
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return s.settings.Set(ctx, disabledModelsPrefix+provider, string(raw))
}

func (s *Server) adminListDisabledModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider query param is required")
		return
	}
	ids := s.loadDisabledModels(r.Context(), provider)
	writeJSON(w, http.StatusOK, map[string]any{"ids": ids})
}

func (s *Server) adminDisableModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string   `json:"providerAlias"`
		IDs      []string `json:"ids"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Provider == "" {
		writeError(w, http.StatusBadRequest, "providerAlias is required")
		return
	}
	existing := s.loadDisabledModels(r.Context(), body.Provider)
	seen := map[string]bool{}
	for _, id := range existing {
		seen[id] = true
	}
	for _, id := range body.IDs {
		seen[id] = true
	}
	merged := make([]string, 0, len(seen))
	for id := range seen {
		merged = append(merged, id)
	}
	if err := s.saveDisabledModels(r.Context(), body.Provider, merged); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ids": merged})
}

func (s *Server) adminEnableModels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string   `json:"providerAlias"`
		IDs      []string `json:"ids"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Provider == "" {
		writeError(w, http.StatusBadRequest, "providerAlias is required")
		return
	}
	existing := s.loadDisabledModels(r.Context(), body.Provider)
	remove := map[string]bool{}
	for _, id := range body.IDs {
		remove[id] = true
	}
	var kept []string
	for _, id := range existing {
		if !remove[id] {
			kept = append(kept, id)
		}
	}
	if err := s.saveDisabledModels(r.Context(), body.Provider, kept); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ids": kept})
}

// ---- console SSE stream -----------------------------------------------------
