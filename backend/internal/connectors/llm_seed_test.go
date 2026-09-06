package connectors

import (
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// llm builds an LLM-kind ModelSpec for fixtures.
func llmSpec(id, name string) ModelSpec {
	return ModelSpec{ID: id, Name: name, Kind: core.ServiceLLM}
}

// SeedLLMCatalog replaces the models.dev snapshot with the given LLM models
// for the duration of the test and restores an empty snapshot afterwards.
// Use it to fixture dynamic LLM discovery now that there is no hardcoded LLM
// catalog.
func SeedLLMCatalog(t *testing.T, modelsByProvider map[string][]ModelSpec) {
	t.Helper()
	ReplaceFetchedCatalog(modelsByProvider, nil)
	t.Cleanup(func() { ReplaceFetchedCatalog(nil, nil) })
}
