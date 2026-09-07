---
id: TASK-019
title: Connected-only chain model catalog + 12h model cache
status: Done
assignee: []
created_date: '2026-09-07 19:09'
updated_date: '2026-09-07 19:19'
labels:
  - chains
  - models
  - cache
dependencies: []
priority: medium
type: enhancement
ordinal: 22000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
## Goal\n\n1. Chain model catalog fetches only **connected** providers (≥1 enabled account, not `Disabled`, not `NeedsReconnect`). Unconnected providers (no API key) do not appear in the chain model list at all — not as models, not as custom-mode targets.\n2. Backend 12h in-memory cache of per-provider model lists for connected providers. Filled on startup, on provider connect; entries expire after 12h.\n\n## Plan\n\nApproved plan: /Users/zsolt.gajdacs/.plannotator/plans/chain-model-picker-fetch-conne-2026-09-07-approved.md\n\n### Backend\n- `adminListProviders` adds `connected` flag (shared helper extracted from `usableModelProviders`).\n- New `backend/internal/gateway/model_cache.go`: `modelCache` (12h TTL, per-provider entries), warm/get/invalidate.\n- `adminProviderModels`: read-through cache for connected providers; connect hooks warm (adminCreateAccount, bulk, OAuth); disconnect hooks invalidate (delete, disable, needs-reconnect).\n- Startup: background goroutine in `App.Run` warms all connected providers.\n\n### Frontend\n- `useModelCatalog` gains `{ connectedOnly?: boolean }`; `ChainModelPicker` filters providers + catalog to connected only.\n\nBranch: `feat/chain-models-connected-cache`
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Chain editor model picker lists only providers with a connected (enabled, not needs-reconnect) account — as models and as custom-mode targets
- [x] #2 Backend exposes `connected` flag on admin /providers entries
- [x] #3 adminProviderModels serves connected providers through a 12h in-memory cache (read-through); unconnected providers keep current uncached behavior
- [x] #4 Cache warmed on startup for connected providers, warmed asynchronously on account connect (single, bulk, OAuth), invalidated on account delete/disable/needs-reconnect
- [x] #5 Frontend useModelCatalog supports connectedOnly option; ChainModelPicker uses it
- [x] #6 go test ./..., make vet, frontend typecheck pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
## Connected-only chain model catalog + 12h model cache\n\n### Backend\n- `adminListProviders` now returns `connected` per provider, computed via `connectedProviderSet` (renamed from `usableModelProviders`; all call sites updated).\n- New `backend/internal/gateway/model_cache.go`: `modelCache` (12h TTL, per-provider `[]providerModelInfo` entries, mutex-guarded, nil-safe). Read-through in `adminProviderModels` for connected providers; unconnected providers keep uncached behavior and can never serve stale entries.\n- Connect hooks warm the cache asynchronously: `adminCreateAccount`, bulk import, OAuth connect, needs-reconnect cleared (admin test + credential-retry recovery).\n- Disconnect hooks invalidate: account delete, disable via `adminUpdateAccount`, needs-reconnect set on failed credential retry.\n- `App.Run` warms all connected providers in a bounded-concurrency background goroutine at startup; `warmModelCache` recovers panics so background warm is best-effort and never takes the server down.\n\n### Frontend\n- `Provider` gains `connected?: boolean`; `useModelCatalog` accepts `{ connectedOnly?: boolean }`, filtering providers before building per-provider model queries.\n- `ChainModelPicker` uses `connectedOnly: true` and filters its provider list (catalog entries and custom-model target selector) to connected providers — unconnected providers no longer appear anywhere in the chain model picker.\n\n### Tests\n- New `model_cache_test.go`: TTL expiry, invalidate, nil-safety, cache read-through for connected provider, unconnected bypass, `connected` flag on provider listing.\n- `go test ./...`, `go vet ./...`, `gofmt`, and frontend `npm run typecheck` all pass.\n\n> ponytail: keepalive-driven needs-reconnect (oauth/manager.go) does not invalidate the cache — read-through only engages connected providers, so stale entries are never served there; entries simply age out after 12h. Add a cross-package hook if eager invalidation becomes necessary.
<!-- SECTION:FINAL_SUMMARY:END -->
