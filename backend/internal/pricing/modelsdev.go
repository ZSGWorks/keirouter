// Package pricing fetches community-maintained model metadata from models.dev
// and projects it into KeiRouter's pricing table and provider model catalogs.
// It is additive: static catalog entries and user-entered custom prices always
// remain available as fallbacks.
package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/core"
)

const (
	modelsDevURL    = "https://models.dev/api.json"
	modelsDevSource = "models_dev"

	// DefaultRefreshInterval is how often the catalog is refreshed.
	DefaultRefreshInterval = 24 * time.Hour
	// fetchTimeout bounds a single api.json download.
	fetchTimeout = 60 * time.Second
)

// modelsDevToKeirouter maps models.dev provider ids to the KeiRouter provider
// ids they feed. models.dev providers absent from this table are ignored.
var modelsDevToKeirouter = map[string][]string{
	"openrouter":            {"openrouter"},
	"ollama-cloud":          {"ollama"},
	"zhipuai":               {"glm"},
	"alibaba":               {"qwen"},
	"cloudflare-workers-ai": {"cloudflare-ai"},
	"togetherai":            {"together"},
	"fireworks-ai":          {"fireworks"},
	"vercel":                {"vercel-ai-gateway"},
	"openai":                {"openai"},
	"anthropic":             {"anthropic"},
	"google":                {"gemini"},
	"groq":                  {"groq"},
	"mistral":               {"mistral"},
	"xai":                   {"xai"},
	"perplexity":            {"perplexity"},
	"cohere":                {"cohere"},
	"cerebras":              {"cerebras"},
	"nebius":                {"nebius"},
	"nvidia":                {"nvidia"},
	"minimax":               {"minimax"},
	"deepseek":              {"deepseek"},
	"chutes":                {"chutes"},
	"siliconflow":           {"siliconflow"},
	"venice":                {"venice"},
	"opencode":              {"opencode"},
	"moonshotai":            {"moonshotai"},
}

// modelsDevProvider is the subset of the models.dev api.json provider object
// KeiRouter consumes. Unknown fields are ignored on decode.
type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Name   string                    `json:"name"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Modalities *modelsDevMod   `json:"modalities"`
	ToolCall   bool            `json:"tool_call"`
	Reasoning  bool            `json:"reasoning"`
	Cost       *modelsDevCost  `json:"cost"`
	Limit      *modelsDevLimit `json:"limit"`
}

type modelsDevMod struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type modelsDevCost struct {
	Input      float64        `json:"input"`
	Output     float64        `json:"output"`
	CacheRead  float64        `json:"cache_read"`
	CacheWrite float64        `json:"cache_write"`
	Over200k   *modelsDevCost `json:"context_over_200k"`
}

type modelsDevLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// Fetcher retrieves and caches the models.dev catalog. It is safe for
// concurrent use; the ETag makes repeated refreshes cheap (304 responses).
type Fetcher struct {
	mu        sync.Mutex
	url       string
	client    *http.Client
	etag      string
	providers map[string]modelsDevProvider // raw decoded catalog, empty until first successful fetch
	prices    map[string][]connectors.ModelPrice
	models    map[string][]connectors.ModelSpec
}

// NewFetcher builds a models.dev fetcher.
func NewFetcher() *Fetcher {
	return &Fetcher{url: modelsDevURL, client: &http.Client{Timeout: fetchTimeout}}
}

// Result is the projection of one models.dev fetch into KeiRouter structures.
type Result struct {
	// Prices maps KeiRouter provider id -> priced models.
	Prices map[string][]connectors.ModelPrice
	// Models maps KeiRouter provider id -> model specs for discovery.
	Models map[string][]connectors.ModelSpec
	// Providers is the number of mapped providers that yielded entries.
	Providers int
}

// Refresh fetches (or 304-revalidates) the catalog and returns the projected
// result. On network failure the last successful projection is returned with
// an error, so callers can decide whether to keep serving stale data.
func (f *Fetcher) Refresh(ctx context.Context) (Result, error) {
	provs, err := f.fetch(ctx)
	res := project(provs)
	f.mu.Lock()
	f.prices, f.models = res.Prices, res.Models
	f.mu.Unlock()
	return res, err
}

// CachedPrices returns the last projected prices keyed by provider id. Empty
// until the first successful refresh.
func (f *Fetcher) CachedPrices() map[string][]connectors.ModelPrice {
	f.mu.Lock()
	defer f.mu.Unlock()
	return copyPrices(f.prices)
}

// CachedModels returns the last projected model specs keyed by provider id.
// Empty until the first successful refresh.
func (f *Fetcher) CachedModels() map[string][]connectors.ModelSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return copyModelSpecs(f.models)
}

func copyPrices(prices map[string][]connectors.ModelPrice) map[string][]connectors.ModelPrice {
	copy := make(map[string][]connectors.ModelPrice, len(prices))
	for provider, entries := range prices {
		copy[provider] = append([]connectors.ModelPrice(nil), entries...)
	}
	return copy
}

func copyModelSpecs(models map[string][]connectors.ModelSpec) map[string][]connectors.ModelSpec {
	copy := make(map[string][]connectors.ModelSpec, len(models))
	for provider, entries := range models {
		copy[provider] = append([]connectors.ModelSpec(nil), entries...)
	}
	return copy
}

func (f *Fetcher) fetch(ctx context.Context) (map[string]modelsDevProvider, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.providers == nil {
		f.providers = map[string]modelsDevProvider{}
	}

	req, err := f.request(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return f.providers, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return f.providers, nil
	}
	if resp.StatusCode >= 400 {
		return f.providers, fmt.Errorf("models.dev GET returned %d", resp.StatusCode)
	}
	next, err := decodeProviders(resp)
	if err != nil {
		return f.providers, fmt.Errorf("decode models.dev catalog: %w", err)
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		f.etag = etag
	}
	f.providers = next
	return f.providers, nil
}

func (f *Fetcher) request(ctx context.Context) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if f.etag != "" {
		req.Header.Set("If-None-Match", f.etag)
	}
	return req, nil
}

func decodeProviders(resp *http.Response) (map[string]modelsDevProvider, error) {
	var providers map[string]modelsDevProvider
	return providers, json.NewDecoder(resp.Body).Decode(&providers)
}

// project turns the raw catalog into prices and model specs keyed by
// KeiRouter provider id. Models without a cost object still contribute model
// specs (discovery) but no price entry.
func project(provs map[string]modelsDevProvider) Result {
	res := Result{
		Prices: map[string][]connectors.ModelPrice{},
		Models: map[string][]connectors.ModelSpec{},
	}
	for mdID, prov := range provs {
		projectProvider(&res, keirouterIDsFor(mdID), prov.Models)
	}
	return res
}

func projectProvider(res *Result, providerIDs []string, models map[string]modelsDevModel) {
	for _, providerID := range providerIDs {
		if projectModels(res, providerID, models) {
			res.Providers++
		}
	}
}

func projectModels(res *Result, providerID string, models map[string]modelsDevModel) bool {
	mapped := false
	for _, model := range models {
		if price, ok := modelPrice(providerID, model); ok {
			res.Prices[providerID] = append(res.Prices[providerID], price)
		}
		if specs := modelSpecs(model); len(specs) > 0 {
			mapped = true
			res.Models[providerID] = append(res.Models[providerID], specs...)
		}
	}
	return mapped
}

// keirouterIDsFor resolves the KeiRouter provider ids a models.dev provider
// feeds, or nil when the provider is not mapped.
func keirouterIDsFor(mdID string) []string {
	return modelsDevToKeirouter[mdID]
}

// modelPrice converts a models.dev cost object into a ModelPrice. Returns
// false when the model has no usable cost data (subscription-only, unknown).
func modelPrice(provider string, m modelsDevModel) (connectors.ModelPrice, bool) {
	if !validModelCost(m) {
		return connectors.ModelPrice{}, false
	}
	c := m.Cost
	mp := baseModelPrice(provider, m.ID, c)
	applyCacheRates(&mp, c)
	applyLongContextRates(&mp, c.Over200k)
	mp.ExplicitFree = freeTierID(m.ID) && c.Input == 0 && c.Output == 0
	return mp, true
}

func validModelCost(m modelsDevModel) bool {
	if m.Cost == nil || invalidCost(m.Cost) {
		return false
	}
	return m.Cost.Input != 0 || m.Cost.Output != 0 || freeTierID(m.ID)
}

func baseModelPrice(provider, model string, cost *modelsDevCost) connectors.ModelPrice {
	return connectors.ModelPrice{Provider: provider, Model: model, InputPerM: cost.Input, OutputPerM: cost.Output, Source: modelsDevSource, SourceURL: "https://models.dev/" + providerIDForURL(provider)}
}

func applyCacheRates(price *connectors.ModelPrice, cost *modelsDevCost) {
	price.CachedInputPerM = cost.CacheRead
	price.CacheWritePerM = cost.CacheWrite
}

func applyLongContextRates(price *connectors.ModelPrice, cost *modelsDevCost) {
	if cost == nil {
		return
	}
	price.LongContextThreshold = 200000
	price.LongInputPerM = cost.Input
	price.LongOutputPerM = cost.Output
	price.LongCachedInputPerM = cost.CacheRead
	price.LongCacheWritePerM = cost.CacheWrite
}

// invalidCost rejects variable (-1) and malformed rates. Metering treats
// negative rates as a zero charge, but still reports the model as priced.
func invalidCost(c *modelsDevCost) bool {
	return invalidRate(c.Input) || invalidRate(c.Output) || invalidRate(c.CacheRead) || invalidRate(c.CacheWrite) || invalidLongContextCost(c.Over200k)
}

func invalidRate(rate float64) bool {
	return rate < 0
}

func invalidLongContextCost(cost *modelsDevCost) bool {
	if cost == nil {
		return false
	}
	return invalidCost(cost)
}

// freeTierID reports whether the model id marks a known free tier.
func freeTierID(id string) bool {
	for _, suffix := range []string{":free", "-free"} {
		if len(id) >= len(suffix) && id[len(id)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}

func providerIDForURL(provider string) string {
	return provider
}

// modelSpecs projects a models.dev model into discovery specs. Multimodal
// inputs fan out into additional kind-tagged specs, mirroring the static
// media-tag pattern in connectors/models.go.
func modelSpecs(m modelsDevModel) []connectors.ModelSpec {
	if m.ID == "" {
		return nil
	}
	name := m.Name
	if name == "" {
		name = m.ID
	}
	specs := []connectors.ModelSpec{{ID: m.ID, Name: name, Kind: core.ServiceLLM}}
	if m.Modalities != nil {
		has := func(list []string, v string) bool {
			for _, s := range list {
				if s == v {
					return true
				}
			}
			return false
		}
		if has(m.Modalities.Input, "image") {
			specs = append(specs, connectors.ModelSpec{ID: m.ID, Name: name, Kind: core.ServiceImageToText})
		}
		if has(m.Modalities.Output, "image") {
			specs = append(specs, connectors.ModelSpec{ID: m.ID, Name: name, Kind: core.ServiceImage})
		}
	}
	return specs
}
