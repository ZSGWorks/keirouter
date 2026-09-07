package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/core"
)

// modelCacheTTL bounds how long a connected provider's model listing stays
// fresh. Entries are recomputed lazily after expiry and proactively warmed on
// startup and when a provider is connected.
const modelCacheTTL = 12 * time.Hour

type modelCacheEntry struct {
	models  []providerModelInfo
	expires time.Time
}

// modelCache is an in-memory, per-provider cache of model listings for
// connected providers only. Unconnected providers bypass the cache.
type modelCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]modelCacheEntry
}

func newModelCache() *modelCache {
	return &modelCache{ttl: modelCacheTTL, entries: map[string]modelCacheEntry{}}
}

// get returns the cached models for a provider when a fresh entry exists.
func (c *modelCache) get(provider string) ([]providerModelInfo, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[provider]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.models, true
}

// set stores a fresh listing for a provider, overwriting any prior entry.
func (c *modelCache) set(provider string, models []providerModelInfo) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[provider] = modelCacheEntry{models: models, expires: time.Now().Add(c.ttl)}
}

// invalidate drops a provider's entry (e.g. after disconnect or account
// removal) so the next read recomputes from scratch.
func (c *modelCache) invalidate(provider string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, provider)
}

// providerModelCache returns the server's model cache, initializing it once.
// sync.Once keeps concurrent first use (async warm hooks, non-New test
// servers) race-free; get/set/invalidate stay nil-safe regardless.
func (s *Server) providerModelCache() *modelCache {
	s.modelCacheOnce.Do(func() { s.modelCache = newModelCache() })
	return s.modelCache
}

// isConnectedProvider reports whether the provider has at least one usable
// account and is therefore eligible for the model cache.
func (s *Server) isConnectedProvider(ctx context.Context, providerID string) bool {
	return s.connectedProviderSet(ctx, adminTenant)[providerID]
}

// invalidateProviderModelCache drops a provider's cached listing after
// disconnect-style events (account delete/disable/needs-reconnect). Safe to
// call on nil caches and test servers without accounts.
func (s *Server) invalidateProviderModelCache(providerID string) {
	s.providerModelCache().invalidate(providerID)
}

// cachedProviderModels returns the cached listing for a connected provider
// on full (unfiltered) catalog reads.
func (s *Server) cachedProviderModels(connected bool, providerID string, kindFilter core.ServiceKind) ([]providerModelInfo, bool) {
	if !connected || kindFilter != "" {
		return nil, false
	}
	return s.providerModelCache().get(providerID)
}

// storeProviderModelCache persists a computed listing under the same rules.
func (s *Server) storeProviderModelCache(connected bool, providerID string, kindFilter core.ServiceKind, models []providerModelInfo) {
	if !connected || kindFilter != "" {
		return
	}
	s.providerModelCache().set(providerID, models)
}

// invalidateModelCacheIfDisabled drops a provider's cached listing when an
// account update disables it.
func (s *Server) invalidateModelCacheIfDisabled(disabled *bool, provider string) {
	if disabled != nil && *disabled {
		s.invalidateProviderModelCache(provider)
	}
}

// warmProviderModelCacheAsync refreshes a provider's cached listing in the
// background after connect-style events.
func (s *Server) warmProviderModelCacheAsync(providerID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.warmModelCache(ctx, providerID)
	}()
}

// connectedProvidersWithModels returns the subset of usable providers whose
// model listings should be cached (those with a connected account).
func (s *Server) connectedProvidersWithModels(ctx context.Context, tenantID string) []string {
	connected := s.connectedProviderSet(ctx, tenantID)
	out := make([]string, 0, len(connected))
	for provider := range connected {
		if _, ok := connectors.SpecByID(provider); ok {
			out = append(out, provider)
		}
	}
	return out
}

// warmModelCache computes and stores the model listing for one provider. It
// mirrors the compute path of adminProviderModels so cached entries always
// match what a cold read would produce.
func (s *Server) warmModelCache(ctx context.Context, providerID string) {
	// Background warm must never take the server down: recovery mirrors the
	// best-effort contract of the startup warm path.
	defer func() {
		if r := recover(); r != nil && s.log != nil {
			s.log.Warn("model cache warm panicked", "provider", providerID, "panic", r)
		}
	}()
	if s.db == nil {
		return
	}
	if !s.isConnectedProvider(ctx, providerID) {
		return
	}
	cache := s.providerModelCache()
	out, seen := s.catalogProviderModels(ctx, providerID, "")
	s.discoverProviderModels(ctx, providerID, "", &out, seen)
	cache.set(providerID, out)
}

// WarmModelCache warms model listings for every connected provider. It runs
// best-effort in the background and never fails startup.
func (s *Server) WarmModelCache(ctx context.Context) {
	if s.accounts == nil {
		return
	}
	providers := s.connectedProvidersWithModels(ctx, adminTenant)
	probes := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, provider := range providers {
		select {
		case probes <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(provider string) {
			defer wg.Done()
			defer func() { <-probes }()
			warmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			s.warmModelCache(warmCtx, provider)
		}(provider)
	}
	wg.Wait()
}
