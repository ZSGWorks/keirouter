package app

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/pricing"
	"github.com/mydisha/keirouter/backend/internal/store"
)

type fakeProviderAccountLister struct {
	accounts []store.Account
	err      error
	onList   func(context.Context)
}

func (f fakeProviderAccountLister) ListByProviders(ctx context.Context, _ string, providers []string) ([]store.Account, error) {
	if f.onList != nil {
		f.onList(ctx)
	}
	if f.err != nil {
		return nil, f.err
	}
	connected := make([]store.Account, 0, len(f.accounts))
	for _, account := range f.accounts {
		if slices.Contains(providers, account.Provider) {
			connected = append(connected, account)
		}
	}
	return connected, nil
}

func TestFilterFetchedModelsKeepsConnectedProviders(t *testing.T) {
	fetched := map[string][]connectors.ModelSpec{
		"openrouter": {{ID: "connected", Kind: core.ServiceLLM}},
		"venice":     {{ID: "unconnected", Kind: core.ServiceLLM}},
	}
	got, err := filterFetchedModels(context.Background(), fakeProviderAccountLister{
		accounts: []store.Account{{Provider: "openrouter"}},
	}, fetched)
	if err != nil {
		t.Fatalf("filter fetched models: %v", err)
	}
	if len(got) != 1 || len(got["openrouter"]) != 1 {
		t.Fatalf("filtered models = %v, want only openrouter", got)
	}
}

func TestFilterFetchedModelsPreservesCurrentSnapshotOnAccountLookupError(t *testing.T) {
	_, err := filterFetchedModels(context.Background(), fakeProviderAccountLister{err: errors.New("database unavailable")}, map[string][]connectors.ModelSpec{
		"openrouter": {{ID: "model", Kind: core.ServiceLLM}},
	})
	if err == nil {
		t.Fatal("expected account lookup error")
	}
}

func TestFilterFetchedPricesSkipsUnconnectedProviders(t *testing.T) {
	got, err := filterFetchedPrices(context.Background(), fakeProviderAccountLister{
		accounts: []store.Account{{Provider: "openrouter"}},
	}, map[string][]connectors.ModelPrice{
		"openrouter": {{Provider: "openrouter", Model: "connected", InputPerM: 1}},
		"venice":     {{Provider: "venice", Model: "unconnected", InputPerM: 1}},
	})
	if err != nil {
		t.Fatalf("filter fetched prices: %v", err)
	}
	if len(got) != 1 || len(got["openrouter"]) != 1 {
		t.Fatalf("filtered prices = %v, want only openrouter", got)
	}
}

func TestFilterFetchedPricesReturnsAccountLookupError(t *testing.T) {
	_, err := filterFetchedPrices(context.Background(), fakeProviderAccountLister{err: errors.New("database unavailable")}, map[string][]connectors.ModelPrice{
		"openrouter": {{Provider: "openrouter", Model: "model", InputPerM: 1}},
	})
	if err == nil {
		t.Fatal("expected account lookup error")
	}
}

func TestPricingCatalogRefresherPublishesConnectedSnapshotAfterPrices(t *testing.T) {
	var events []string
	var publishedPrices map[string][]connectors.ModelPrice
	var publishedModels map[string][]connectors.ModelSpec
	r := connectedCatalogRefresher(t, &events, &publishedPrices, &publishedModels)

	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	assertPublishedCatalog(t, publishedPrices, publishedModels, events)
}

func TestPricingCatalogRefresherPublishesConnectedCatalogAtomically(t *testing.T) {
	var events []string
	var publishedPrices map[string][]connectors.ModelPrice
	var publishedModels map[string][]connectors.ModelSpec
	r := connectedCatalogRefresher(t, &events, &publishedPrices, &publishedModels)
	legacyModelPublishCalled := false
	r.replaceModels = func(map[string][]connectors.ModelSpec) { legacyModelPublishCalled = true }
	r.replaceCatalog = func(models map[string][]connectors.ModelSpec, prices map[string][]connectors.ModelPrice) {
		events = append(events, "catalog")
		publishedModels = models
		publishedPrices = prices
	}

	if err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if legacyModelPublishCalled {
		t.Fatal("published models through separate legacy callback")
	}
	assertPublished(t, publishedPrices, "openrouter", func(price connectors.ModelPrice) bool { return price.Model == "price-only" })
	assertPublished(t, publishedModels, "venice", func(model connectors.ModelSpec) bool { return model.ID == "model-only" })
	if !slices.Equal(events, []string{"prices", "catalog"}) {
		t.Fatalf("publication order = %v, want prices then catalog", events)
	}
}

func connectedCatalogRefresher(t *testing.T, events *[]string, publishedPrices *map[string][]connectors.ModelPrice, publishedModels *map[string][]connectors.ModelSpec) pricingCatalogRefresher {
	t.Helper()
	return pricingCatalogRefresher{
		fetch:    func(context.Context) (pricing.Result, error) { return connectedCatalogResult(), nil },
		accounts: fakeProviderAccountLister{accounts: []store.Account{{Provider: "openrouter"}, {Provider: "venice"}}, onList: requireDeadline(t)},
		replacePrices: func(ctx context.Context, prices map[string][]connectors.ModelPrice) error {
			requireDeadline(t)(ctx)
			*events = append(*events, "prices")
			*publishedPrices = prices
			return nil
		},
		replaceModels: func(models map[string][]connectors.ModelSpec) {
			*events = append(*events, "models")
			*publishedModels = models
		},
		setAppliedPrices: func(map[string][]connectors.ModelPrice) {}, log: slog.Default(), lock: make(chan struct{}, 1),
	}
}

func connectedCatalogResult() pricing.Result {
	return pricing.Result{Prices: map[string][]connectors.ModelPrice{"openrouter": {{Provider: "openrouter", Model: "price-only", InputPerM: 1}}, "mistral": {{Provider: "mistral", Model: "unconnected-price", InputPerM: 1}}}, Models: map[string][]connectors.ModelSpec{"venice": {{ID: "model-only", Kind: core.ServiceLLM}}, "mistral": {{ID: "unconnected-model", Kind: core.ServiceLLM}}}, Providers: 2}
}

func requireDeadline(t *testing.T) func(context.Context) {
	t.Helper()
	return func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("refresh callback missing deadline")
		}
	}
}

func assertPublishedCatalog(t *testing.T, prices map[string][]connectors.ModelPrice, models map[string][]connectors.ModelSpec, events []string) {
	t.Helper()
	assertPublished(t, prices, "openrouter", func(price connectors.ModelPrice) bool { return price.Model == "price-only" })
	assertPublished(t, models, "venice", func(model connectors.ModelSpec) bool { return model.ID == "model-only" })
	if !slices.Equal(events, []string{"prices", "models"}) {
		t.Fatalf("publication order = %v, want prices then models", events)
	}
}

func assertPublished[T any](t *testing.T, values map[string][]T, provider string, matches func(T) bool) {
	t.Helper()
	if len(values) != 1 {
		t.Fatalf("published values = %v, want one provider", values)
	}
	if len(values[provider]) != 1 {
		t.Fatalf("published values = %v, want one %s value", values, provider)
	}
	if !matches(values[provider][0]) {
		t.Fatalf("published values = %v, want matching %s value", values, provider)
	}
}

func TestPricingCatalogRefresherPreservesPublishedSnapshotOnFailure(t *testing.T) {
	oldPrices := map[string][]connectors.ModelPrice{"openrouter": {{Model: "old"}}}
	oldModels := map[string][]connectors.ModelSpec{"openrouter": {{ID: "old", Kind: core.ServiceLLM}}}
	prices, models := oldPrices, oldModels
	r := pricingCatalogRefresher{
		fetch: func(context.Context) (pricing.Result, error) {
			return pricing.Result{Models: map[string][]connectors.ModelSpec{"openrouter": {{ID: "new", Kind: core.ServiceLLM}}}}, nil
		},
		accounts: fakeProviderAccountLister{err: errors.New("database unavailable")},
		replacePrices: func(_ context.Context, next map[string][]connectors.ModelPrice) error {
			prices = next
			return nil
		},
		replaceModels:    func(next map[string][]connectors.ModelSpec) { models = next },
		setAppliedPrices: func(map[string][]connectors.ModelPrice) {},
		log:              slog.Default(),
		lock:             make(chan struct{}, 1),
	}

	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("expected account lookup failure")
	}
	if prices["openrouter"][0].Model != "old" || models["openrouter"][0].ID != "old" {
		t.Fatalf("failed refresh changed published snapshot: prices=%v models=%v", prices, models)
	}
}

func TestPricingCatalogRefresherDoesNotPublishModelsWhenPriceReplacementFails(t *testing.T) {
	publishedModels := map[string][]connectors.ModelSpec{"openrouter": {{ID: "old", Kind: core.ServiceLLM}}}
	applied := false
	r := pricingCatalogRefresher{
		fetch: func(context.Context) (pricing.Result, error) {
			return pricing.Result{
				Prices: map[string][]connectors.ModelPrice{"openrouter": {{Provider: "openrouter", Model: "new", InputPerM: 1}}},
				Models: map[string][]connectors.ModelSpec{"openrouter": {{ID: "new", Kind: core.ServiceLLM}}},
			}, nil
		},
		accounts: fakeProviderAccountLister{accounts: []store.Account{{Provider: "openrouter"}}},
		replacePrices: func(context.Context, map[string][]connectors.ModelPrice) error {
			return errors.New("meter unavailable")
		},
		replaceModels:    func(models map[string][]connectors.ModelSpec) { publishedModels = models },
		setAppliedPrices: func(map[string][]connectors.ModelPrice) { applied = true },
		log:              slog.Default(),
		lock:             make(chan struct{}, 1),
	}

	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("expected price replacement failure")
	}
	if publishedModels["openrouter"][0].ID != "old" {
		t.Fatalf("price failure published models: %v", publishedModels)
	}
	if applied {
		t.Fatal("price failure recorded un-applied pricing snapshot")
	}
}

func TestPricingCatalogRefresherStopsWhenContextCanceled(t *testing.T) {
	started := make(chan struct{})
	r := pricingCatalogRefresher{
		fetch: func(ctx context.Context) (pricing.Result, error) {
			close(started)
			<-ctx.Done()
			return pricing.Result{}, ctx.Err()
		},
		setAppliedPrices: func(map[string][]connectors.ModelPrice) {},
		log:              slog.Default(),
		lock:             make(chan struct{}, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Refresh(ctx) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("refresh error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh did not stop after cancellation")
	}
}

func TestPricingCatalogRefresherDoesNotStartWhenCanceledWhileQueued(t *testing.T) {
	lock := make(chan struct{}, 1)
	lock <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- acquirePricingRefreshLock(ctx, lock, func() { close(waiting) }) }()
	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("queued lock acquisition did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("refresh error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued lock acquisition did not stop after cancellation")
	}
}

func TestRunPricingRefresherReturnsAfterCancellationDuringRefresh(t *testing.T) {
	started := make(chan struct{})
	a := App{
		log: slog.Default(),
		refreshPricingCatalog: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.runPricingRefresher(ctx)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("pricing refresher did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pricing refresher did not stop after cancellation")
	}
}
