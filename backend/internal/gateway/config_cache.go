package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/mydisha/keirouter/backend/internal/limits"
	"github.com/mydisha/keirouter/backend/internal/store"
)

// configCacheTTL bounds how long request-path config lookups (chains, aliases,
// endpoint/provider settings, plans, per-key model access) may be served from
// memory. Admin mutations invalidate explicitly, so the TTL is only a safety
// net for writes that bypass the admin handlers (e.g. direct DB edits).
const configCacheTTL = 5 * time.Second

// configCacheMaxEntries caps each cache to keep memory bounded; eviction is
// oldest-expiry-first, mirroring the identity auth cache.
const configCacheMaxEntries = 256

// configCache is a tiny generic TTL cache. Entries are positive-only:
// errors and not-found results are never cached, so a missing chain or alias
// keeps hitting the store until it exists.
type configCache[T any] struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]configCacheEntry[T]
}

type configCacheEntry[T any] struct {
	value   T
	expires time.Time
}

func newConfigCache[T any]() *configCache[T] {
	return &configCache[T]{ttl: configCacheTTL, entries: make(map[string]configCacheEntry[T])}
}

func (c *configCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var zero T
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expires) {
		if ok {
			delete(c.entries, key)
		}
		return zero, false
	}
	return e.value, true
}

func (c *configCache[T]) set(key string, value T) {
	c.mu.Lock()
	if len(c.entries) >= configCacheMaxEntries {
		var oldest string
		var oldestExpiry time.Time
		for k, v := range c.entries {
			if oldest == "" || v.expires.Before(oldestExpiry) {
				oldest, oldestExpiry = k, v.expires
			}
		}
		if oldest != "" {
			delete(c.entries, oldest)
		}
	}
	c.entries[key] = configCacheEntry[T]{value: value, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *configCache[T]) invalidate() {
	c.mu.Lock()
	c.entries = make(map[string]configCacheEntry[T])
	c.mu.Unlock()
}

// Server-level caches. Server literals in tests may leave these nil, so all
// accessors nil-check before use and fall through to the store.
func (s *Server) chainsCache() *configCache[[]store.Chain] {
	s.configCacheMu.Lock()
	defer s.configCacheMu.Unlock()
	if s.chainCache == nil {
		s.chainCache = newConfigCache[[]store.Chain]()
	}
	return s.chainCache
}

func (s *Server) aliasesCache() *configCache[aliasLookup] {
	s.configCacheMu.Lock()
	defer s.configCacheMu.Unlock()
	if s.aliasCache == nil {
		s.aliasCache = newConfigCache[aliasLookup]()
	}
	return s.aliasCache
}

func (s *Server) endpointSettingsCache() *configCache[EndpointSettings] {
	s.configCacheMu.Lock()
	defer s.configCacheMu.Unlock()
	if s.esCache == nil {
		s.esCache = newConfigCache[EndpointSettings]()
	}
	return s.esCache
}

func (s *Server) providerRoutingCache() *configCache[ProviderRoutingSettings] {
	s.configCacheMu.Lock()
	defer s.configCacheMu.Unlock()
	if s.prCache == nil {
		s.prCache = newConfigCache[ProviderRoutingSettings]()
	}
	return s.prCache
}

func (s *Server) planLimitsCache() *configCache[planLimitsLookup] {
	s.configCacheMu.Lock()
	defer s.configCacheMu.Unlock()
	if s.planCache == nil {
		s.planCache = newConfigCache[planLimitsLookup]()
	}
	return s.planCache
}

func (s *Server) allowedModelsCache() *configCache[[]string] {
	s.configCacheMu.Lock()
	defer s.configCacheMu.Unlock()
	if s.allowedCache == nil {
		s.allowedCache = newConfigCache[[]string]()
	}
	return s.allowedCache
}

// aliasLookup caches an alias row including its not-found state: a repeated
// miss for the same unknown model must not re-query the store on every request
// (clients frequently route to aliases that simply do not exist yet).
type aliasLookup struct {
	alias store.ModelAlias
	found bool
}

// planLimitsLookup caches the effective limits derived from a plan, including
// the not-found state.
type planLimitsLookup struct {
	limits limits.EffectiveLimits
	found  bool
}

// invalidateConfigCaches clears every request-path config cache. Admin write
// handlers call this after a successful mutation so routing changes take
// effect on the next request.
func (s *Server) invalidateConfigCaches() {
	s.configCacheMu.Lock()
	defer s.configCacheMu.Unlock()
	if s.chainCache != nil {
		s.chainCache.invalidate()
	}
	if s.aliasCache != nil {
		s.aliasCache.invalidate()
	}
	if s.esCache != nil {
		s.esCache.invalidate()
	}
	if s.prCache != nil {
		s.prCache.invalidate()
	}
	if s.planCache != nil {
		s.planCache.invalidate()
	}
	if s.allowedCache != nil {
		s.allowedCache.invalidate()
	}
}

// cachedChainSource adapts the chains repo into a ChainSource backed by the
// per-tenant chain-list cache.
type cachedChainSource struct {
	s *Server
}

func (c cachedChainSource) ListByTenant(ctx context.Context, tenantID string) ([]store.Chain, error) {
	if c.s.chains == nil {
		return nil, store.ErrNotFound
	}
	cache := c.s.chainsCache()
	if chains, ok := cache.get(tenantID); ok {
		return chains, nil
	}
	chains, err := c.s.chains.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	cache.set(tenantID, chains)
	return chains, nil
}

// cachedAliasSource adapts the aliases repo into an AliasSource backed by the
// alias cache (positive and not-found entries).
type cachedAliasSource struct {
	s *Server
}

func (c cachedAliasSource) Get(ctx context.Context, alias string) (store.ModelAlias, error) {
	if c.s.aliases == nil {
		return store.ModelAlias{}, store.ErrNotFound
	}
	cache := c.s.aliasesCache()
	if lookup, ok := cache.get(alias); ok {
		if !lookup.found {
			return store.ModelAlias{}, store.ErrNotFound
		}
		return lookup.alias, nil
	}
	rec, err := c.s.aliases.Get(ctx, alias)
	if err != nil {
		if err == store.ErrNotFound {
			cache.set(alias, aliasLookup{found: false})
		}
		return store.ModelAlias{}, err
	}
	cache.set(alias, aliasLookup{alias: rec, found: true})
	return rec, nil
}

// chainSource returns the cache-backed ChainSource used on the request path.
func (s *Server) chainSource() ChainSource { return cachedChainSource{s: s} }

// aliasSource returns the cache-backed AliasSource used on the request path.
func (s *Server) aliasSource() AliasSource { return cachedAliasSource{s: s} }

// cachedEffectiveLimits resolves per-key effective limits, caching the plan
// lookup by plan ID. Keys without a plan use the static config defaults and
// skip the cache entirely.
func (s *Server) cachedEffectiveLimits(ctx context.Context, key store.APIKey) (limits.EffectiveLimits, error) {
	if key.PlanID == "" {
		return limits.EffectiveLimits{
			RPM:         s.cfg.Limits.DefaultRPM,
			TPM:         s.cfg.Limits.DefaultTPM,
			Concurrency: s.cfg.Limits.DefaultConcurrency,
		}, nil
	}
	cache := s.planLimitsCache()
	if lookup, ok := cache.get(key.PlanID); ok {
		if !lookup.found {
			return limits.EffectiveLimits{}, store.ErrNotFound
		}
		return lookup.limits, nil
	}
	plan, err := s.db.Plans().Get(ctx, key.PlanID)
	if err != nil {
		if err == store.ErrNotFound {
			cache.set(key.PlanID, planLimitsLookup{found: false})
		}
		return limits.EffectiveLimits{}, err
	}
	limits := limits.EffectiveLimits{
		RPM:         plan.RPMLimit,
		TPM:         plan.TPMLimit,
		Concurrency: plan.ConcurrencyLimit,
	}
	cache.set(key.PlanID, planLimitsLookup{limits: limits, found: true})
	return limits, nil
}

// cachedAllowedModels returns the per-key allowed-models list via the cache.
// An empty result means unrestricted, matching the store semantics.
func (s *Server) cachedAllowedModels(ctx context.Context, keyID string) ([]string, error) {
	cache := s.allowedModelsCache()
	if models, ok := cache.get(keyID); ok {
		return models, nil
	}
	models, err := s.identity.Keys().GetAllowedModels(ctx, keyID)
	if err != nil {
		return nil, err
	}
	cache.set(keyID, models)
	return models, nil
}
