---
id: TASK-030
title: >-
  Make Provider Health dashboard time selector actually change the aggregation
  window
status: Done
assignee: []
created_date: '2026-09-17 12:25'
updated_date: '2026-09-17 13:18'
labels: []
dependencies: []
modified_files:
  - backend/internal/health/telemetry.go
  - backend/internal/health/chain_test.go
  - backend/internal/health/telemetry_test.go
  - backend/internal/config/config.go
  - backend/internal/app/app.go
  - backend/internal/gateway/provider_health.go
  - backend/internal/gateway/provider_health_chains_test.go
  - config.example.yaml
  - frontend/src/pages/ProviderHealth.tsx
  - frontend/src/lib/api.ts
type: bug
ordinal: 36000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Provider Health dashboard defaults to a 1h range but the backend endpoints behind the Providers, Models, and Chains tabs ignore the ?range= query parameter entirely: they only serve data aggregated over the provider-health rolling window (15m default), and the window descriptor honestly reports that mismatch. So the 1h default actually shows 15 minutes of data and changing the selector does nothing. Only the Probes tab and the provider detail page honor range today because they query persisted tables. Decision: serve range aggregation from in-memory minute buckets capped at 24h (no schema changes); status/score stay live-rolling while traffic metrics follow the selected range. Also fold in maintainability refactors: shared aggregation path for overview/models, deduplicated chain-stats lookup, single range-resolution helper, and typed response structs for the touched handlers.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Provider Health dashboard default 1h view aggregates a full hour of data, not the 15m rolling window
- [x] #2 Selecting a different range changes the numbers shown in Providers, Models, and Chains tabs
- [x] #3 Range aggregation is served from in-memory minute buckets capped at 24h; buckets older than the cap are pruned (chain buckets too, fixing existing unbounded growth)
- [x] #4 Chain stats endpoints (/health/chains, /health/chains/{id}) aggregate over the requested range
- [x] #5 Overview/models endpoints report request counts, success/error rates, fallback counts, and latency p95 for the selected range while status/score/main issue remain live-rolling
- [x] #6 Window descriptor in API responses reports the applied window (requested range clamped to the 24h cap) instead of only the collector rolling window
- [x] #7 Frontend range selector drops options beyond the 24h cap (7d removed)
- [x] #8 Bucket retention extension includes pruning old chain buckets and freezing latency/TTFT samples past the rolling window so memory stays bounded
- [x] #9 Tests cover window-parameterized chain stats, retention/pruning, sample freezing, and per-range endpoint differences; existing window-descriptor tests updated
- [x] #10 gofmt, make vet, make test, frontend npm run typecheck, and ./scripts/verify.sh all pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
## Implementation plan (approved)

Data model: all range aggregation from in-memory minute buckets capped at 24h (new health.Config.MaxHistoryWindow, default 24h, koanf max_history_window). Status/score/main issue stay live-rolling; traffic metrics follow selected range.

1. backend/internal/health/telemetry.go: MaxHistoryWindow config + default/clamp; retain keyState buckets to MaxHistoryWindow; prune chain buckets past MaxHistoryWindow (fixes unbounded growth); freeze latency/TTFT samples into p50/p95/p99 scalars once a bucket ages past RollingWindow and is persisted; new ProviderStat + ProviderStatsSince(window); ChainStatsSince(window) with ChainStats() kept as wrapper; EffectiveWindow clamp helper.
2. backend/internal/config/config.go + app/app.go + config.example.yaml: max_history_window wiring.
3. backend/internal/gateway/provider_health.go: resolveHealthRange single range-resolution path; typed healthWindow response (kind in_memory_history, applied duration, requested_range, since); typed response structs for overview/models/chains; overview+models take metrics from ProviderStatsSince (fallback to current-table metrics when telemetry disabled, via shared providerMetrics helper that also replaces adminHealthProviderDetail's duplicated math); chains+chain detail use ChainStatsSince; chainStatsByChainID helper dedupes the map build.
4. frontend: ProviderHealth.tsx gets OVERVIEW_RANGES (5m..24h, no 7d) for the overview selector while detail page keeps full RANGES (persisted data); ChainDetail modal threads the selected range through api.healthChainDetail(id, range); api.ts widens window kind union.
5. Tests: chain_test.go ChainStatsSince window filtering; telemetry_test.go retention/freeze/prune; provider_health_chains_test.go window descriptor updated to applied window; gateway range-difference test using Record + Eventually.

Verification: gofmt, make vet, make test, frontend npm run typecheck, ./scripts/verify.sh.

6. Code-health follow-up (during execution): extracted pure helpers to remove the complexity the change introduced - normalizeConfig, markFinalFailure/recordProviderAttempt/recordChainEvent/appendSamples, collectSnapshotWrites/pruneChainBuckets/persistSnapshots, providerStatFor, defaultProviderHealthConfig, buildProviderHealthConfig. CodeScene pre-commit gate now passes with telemetry.go/provider_health.go/config.go/app.go all improved or stable.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementation notes: freeze requires two flushSnapshots passes - the first marks snapshottedRevision after the DB write, the second sees revision==snapshottedRevision and freezes/prunes. Tests account for this. Chain buckets are counter-only and never persisted, so they are pruned directly past MaxHistoryWindow. Gateway chain-detail handler already accepted ?range=; only the frontend needed to thread it. Pre-existing gofmt-unformatted files (health/probe.go, app/bootstrap.go) were left untouched to keep the diff scoped.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
## Summary

Provider Health dashboard time selector now actually changes the aggregation window. Providers, Models, Chains tabs and the chain-detail modal aggregate traffic over the selected `?range=`, while status/score/main-issue stay live-rolling.

## Approach

- Range aggregation served from in-memory per-minute buckets retained up to `ProviderHealthConfig.MaxHistoryWindow` (new `max_history_window`, default 24h). Raw latency/TTFT samples pool only inside the rolling window; once a persisted bucket ages past the rolling window its samples freeze into p50/p95/p99 scalars, and buckets (provider and chain) are pruned past the retained history window - fixing pre-existing unbounded chain-bucket growth.
- New `Service.ProviderStatsSince` / `ChainStatsSince` + `EffectiveWindow` clamp; `ChainStats()` kept as a rolling-window wrapper.
- Gateway: single `resolveHealthRange` path, typed `healthWindow` descriptor (`kind: in_memory_history|unavailable`, applied duration, requested range, since) and typed overview/models/chains responses. Overview/models take traffic metrics from `ProviderStatsSince` while status stays live.
- Frontend: `OVERVIEW_RANGES` drops the 7d option (clamped to 24h cap); chain-detail modal threads the selected range via `api.healthChainDetail(id, range)`; `HealthOverviewWindow.kind` union widened.

## Tests

- `health/chain_test.go`: `TestChainStatsSince_Window` (5m excludes a 20m-old event; 1h includes it; `ChainStats()` uses rolling).
- `health/telemetry_test.go`: `TestHealthBucketFreezeAndPruneBoundsMemory` (freeze discards raw samples, frozen p95 preserved, aged bucket excluded from rolling-only window, bucket past MaxHistoryWindow pruned).
- `gateway/provider_health_chains_test.go`: window descriptor reports applied span, clamps 7d to retained history, zero-duration when telemetry disabled, and a range-difference test proving `?range=1h` counts a 20m-old event while `?range=5m` does not.

## Verification

`gofmt`, `make vet`, `make test`, `cd frontend && npm run typecheck`, `./scripts/verify.sh` all pass. CodeScene pre-commit gate passes with all touched backend files improved or stable.

## Notes

Freezing needs two `flushSnapshots` passes (persist marks revision, next pass freezes/prunes). Untouched pre-existing gofmt-unformatted files (`health/probe.go`, `app/bootstrap.go`) left out of scope.
<!-- SECTION:FINAL_SUMMARY:END -->
