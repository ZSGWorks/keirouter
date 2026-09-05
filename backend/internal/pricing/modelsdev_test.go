package pricing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/core"
)

func fixture(t *testing.T) map[string]modelsDevProvider {
	t.Helper()
	raw, err := os.ReadFile("testdata/modelsdev_fixture.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var provs map[string]modelsDevProvider
	if err := json.Unmarshal(raw, &provs); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return provs
}

func TestProjectOpenRouterPrices(t *testing.T) {
	price := requiredPrice(t, project(fixture(t)), "openrouter", "anthropic/claude-opus-4.7")
	assertOpusRates(t, price)
	assertOpusCacheRates(t, price)
	assertOpusLongContextRates(t, price)
	assertOpusProvenance(t, price)
}

func assertOpusRates(t *testing.T, price connectors.ModelPrice) {
	t.Helper()
	if price.InputPerM != 5 || price.OutputPerM != 25 {
		t.Fatalf("claude-opus-4.7 rates = %v/%v, want 5/25", price.InputPerM, price.OutputPerM)
	}
}

func assertOpusCacheRates(t *testing.T, price connectors.ModelPrice) {
	t.Helper()
	if price.CachedInputPerM != 0.5 || price.CacheWritePerM != 6.25 {
		t.Fatalf("claude-opus-4.7 cache rates = %v/%v", price.CachedInputPerM, price.CacheWritePerM)
	}
}

func assertOpusLongContextRates(t *testing.T, price connectors.ModelPrice) {
	t.Helper()
	if price.LongContextThreshold != 200000 {
		t.Fatalf("claude-opus-4.7 long-context threshold = %v, want 200000", price.LongContextThreshold)
	}
	if price.LongInputPerM != 10 {
		t.Fatalf("claude-opus-4.7 long-context input = %v, want 10", price.LongInputPerM)
	}
	if price.LongOutputPerM != 37.5 {
		t.Fatalf("claude-opus-4.7 long-context = %v/%v/%v", price.LongContextThreshold, price.LongInputPerM, price.LongOutputPerM)
	}
}

func assertOpusProvenance(t *testing.T, price connectors.ModelPrice) {
	t.Helper()
	if price.Source != modelsDevSource || price.Estimated {
		t.Fatalf("claude-opus-4.7 provenance = %q estimated=%v", price.Source, price.Estimated)
	}
}

func requiredPrice(t *testing.T, res Result, provider, model string) connectors.ModelPrice {
	t.Helper()
	var found *connectors.ModelPrice
	for _, price := range res.Prices[provider] {
		if price.Model == model {
			if found != nil {
				t.Fatalf("%s priced more than once", model)
			}
			found = &price
		}
	}
	if found == nil {
		t.Fatalf("missing %s price", model)
	}
	return *found
}

func TestProjectFreeAndVariablePricing(t *testing.T) {
	prices := project(fixture(t)).Prices["openrouter"]
	assertFreePrice(t, prices)
	if hasPrice(prices, "openrouter/auto") {
		t.Fatal("variable-priced (-1) model should be skipped, not priced zero")
	}
}

func assertFreePrice(t *testing.T, prices []connectors.ModelPrice) {
	t.Helper()
	for _, price := range prices {
		if strings.HasSuffix(price.Model, ":free") {
			if !price.ExplicitFree {
				t.Fatalf("free model %s should be explicit", price.Model)
			}
			if price.InputPerM != 0 {
				t.Fatalf("free model %s input = %v, want 0", price.Model, price.InputPerM)
			}
			if price.OutputPerM != 0 {
				t.Fatalf("free model %s = %+v, want ExplicitFree zero-rate", price.Model, price)
			}
			return
		}
	}
	t.Fatal("expected a :free model in projected prices")
}

func hasPrice(prices []connectors.ModelPrice, model string) bool {
	for _, price := range prices {
		if price.Model == model {
			return true
		}
	}
	return false
}

func TestProjectMapsGoogleAndRejectsInvalidRates(t *testing.T) {
	res := project(map[string]modelsDevProvider{
		"google": {Models: map[string]modelsDevModel{
			"gemini-test": {ID: "gemini-test", Cost: &modelsDevCost{Input: 1, Output: 2}},
		}},
		"openrouter": {Models: map[string]modelsDevModel{
			"variable": {ID: "variable", Cost: &modelsDevCost{Input: -1, Output: 1}},
			"bad-long": {ID: "bad-long", Cost: &modelsDevCost{Input: 1, Output: 1, Over200k: &modelsDevCost{Output: -1}}},
		}},
	})
	if got := len(res.Prices["gemini"]); got != 1 {
		t.Fatalf("google prices mapped to gemini = %d, want 1", got)
	}
	if got := len(res.Prices["openrouter"]); got != 0 {
		t.Fatalf("invalid prices retained = %d", got)
	}
}

func TestProjectModelSpecs(t *testing.T) {
	res := project(fixture(t))
	assertModelKinds(t, res.Models["openrouter"], "anthropic/claude-opus-4.7")
	if got := len(res.Models["glm"]); got == 0 {
		t.Fatal("expected zhipuai models mapped to glm provider id")
	}
	if got := len(res.Models["ollama"]); got == 0 {
		t.Fatal("expected ollama-cloud models mapped to ollama provider id")
	}
	assertModelsDevPrices(t, res.Prices["ollama"])
	if _, ok := res.Models["notmapped"]; ok {
		t.Fatal("unmapped provider leaked into result")
	}
}

func assertModelKinds(t *testing.T, models []connectors.ModelSpec, id string) {
	t.Helper()
	kinds := map[core.ServiceKind]bool{}
	for _, model := range models {
		if model.ID == id {
			kinds[model.Kind] = true
		}
	}
	if !kinds[core.ServiceLLM] || !kinds[core.ServiceImageToText] {
		t.Fatalf("%s kinds = %v, want LLM and vision", id, kinds)
	}
}

func assertModelsDevPrices(t *testing.T, prices []connectors.ModelPrice) {
	t.Helper()
	for _, price := range prices {
		if price.Source != modelsDevSource {
			t.Fatalf("unexpected price source %q", price.Source)
		}
	}
}

func TestFetcherRefreshAndETag(t *testing.T) {
	raw, err := os.ReadFile("testdata/modelsdev_fixture.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var etagCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etagCalls++
		if r.Header.Get("If-None-Match") == "\"v1\"" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", "\"v1\"")
		w.Write(raw)
	}))
	defer srv.Close()

	f := NewFetcher()
	f.url = srv.URL

	res, err := f.Refresh(context.Background())
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if len(res.Prices["openrouter"]) == 0 {
		t.Fatal("expected openrouter prices after first refresh")
	}

	// Second refresh sends If-None-Match and gets 304; cached projection returned.
	res2, err := f.Refresh(context.Background())
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	if len(res2.Prices["openrouter"]) != len(res.Prices["openrouter"]) {
		t.Fatal("304 refresh should keep last catalog")
	}
}

func TestFetcherKeepsLastGoodOnServerError(t *testing.T) {
	raw, err := os.ReadFile("testdata/modelsdev_fixture.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(raw)
	}))
	defer srv.Close()

	f := NewFetcher()
	f.url = srv.URL
	if _, err := f.Refresh(context.Background()); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	fail = true
	res, rerr := f.Refresh(context.Background())
	if rerr == nil {
		t.Fatal("expected error on server failure")
	}
	if len(res.Prices["openrouter"]) == 0 {
		t.Fatal("stale projection should survive a failed refresh")
	}
}
