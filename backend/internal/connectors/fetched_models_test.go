package connectors

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func TestSetFetchedModelsPreservesFetchedModelsAcrossCustomReload(t *testing.T) {
	t.Cleanup(func() {
		SetFetchedModels(FetchedModelUpdate{ProviderID: "openrouter"})
		SetDynamicModels("openrouter", nil)
	})

	SetFetchedModels(FetchedModelUpdate{
		ProviderID: "openrouter",
		Models:     []ModelSpec{{ID: "vendor/fetched", Name: "Fetched", Kind: core.ServiceLLM}},
	})
	SetDynamicModels("openrouter", []ModelSpec{
		{ID: "vendor/custom", Name: "Custom", Kind: core.ServiceLLM},
	})

	got := ModelsForProvider("openrouter")
	ids := []string{}
	for _, m := range got {
		ids = append(ids, m.ID)
	}
	for _, want := range []string{"openai/gpt-4o", "vendor/fetched", "vendor/custom"} {
		if !slices.Contains(ids, want) {
			t.Fatalf("ModelsForProvider missing %q; got %v", want, ids)
		}
	}

	// A custom-model reload must not wipe fetched entries.
	SetDynamicModels("openrouter", []ModelSpec{
		{ID: "vendor/custom2", Name: "Custom2", Kind: core.ServiceLLM},
	})
	ids = nil
	for _, m := range ModelsForProvider("openrouter") {
		ids = append(ids, m.ID)
	}
	if !slices.Contains(ids, "vendor/fetched") {
		t.Fatalf("fetched models lost after custom reload: %v", ids)
	}
	if slices.Contains(ids, "vendor/custom") {
		t.Fatalf("old custom model should be replaced: %v", ids)
	}

	assertFetchedModelsSurviveCustomReload(t, ids)
}

func assertFetchedModelsSurviveCustomReload(t *testing.T, ids []string) {
	t.Helper()
	if !slices.Contains(ids, "vendor/fetched") {
		t.Fatalf("fetched models lost after custom reload: %v", ids)
	}
	if slices.Contains(ids, "vendor/custom") {
		t.Fatalf("old custom model should be replaced: %v", ids)
	}
}

func TestSetFetchedModelsMergesServiceKindVariants(t *testing.T) {
	t.Cleanup(func() { SetFetchedModels(FetchedModelUpdate{ProviderID: "openrouter"}) })
	SetFetchedModels(FetchedModelUpdate{
		ProviderID: "openrouter",
		Models:     []ModelSpec{{ID: "openai/gpt-4o", Name: "GPT-4o dup", Kind: core.ServiceLLM}},
	})
	var models []ModelSpec
	for _, m := range ModelsForProvider("openrouter") {
		if m.ID == "openai/gpt-4o" {
			models = append(models, m)
		}
	}
	require.Len(t, models, 1)
	require.True(t, models[0].SupportsKind(core.ServiceLLM))
	require.True(t, models[0].SupportsKind(core.ServiceImageToText))
	model, ok := FindModel("openrouter", "openai/gpt-4o")
	if !ok || model.Kind != core.ServiceLLM {
		t.Fatalf("FindModel = %+v, %v; want LLM variant", model, ok)
	}
}

func TestSetFetchedModelsClear(t *testing.T) {
	SetFetchedModels(FetchedModelUpdate{
		ProviderID: "venice",
		Models:     []ModelSpec{{ID: "venice-model", Name: "V", Kind: core.ServiceLLM}},
	})
	if len(fetchedModelsFor("venice")) != 1 {
		t.Fatal("expected one fetched model")
	}
	SetFetchedModels(FetchedModelUpdate{ProviderID: "venice"})
	if got := fetchedModelsFor("venice"); got != nil {
		t.Fatalf("expected cleared, got %v", got)
	}
}

func TestReplaceFetchedModelsClearsAbsentProviders(t *testing.T) {
	t.Cleanup(func() { ReplaceFetchedModels(nil) })
	ReplaceFetchedModels(map[string][]ModelSpec{
		"openrouter": {{ID: "first", Kind: core.ServiceLLM}},
		"venice":     {{ID: "second", Kind: core.ServiceLLM}},
	})
	ReplaceFetchedModels(map[string][]ModelSpec{
		"openrouter": {{ID: "replacement", Kind: core.ServiceLLM}},
	})
	if got := fetchedModelsFor("venice"); got != nil {
		t.Fatalf("stale fetched provider retained: %v", got)
	}
	if got := fetchedModelsFor("openrouter"); len(got) != 1 || got[0].ID != "replacement" {
		t.Fatalf("replacement snapshot = %v", got)
	}
}

func TestCustomModelMergesServiceKindsWithFetchedModel(t *testing.T) {
	t.Cleanup(func() {
		ReplaceFetchedModels(nil)
		SetDynamicModels("openrouter", nil)
	})
	ReplaceFetchedModels(map[string][]ModelSpec{
		"openrouter": {{ID: "vendor/model", Name: "Fetched", Kind: core.ServiceLLM}},
	})
	SetDynamicModels("openrouter", []ModelSpec{
		{ID: "vendor/model", Name: "Custom vision", Kind: core.ServiceImageToText},
	})
	var models []ModelSpec
	for _, m := range ModelsForProvider("openrouter") {
		if m.ID != "vendor/model" {
			continue
		}
		models = append(models, m)
	}
	require.Len(t, models, 1)
	require.Equal(t, "Custom vision", models[0].Name)
	require.True(t, models[0].SupportsKind(core.ServiceLLM))
	require.True(t, models[0].SupportsKind(core.ServiceImageToText))
}
