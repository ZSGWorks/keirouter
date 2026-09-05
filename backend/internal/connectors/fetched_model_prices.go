package connectors

import "strings"

// fetchedModelPrices holds the latest connected-provider models.dev price
// snapshot. It is separate from built-in provider pricing, which takes priority.
var fetchedModelPrices = map[string][]ModelPrice{}

// ReplaceFetchedModelPrices atomically replaces the complete models.dev price
// snapshot used by dashboard model details.
func ReplaceFetchedModelPrices(pricesByProvider map[string][]ModelPrice) {
	next := make(map[string][]ModelPrice, len(pricesByProvider))
	for providerID, prices := range pricesByProvider {
		if len(prices) == 0 {
			continue
		}
		next[providerID] = append([]ModelPrice(nil), prices...)
	}

	dynMu.Lock()
	defer dynMu.Unlock()
	fetchedModelPrices = next
}

// ModelDisplayPriceByProviderModel resolves built-in provider pricing first,
// then uses models.dev data when the provider catalog has no matching price.
func ModelDisplayPriceByProviderModel(provider, model string) (ModelPrice, bool) {
	if price, ok := ModelPriceByProviderModel(provider, model); ok {
		return price, true
	}
	return fetchedModelDisplayPrice(provider, model)
}

func fetchedModelDisplayPrice(provider, model string) (ModelPrice, bool) {
	dynMu.RLock()
	defer dynMu.RUnlock()
	return fetchedModelDisplayPriceFromSnapshot(fetchedModelPrices, provider, model)
}

func fetchedModelDisplayPriceFromSnapshot(pricesByProvider map[string][]ModelPrice, provider, model string) (ModelPrice, bool) {
	provider = normalizePriceProvider(provider)
	for _, prices := range pricesByProvider {
		for i := len(prices) - 1; i >= 0; i-- {
			price := prices[i]
			if normalizePriceProvider(price.Provider) == provider && priceMatchesModel(price.Model, model) {
				return price, true
			}
		}
	}
	return ModelPrice{}, false
}

func priceMatchesModel(priceModel, model string) bool {
	for _, candidate := range priceModelCandidates(model) {
		if strings.EqualFold(priceModel, candidate) {
			return true
		}
	}
	return false
}
