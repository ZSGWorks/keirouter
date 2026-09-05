package connectors

// FetchedModelUpdate replaces models.dev catalog models for one provider.
// Empty Models clears the provider entry.
type FetchedModelUpdate struct {
	ProviderID string
	Models     []ModelSpec
}

var fetchedModels = map[string][]ModelSpec{} // providerID -> models.dev catalog models

// SetFetchedModels applies one provider update without affecting user-defined models.
func SetFetchedModels(update FetchedModelUpdate) {
	dynMu.Lock()
	defer dynMu.Unlock()
	setModelSpecs(fetchedModels, update.ProviderID, update.Models)
}

// ReplaceFetchedModels atomically replaces the complete models.dev snapshot.
// User-defined models and dynamic providers are held in separate stores.
func ReplaceFetchedModels(modelsByProvider map[string][]ModelSpec) {
	next := make(map[string][]ModelSpec, len(modelsByProvider))
	for providerID, models := range modelsByProvider {
		if len(models) == 0 {
			continue
		}
		next[providerID] = copiedModelSpecs(models)
	}

	dynMu.Lock()
	defer dynMu.Unlock()
	fetchedModels = next
}

// ReplaceFetchedCatalog atomically publishes models.dev model and price data so
// callers never observe a newly discovered model with a stale price snapshot.
func ReplaceFetchedCatalog(modelsByProvider map[string][]ModelSpec, pricesByProvider map[string][]ModelPrice) {
	models := make(map[string][]ModelSpec, len(modelsByProvider))
	for providerID, specs := range modelsByProvider {
		models[providerID] = copiedModelSpecs(specs)
	}
	prices := make(map[string][]ModelPrice, len(pricesByProvider))
	for providerID, entries := range pricesByProvider {
		prices[providerID] = append([]ModelPrice(nil), entries...)
	}

	dynMu.Lock()
	defer dynMu.Unlock()
	fetchedModels = models
	fetchedModelPrices = prices
}

// fetchedModelsFor returns a copy of the models.dev models for a provider id.
func fetchedModelsFor(providerID string) []ModelSpec {
	dynMu.RLock()
	defer dynMu.RUnlock()
	return copiedModelSpecs(fetchedModels[providerID])
}
