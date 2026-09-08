---
id: TASK-024
title: 'Perf: request-path config caching (chains/aliases/settings/models/plan)'
status: Done
assignee: []
created_date: '2026-09-08 03:05'
updated_date: '2026-09-08 03:27'
labels:
  - performance
  - backend
milestone: Performance Scan 2026-09
dependencies: []
priority: high
type: enhancement
ordinal: 27000
---

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 chains/aliases/settings/allowed-models/plan lookups served from in-memory TTL cache on hot path; DB hit only on miss/expiry
- [ ] #2 Admin CRUD for these resources invalidates cache entries
- [ ] #3 Existing gateway tests pass; new test covers cache hit + invalidation
- [ ] #4 gofmt clean, make vet, make test pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
## Plan: request-path config caching

Goal: remove 5-8 DB roundtrips per proxy request for rarely-changing admin data.

1. Inspect hot-path call sites: resolve.go (chains/aliases), settings.go (endpoint+provider routing settings), handlers.go (plan, allowed models).
2. Inspect existing cache pattern: budget.go cachedListByScope; check for existing generic TTL cache in codebase (cache/ pkg, insights_cache.go).
3. Design: single small TTL cache type (package gateway or reuse existing), TTL ~5s, entries keyed by lookup key. Invalidate hooks in admin CRUD handlers for chains/aliases/settings/keys/allowed-models/plans.
4. Wrap lookups: chains.ListByTenant, aliases.Get, settings.Get (endpoint+provider routing), Plans().Get, GetAllowedModels.
5. Concurrency: RWMutex or sharded map; keep it tiny.
6. Tests: unit test for TTL cache (hit within TTL, miss after, invalidate); wire-level test proving repeat calls don't re-hit store (count via fake store if pattern exists).
7. Verify: gofmt, make vet, make test.

Risks: stale reads up to TTL after admin change → mitigated by explicit invalidation on CRUD paths. Multi-instance deployments unaffected (cache is per-process, DB still source of truth on miss).
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented generic configCache[T] (TTL 5s, bounded 256, oldest-expiry eviction) in gateway/config_cache.go; caches: chains per tenant, aliases (incl. not-found), endpoint settings, provider routing settings, plan limits (incl. not-found), per-key allowed models.

Hot path rewired: resolveTargets callers use cache-backed ChainSource/AliasSource; effectiveLimits + filterAllowedTargets + endpointPlanOptions/effectiveTokenSaving read via cache. Admin handlers keep fresh direct reads.

Invalidation hooks on all admin write paths: chains create/update/delete, aliases set/delete (incl. foreign + bulk import), endpoint + provider routing settings save, plans create/update/delete, keys create/update/delete.

Tests: TestConfigCacheHitAndExpiry, TestCachedChainSourceHitsCache, TestCachedAliasSourceCachesMiss, TestPlanLimitsCacheStoresNotFound. Full gateway suite + go test ./... green; gofmt/vet clean.

Side benefit noted: ChainRepo.ListByTenant does N+1 steps query per chain; cache absorbs it on hot path, deeper fix deferred.
<!-- SECTION:NOTES:END -->
