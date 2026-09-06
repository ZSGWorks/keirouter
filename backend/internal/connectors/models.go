package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// ModelSpec describes a single model offered by a provider, tagged with the
// service kind it serves. It backs the per-kind discovery endpoints
// (GET /v1/models/<kind>) and model info lookups (GET /v1/models/info).
type ModelSpec struct {
	// ID is the model id the client passes (provider-local, no alias prefix).
	ID string `json:"id"`
	// Name is a human-friendly display name.
	Name string `json:"name"`
	// Kind is the service kind this model serves (defaults to LLM).
	Kind core.ServiceKind `json:"kind"`
	// Kinds lists every service kind supported by this model. Kind remains the
	// primary kind for callers that only understand one service kind.
	Kinds []core.ServiceKind `json:"kinds,omitempty"`
	// Dimensions is the embedding vector width (embedding models only).
	Dimensions int `json:"dimensions,omitempty"`
}

// SupportedKinds returns all model service kinds in a stable order. Legacy
// single-kind entries remain LLM-only when Kind is empty.
func (m ModelSpec) SupportedKinds() []core.ServiceKind {
	kinds := make([]core.ServiceKind, 0, len(m.Kinds)+1)
	primary := m.Kind
	if primary == "" {
		primary = core.ServiceLLM
	}
	kinds = append(kinds, primary)
	for _, kind := range m.Kinds {
		if !core.HasServiceKind(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// SupportsKind reports whether this model can serve the requested operation.
func (m ModelSpec) SupportsKind(kind core.ServiceKind) bool {
	return core.HasServiceKind(m.SupportedKinds(), kind)
}

// ModelsForProvider returns the model list for a provider id, composing the
// hardcoded non-LLM entries with the dynamically discovered LLM models
// (models.dev snapshot plus user-registered custom models). Custom models
// override entries with the same id.
func ModelsForProvider(providerID string) []ModelSpec {
	return modelsForProviderFromSnapshot(providerID, fetchedModelsFor(providerID), dynamicModelsFor(providerID))
}

// LLMModelsForProvider returns the dynamically discovered LLM models for a
// provider id: the models.dev snapshot first, then user-registered custom
// models. There is no hardcoded LLM fallback; an empty result means neither
// source knows this provider.
func LLMModelsForProvider(providerID string) []ModelSpec {
	return mergeLLMModels(fetchedModelsFor(providerID), dynamicLLMModelsFor(providerID))
}

// StaticNonLLMModelsForProvider returns the hardcoded non-LLM models for a
// provider id (embedding, TTS, STT, image, video, image-to-text, search,
// fetch), plus user-registered custom non-LLM models.
func StaticNonLLMModelsForProvider(providerID string) []ModelSpec {
	return mergeNonLLMModels(providerStaticModels[providerID], dynamicNonLLMModelsFor(providerID))
}

// dynamicLLMModelsFor returns the user-registered custom LLM models.
func dynamicLLMModelsFor(providerID string) []ModelSpec {
	return filterModelSpecsByKind(dynamicModelsFor(providerID), core.ServiceLLM)
}

// dynamicNonLLMModelsFor returns the user-registered custom non-LLM models.
func dynamicNonLLMModelsFor(providerID string) []ModelSpec {
	return filterModelSpecsExcludingKind(dynamicModelsFor(providerID), core.ServiceLLM)
}

func filterModelSpecsByKind(models []ModelSpec, kind core.ServiceKind) []ModelSpec {
	var out []ModelSpec
	for _, mdl := range models {
		if mdl.SupportsKind(kind) {
			out = append(out, mdl)
		}
	}
	return out
}

func filterModelSpecsExcludingKind(models []ModelSpec, kind core.ServiceKind) []ModelSpec {
	var out []ModelSpec
	for _, mdl := range models {
		for _, supported := range mdl.SupportedKinds() {
			if supported != kind {
				out = append(out, mdl)
				break
			}
		}
	}
	return out
}

// modelsForProviderFromSnapshot composes the model list for a provider id
// from previously snapshotted fetched and custom models, without taking any
// locks. Callers holding (or having just released) dynMu use it to keep the
// model list consistent with a price snapshot.
func modelsForProviderFromSnapshot(providerID string, fetched, custom []ModelSpec) []ModelSpec {
	base := mergeModelSpecs(providerStaticModels[providerID], fetched)
	return mergeCustomModelSpecs(base, custom)
}

// ModelsAndDisplayPricesForProvider returns a model list and its resolved
// prices from one fetched catalog snapshot. Built-in provider prices still win.
func ModelsAndDisplayPricesForProvider(providerID string) ([]ModelSpec, map[string]ModelPrice) {
	dynMu.RLock()
	fetched := copiedModelSpecs(fetchedModels[providerID])
	custom := copiedModelSpecs(dynModels[providerID])
	prices := make(map[string][]ModelPrice, len(fetchedModelPrices))
	for sourceProvider, entries := range fetchedModelPrices {
		prices[sourceProvider] = append([]ModelPrice(nil), entries...)
	}
	dynMu.RUnlock()

	models := modelsForProviderFromSnapshot(providerID, fetched, custom)
	resolved := make(map[string]ModelPrice, len(models))
	for _, model := range models {
		if price, ok := ModelPriceByProviderModel(providerID, model.ID); ok {
			resolved[model.ID] = price
			continue
		}
		if price, ok := fetchedModelDisplayPriceFromSnapshot(prices, providerID, model.ID); ok {
			resolved[model.ID] = price
		}
	}
	return models, resolved
}

// mergeLLMModels composes the dynamic LLM set: the models.dev snapshot is
// primary, user-registered custom models override same-id entries.
func mergeLLMModels(fetched, custom []ModelSpec) []ModelSpec {
	return mergeCustomModelSpecs(fetched, custom)
}

// mergeNonLLMModels composes the non-LLM set: hardcoded entries are primary,
// user-registered custom models override same-id entries.
func mergeNonLLMModels(static, custom []ModelSpec) []ModelSpec {
	return mergeCustomModelSpecs(static, custom)
}

func mergeCustomModelSpecs(base, custom []ModelSpec) []ModelSpec {
	return mergeModelSpecs(base, custom)
}

func appendMissingModelSpecs(primary, additions []ModelSpec) []ModelSpec {
	return mergeModelSpecs(primary, additions)
}

func modelSpecKeys(models []ModelSpec) map[string]bool {
	keys := make(map[string]bool, len(models))
	for _, model := range models {
		keys[modelSpecKey(model)] = true
	}
	return keys
}

func modelSpecKey(m ModelSpec) string {
	return m.ID
}

// mergeModelSpecs preserves one model per provider-local ID. Later entries
// override display metadata while service-kind membership is unioned.
func mergeModelSpecs(primary, additions []ModelSpec) []ModelSpec {
	merged := make([]ModelSpec, 0, len(primary)+len(additions))
	indexes := make(map[string]int, len(primary)+len(additions))
	appendModel := func(model ModelSpec, override bool) {
		key := modelSpecKey(model)
		if index, ok := indexes[key]; ok {
			merged[index] = mergeModelSpec(merged[index], model, override)
			return
		}
		indexes[key] = len(merged)
		merged = append(merged, normalizeModelSpec(model))
	}
	for _, model := range primary {
		appendModel(model, false)
	}
	for _, model := range additions {
		appendModel(model, true)
	}
	return merged
}

func normalizeModelSpec(model ModelSpec) ModelSpec {
	model.Kinds = model.SupportedKinds()
	model.Kind = primaryModelKind(model.Kinds)
	return model
}

func mergeModelSpec(base, addition ModelSpec, override bool) ModelSpec {
	baseKinds := base.SupportedKinds()
	if override {
		if addition.Name == "" {
			addition.Name = base.Name
		}
		if addition.Dimensions == 0 {
			addition.Dimensions = base.Dimensions
		}
		base = addition
	}
	base.Kinds = unionModelKinds(baseKinds, addition.SupportedKinds())
	base.Kind = primaryModelKind(base.Kinds)
	return base
}

func unionModelKinds(primary, additions []core.ServiceKind) []core.ServiceKind {
	merged := append([]core.ServiceKind{}, primary...)
	for _, kind := range additions {
		if !core.HasServiceKind(merged, kind) {
			merged = append(merged, kind)
		}
	}
	return merged
}

func primaryModelKind(kinds []core.ServiceKind) core.ServiceKind {
	if core.HasServiceKind(kinds, core.ServiceLLM) {
		return core.ServiceLLM
	}
	if len(kinds) > 0 {
		return kinds[0]
	}
	return core.ServiceLLM
}

// ModelsByKind returns all (providerID, model) pairs across the catalog that
// serve the given service kind, excluding hidden providers.
type ProviderModel struct {
	Provider string
	Model    ModelSpec
}

// ModelsByKind collects every model of the given kind across all non-hidden
// providers in the catalog.
func ModelsByKind(kind core.ServiceKind) []ProviderModel {
	var out []ProviderModel
	for _, spec := range Catalog() {
		if spec.Hidden {
			continue
		}
		if !core.HasServiceKind(spec.ServiceKinds, kind) {
			continue
		}
		for _, mdl := range ModelsForProvider(spec.ID) {
			if mdl.SupportsKind(kind) {
				out = append(out, ProviderModel{Provider: spec.ID, Model: mdl})
			}
		}
	}
	return out
}

// FindModel locates a model by provider id and model id.
func FindModel(providerID, modelID string) (ModelSpec, bool) {
	for _, mdl := range ModelsForProvider(providerID) {
		if mdl.ID == modelID {
			return mdl, true
		}
	}
	return ModelSpec{}, false
}

// LiveModelSource is implemented by connectors that can fetch their model
// catalog from the upstream API at runtime (e.g. Kiro's ListAvailableModels).
// The gateway uses this to supplement the models.dev/dynamic catalog with
// live data when an account is connected.
type LiveModelSource interface {
	// ListModels fetches the live model catalog from the upstream. The returned
	// models should already include any synthetic variants (e.g. Kiro's
	// -thinking/-agentic expansions). The creds carry the access token needed
	// to authenticate with the upstream.
	ListModels(ctx context.Context, creds core.Credentials) ([]ModelSpec, error)
}

// liveModelSources is the registry of providers that support live model
// discovery. Populated at init time.
var liveModelSources = map[string]LiveModelSource{}

// RegisterLiveModelSource registers a live model source for a provider.
func RegisterLiveModelSource(provider string, src LiveModelSource) {
	liveModelSources[provider] = src
}

// GetLiveModelSource returns the live model source for a provider, or nil.
func GetLiveModelSource(provider string) LiveModelSource {
	if src, ok := liveModelSources[provider]; ok {
		return src
	}
	// Dynamic (user-defined) providers are not in the static registry, so build
	// a discovery source on demand. This lets a custom provider's /models
	// endpoint populate the catalog just like a built-in one.
	if p, ok := DynamicProviderByID(provider); ok {
		switch p.Dialect {
		case core.DialectOpenAI:
			return &OpenAICompatibleModelSource{provider: p.ID, defaultBase: p.BaseURL}
		case core.DialectAnthropic:
			return &AnthropicCompatibleModelSource{provider: p.ID, defaultBase: p.BaseURL}
		}
	}
	return nil
}

// AnthropicCompatibleModelSource implements LiveModelSource for custom
// anthropic-compatible dynamic providers by fetching GET <base>/v1/models
// (Anthropic shape: {"data":[{"id":"...","display_name":"..."}]}).
type AnthropicCompatibleModelSource struct {
	provider    string
	defaultBase string
}

func (s *AnthropicCompatibleModelSource) ListModels(ctx context.Context, creds core.Credentials) ([]ModelSpec, error) {
	base := s.defaultBase
	if creds.BaseURL != "" {
		base = creds.BaseURL
	}
	for key, val := range creds.Extra {
		base = strings.ReplaceAll(base, "{"+key+"}", val)
	}
	base = strings.TrimRight(base, "/")
	base = strings.TrimSuffix(base, "/messages")
	base = strings.TrimSuffix(base, "/v1")
	url := strings.TrimRight(base, "/") + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if k := strings.TrimSpace(creds.APIKey); k != "" {
		req.Header.Set("x-api-key", k)
	} else if t := strings.TrimSpace(creds.AccessToken); t != "" {
		req.Header.Set("Authorization", bearer(t))
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")

	resp, err := sharedClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, fmt.Errorf("GET /v1/models returned %d: %s", resp.StatusCode, truncateError(body))
	}

	var envelope struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode /v1/models response: %w", err)
	}
	out := make([]ModelSpec, 0, len(envelope.Data))
	for _, e := range envelope.Data {
		if e.ID == "" {
			continue
		}
		name := e.DisplayName
		if name == "" {
			name = e.ID
		}
		out = append(out, ModelSpec{ID: e.ID, Name: name, Kind: core.ServiceLLM})
	}
	return out, nil
}

// QuotaEntry is one upstream quota bucket (e.g. AGENTIC_REQUEST usage).
type QuotaEntry struct {
	ResourceType string `json:"resource_type"`
	Used         int    `json:"used"`
	Limit        int    `json:"limit"`
	Remaining    int    `json:"remaining"`
	ResetAt      string `json:"reset_at,omitempty"`
	PlanName     string `json:"plan_name,omitempty"`
}

// QuotaResult holds the upstream quota info for an account.
type QuotaResult struct {
	PlanName string       `json:"plan_name,omitempty"`
	Quotas   []QuotaEntry `json:"quotas"`
	Message  string       `json:"message,omitempty"`
}

// QuotaSource is implemented by connectors that can fetch upstream quota/usage
// info (e.g. Kiro's getUsageLimits).
type QuotaSource interface {
	FetchQuota(ctx context.Context, creds core.Credentials) (*QuotaResult, error)
}

var quotaSources = map[string]QuotaSource{}

// RegisterQuotaSource registers a quota source for a provider.
func RegisterQuotaSource(provider string, src QuotaSource) {
	quotaSources[provider] = src
}

// GetQuotaSource returns the quota source for a provider, or nil.
func GetQuotaSource(provider string) QuotaSource {
	return quotaSources[provider]
}
