package connectors

import (
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func TestCatalogHasCoreProviders(t *testing.T) {
	want := []string{"openai", "anthropic", "deepseek", "groq", "gemini", "openrouter", "tavily", "elevenlabs"}
	for _, id := range want {
		if _, ok := SpecByID(id); !ok {
			t.Errorf("catalog missing provider %q", id)
		}
	}
}

func TestSpecByAlias(t *testing.T) {
	spec, ok := SpecByAlias("ds")
	if !ok || spec.ID != "deepseek" {
		t.Fatalf("alias ds should resolve to deepseek, got %q ok=%v", spec.ID, ok)
	}
	// id also resolves.
	if spec, ok := SpecByAlias("openai"); !ok || spec.ID != "openai" {
		t.Fatalf("id openai should resolve, got %q ok=%v", spec.ID, ok)
	}
}

func TestSpecsByKind(t *testing.T) {
	cases := map[core.ServiceKind][]string{
		core.ServiceImage:  {"nanobanana", "fal-ai", "stability-ai"},
		core.ServiceSTT:    {"deepgram", "assemblyai", "groq"},
		core.ServiceTTS:    {"elevenlabs", "inworld"},
		core.ServiceSearch: {"tavily", "brave-search", "serper", "exa"},
		core.ServiceFetch:  {"firecrawl", "exa", "tavily"},
	}
	for kind, wantProviders := range cases {
		specs := SpecsByKind(kind)
		got := map[string]bool{}
		for _, s := range specs {
			got[s.ID] = true
		}
		for _, id := range wantProviders {
			if !got[id] {
				t.Errorf("kind %q: expected provider %q in results", kind, id)
			}
		}
	}
}

func TestModelsByKind(t *testing.T) {
	// Every LLM model should belong to a provider that advertises the LLM kind.
	llms := ModelsByKind(core.ServiceLLM)
	if len(llms) == 0 {
		t.Fatal("expected at least one LLM model in catalog")
	}
	for _, pm := range llms {
		if pm.Model.Kind != core.ServiceLLM {
			t.Errorf("ModelsByKind(llm) returned non-llm model %q (%q)", pm.Model.ID, pm.Model.Kind)
		}
	}

	// Image models should be present and tagged correctly.
	images := ModelsByKind(core.ServiceImage)
	if len(images) == 0 {
		t.Fatal("expected at least one image model in catalog")
	}
	for _, pm := range images {
		if pm.Model.Kind != core.ServiceImage {
			t.Errorf("ModelsByKind(image) returned non-image model %q", pm.Model.ID)
		}
	}
}

func TestMediaCatalogCompleteness(t *testing.T) {
	// Video providers must surface video-tagged models.
	wantVideo := []string{"xai"}
	video := ModelsByKind(core.ServiceVideo)
	gotVideo := map[string]bool{}
	for _, pm := range video {
		if pm.Model.Kind != core.ServiceVideo {
			t.Errorf("ModelsByKind(video) returned non-video model %q", pm.Model.ID)
		}
		gotVideo[pm.Provider] = true
	}
	for _, id := range wantVideo {
		if !gotVideo[id] {
			t.Errorf("expected video provider %q in catalog", id)
		}
	}

	// ImageToText providers must surface image_to_text-tagged models.
	// (vertex is tagged too but stays Hidden, so it is intentionally not
	// surfaced in the media catalog.)
	wantI2T := []string{
		"anthropic", "gemini", "groq", "mistral", "minimax",
		"openrouter", "vercel-ai-gateway", "xai", "kimchi",
	}
	i2t := ModelsByKind(core.ServiceImageToText)
	gotI2T := map[string]bool{}
	for _, pm := range i2t {
		if pm.Model.Kind != core.ServiceImageToText {
			t.Errorf("ModelsByKind(image_to_text) returned wrong-kind model %q", pm.Model.ID)
		}
		gotI2T[pm.Provider] = true
	}
	for _, id := range wantI2T {
		if !gotI2T[id] {
			t.Errorf("expected image_to_text provider %q in catalog", id)
		}
	}
}

func TestFindModel(t *testing.T) {
	if _, ok := FindModel("openai", "gpt-4o"); !ok {
		t.Error("expected to find openai/gpt-4o")
	}
	if _, ok := FindModel("commandcode", "deepseek/deepseek-v4-pro"); !ok {
		t.Error("expected to find commandcode/deepseek/deepseek-v4-pro")
	}
	if _, ok := FindModel("deepseek", "deepseek-v4-pro-max"); !ok {
		t.Error("expected to find deepseek/deepseek-v4-pro-max")
	}
	if _, ok := FindModel("deepseek", "deepseek-v4-pro-none"); !ok {
		t.Error("expected to find deepseek/deepseek-v4-pro-none")
	}
	if _, ok := FindModel("openai", "nonexistent-model"); ok {
		t.Error("expected miss for nonexistent model")
	}
}

func TestDeepSeekPricing(t *testing.T) {
	flash, ok := ModelPriceByProviderModel("deepseek", "deepseek-v4-flash")
	if !ok {
		t.Fatal("missing deepseek-v4-flash price")
	}
	if flash.InputPerM != 0.14 || flash.OutputPerM != 0.28 || flash.CachedInputPerM != 0.0028 || flash.ReasoningPerM != 0.28 {
		t.Fatalf("unexpected deepseek-v4-flash pricing: %+v", flash)
	}

	pro, ok := ModelPriceByProviderModel("deepseek", "deepseek-v4-pro")
	if !ok {
		t.Fatal("missing deepseek-v4-pro price")
	}
	if pro.InputPerM != 0.435 || pro.OutputPerM != 0.87 || pro.CachedInputPerM != 0.003625 || pro.ReasoningPerM != 0.87 {
		t.Fatalf("unexpected deepseek-v4-pro pricing: %+v", pro)
	}
}

func setFetchedDisplayPrices(t *testing.T) {
	t.Helper()
	ReplaceFetchedModelPrices(map[string][]ModelPrice{
		"openai": {
			{Provider: "openai", Model: "gpt-4o", InputPerM: 999, Source: "models_dev"},
			{Provider: "openai", Model: "models-dev-only", InputPerM: 2.5, OutputPerM: 10, Source: "models_dev"},
		},
	})
	t.Cleanup(func() { ReplaceFetchedModelPrices(nil) })
}

func TestModelDisplayPricePrefersProviderCatalog(t *testing.T) {
	setFetchedDisplayPrices(t)
	catalogPrice, ok := ModelPriceByProviderModel("openai", "gpt-4o")
	if !ok {
		t.Fatal("missing built-in openai/gpt-4o price")
	}
	price, ok := ModelDisplayPriceByProviderModel("openai", "gpt-4o")
	if !ok {
		t.Fatal("missing display price")
	}
	if price.InputPerM != catalogPrice.InputPerM {
		t.Fatalf("display price = %+v, want provider catalog price %+v", price, catalogPrice)
	}
	if price.Source == "models_dev" {
		t.Fatal("models.dev price overrode provider catalog")
	}
}

func TestModelDisplayPriceFallsBackToModelsDev(t *testing.T) {
	setFetchedDisplayPrices(t)
	price, ok := ModelDisplayPriceByProviderModel("openai", "models-dev-only")
	if !ok {
		t.Fatal("missing models.dev fallback")
	}
	if price.InputPerM != 2.5 || price.Source != "models_dev" {
		t.Fatalf("display fallback = %+v, want models.dev price", price)
	}
}

func TestReplaceFetchedCatalogReplacesModelsAndPrices(t *testing.T) {
	ReplaceFetchedCatalog(map[string][]ModelSpec{
		"openai": {{ID: "old-model"}},
	}, map[string][]ModelPrice{
		"openai": {{Provider: "openai", Model: "old-model", InputPerM: 1}},
	})
	t.Cleanup(func() { ReplaceFetchedCatalog(nil, nil) })

	ReplaceFetchedCatalog(map[string][]ModelSpec{
		"openai": {{ID: "new-model"}},
	}, map[string][]ModelPrice{
		"openai": {{Provider: "openai", Model: "new-model", InputPerM: 2}},
	})

	models := fetchedModelsFor("openai")
	if len(models) != 1 || models[0].ID != "new-model" {
		t.Fatalf("fetched models = %+v, want only new-model", models)
	}
	if _, ok := ModelDisplayPriceByProviderModel("openai", "old-model"); ok {
		t.Fatal("stale fetched price remained after catalog replacement")
	}
	price, ok := ModelDisplayPriceByProviderModel("openai", "new-model")
	if !ok || price.InputPerM != 2 {
		t.Fatalf("new fetched price = %+v, %v; want input price 2", price, ok)
	}
}

func TestModelsAndDisplayPricesForProviderReadsOneFetchedSnapshot(t *testing.T) {
	ReplaceFetchedCatalog(map[string][]ModelSpec{
		"openai": {{ID: "old-model"}},
	}, map[string][]ModelPrice{
		"openai": {{Provider: "openai", Model: "old-model", InputPerM: 1}},
	})
	t.Cleanup(func() { ReplaceFetchedCatalog(nil, nil) })

	ReplaceFetchedCatalog(map[string][]ModelSpec{
		"openai": {{ID: "new-model"}},
	}, map[string][]ModelPrice{
		"openai": {{Provider: "openai", Model: "new-model", InputPerM: 2}},
	})

	models, prices := ModelsAndDisplayPricesForProvider("openai")
	if _, ok := prices["old-model"]; ok {
		t.Fatal("stale price returned from fetched catalog snapshot")
	}
	if price, ok := prices["new-model"]; !ok || price.InputPerM != 2 {
		t.Fatalf("new model price = %+v, %v; want input price 2", price, ok)
	}
	for _, model := range models {
		if model.ID == "old-model" {
			t.Fatal("stale model returned from fetched catalog snapshot")
		}
	}
}

func TestModelDisplayPriceOmitsUnpricedModel(t *testing.T) {
	setFetchedDisplayPrices(t)
	if _, ok := ModelDisplayPriceByProviderModel("openai", "unpriced-model"); ok {
		t.Fatal("unexpected price for unpriced model")
	}
}

func TestCommandCodeCatalogVisible(t *testing.T) {
	spec, ok := SpecByID("commandcode")
	if !ok {
		t.Fatal("catalog missing commandcode")
	}
	if spec.Hidden {
		t.Fatal("commandcode should be visible")
	}
	if spec.APIKeyURL != "https://commandcode.ai/studio" {
		t.Fatalf("unexpected commandcode APIKeyURL %q", spec.APIKeyURL)
	}
	if len(ModelsForProvider("commandcode")) == 0 {
		t.Fatal("commandcode should have static models")
	}
}

func TestCatalogHasNewProviders(t *testing.T) {
	// Providers added for coverage parity. Each must exist, expose static
	// models, and (since they use drivable dialects) get a live connector.
	cases := []struct {
		id       string
		alias    string
		authKind string
	}{
		{"venice", "vn", "api_key"},
		{"featherless", "fl", "api_key"},
		{"perplexity-agent", "pa", "api_key"},
		{"mmf", "mmf", "none"},
		{"clinepass", "clinepass", "oauth"},
		{"grok-cli", "gcli", "oauth"},
	}
	r := DefaultRegistry()
	for _, c := range cases {
		spec, ok := SpecByID(c.id)
		if !ok {
			t.Errorf("catalog missing provider %q", c.id)
			continue
		}
		if spec.AuthKind != c.authKind {
			t.Errorf("%s: AuthKind = %q, want %q", c.id, spec.AuthKind, c.authKind)
		}
		if s, ok := SpecByAlias(c.alias); !ok || s.ID != c.id {
			t.Errorf("alias %q should resolve to %q, got %q ok=%v", c.alias, c.id, s.ID, ok)
		}
		if len(ModelsForProvider(c.id)) == 0 {
			t.Errorf("%s should have static models", c.id)
		}
		if !r.Has(c.id) {
			t.Errorf("registry should have connector for %q", c.id)
		}
	}
}

func TestDrivableDialect(t *testing.T) {
	for _, d := range []core.Dialect{
		core.DialectOpenAI, core.DialectAnthropic, core.DialectGemini, core.DialectOllama,
	} {
		if !DrivableDialect(d) {
			t.Errorf("dialect %q must be drivable", d)
		}
	}
	// All proprietary/subscription dialects are now drivable: the full
	// transport surface has been ported.
	for _, d := range []core.Dialect{
		core.DialectKiro, core.DialectGeminiCLI, core.DialectAntigravity,
		core.DialectCommandCode, core.DialectCursor, core.DialectQoder,
	} {
		if !DrivableDialect(d) {
			t.Errorf("dialect %q must be drivable", d)
		}
	}
	// An unknown dialect remains non-drivable.
	if DrivableDialect(core.Dialect("does-not-exist")) {
		t.Error("unknown dialect must not be drivable")
	}
}

func TestRegistryRegistersDrivableProviders(t *testing.T) {
	r := DefaultRegistry()
	// OpenAI/Anthropic chat, Gemini, Ollama, and web providers should be registered.
	for _, id := range []string{"openai", "anthropic", "gemini", "ollama", "ollama-local", "tavily", "firecrawl"} {
		if !r.Has(id) {
			t.Errorf("registry should have connector for %q", id)
		}
	}
	// Every subscription/proprietary provider now has a connector.
	for _, id := range []string{"kiro", "gemini-cli", "antigravity", "commandcode", "cursor", "github", "qwen", "iflow"} {
		if !r.Has(id) {
			t.Errorf("registry should have connector for %q", id)
		}
	}
	// An unknown provider still has no connector.
	if r.Has("does-not-exist") {
		t.Error("registry should not have a connector for an unknown provider")
	}
}
