---
id: TASK-007
title: Harden models.dev pricing and model catalog refresh
status: Done
assignee: []
created_date: '2026-09-01 16:50'
updated_date: '2026-09-02 08:11'
labels:
  - pricing
  - models-dev
  - catalog
dependencies: []
references:
  - 'https://models.dev/api.json'
modified_files:
  - backend/internal/pricing/modelsdev.go
  - backend/internal/app/app.go
  - backend/internal/gateway/admin.go
  - backend/internal/connectors/dynamic.go
  - backend/internal/connectors/fetched_models.go
  - backend/internal/connectors/models.go
  - backend/internal/connectors/fetched_models_test.go
priority: high
type: bug
ordinal: 7000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Use models.dev as an additive live metadata source while preserving static catalog and custom-price fallback. Current implementation needs review findings resolved before completion: provider mapping, manual refresh semantics, cancellation-safe lifecycle, stale fetched-model cleanup, and connected-provider visibility.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Mapped models.dev providers use valid KeiRouter provider IDs
- [x] #2 Manual pricing refresh fetches and applies current models.dev data
- [x] #3 Pricing refresh exits before application shutdown closes the database
- [x] #4 Successful catalog refresh removes models no longer supplied by models.dev
- [x] #5 Fetched models appear only for connected providers and preserve static and custom catalog entries
- [x] #6 Variable or invalid upstream rates are not reported as priced
- [x] #7 Relevant unit and integration tests pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
- [x] #4 Review findings resolved
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Correct models.dev projection: map `google` to the catalog's `gemini` provider and reject any negative input/output/cache/long-context rate before it enters `ModelPrice`.
2. Extract one app-owned catalog-refresh coordinator. It will derive its 90-second timeout from caller context, fetch/revalidate models.dev, filter prices/models to providers with enabled default-tenant accounts using the existing account-repository convention, atomically replace fetched model data, then invoke the existing price reload/backfill path.
3. Add `RefreshPricingCatalog` gateway dependency and route the admin POST through that coordinator; retain `ReloadPricing` for custom-model CRUD so custom edits do not create remote fetches.
4. Replace the per-provider fetched-model mutation API with atomic full-snapshot replacement. A successful refresh clears providers absent from the next snapshot while leaving static and custom stores intact.
5. Merge static, fetched, and custom models by `(model ID, service kind)` so fetched multimodal variants are preserved and custom entries only override the matching variant.
6. Extend pricing, connector, app, and gateway tests for mappings, invalid rates, snapshot replacement, capability variants, account filtering, cancellation, and forced endpoint refresh. Run gofmt, focused tests, `make vet`, full backend tests, Code Health safeguard, and `./scripts/verify.sh` before finalizing.

7. Add coordinator-level app tests by extracting refresh behavior into an injectable unit. Cover successful fetch/filter/publish, cancellation while waiting on refresh serialization, cancellation during fetch, and failure preservation. Keep gateway handler coverage focused on forwarding caller context and status. Re-run focused/backend verification and record results. Plan approved in Plannotator on 2026-09-02.

8. Restore Code Health without rule overrides. Extract app pricing wiring/conversions, catalog merge/projection helpers, dynamic-model storage helpers, and focused test helpers; leave stable but pre-existing oversized gateway admin module untouched. Re-run Code Health safeguard until all changed files are non-degraded. Plan approved in Plannotator on 2026-09-02.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented projection validation, atomic fetched-model snapshots, connected-account filtering, cancellation-aware refresh coordination, forced admin refresh, and capability-kind merge behavior. Added focused pricing, connector, app filtering, and gateway handler tests.

Verification: `go vet ./backend/...`, `go test ./backend/... -count=1`, and focused package tests passed. `./scripts/verify.sh` reached Docker image build but failed pulling `node:20-alpine` and `golang:1.26-alpine` from Docker Hub with EOF; all earlier Go, frontend, and PostgreSQL checks passed.

Final diff review reported no concrete correctness issues. Code Health safeguard remains failed due pre-existing/new-file complexity thresholds in the in-progress models.dev integration; task remains In Progress and DoD Codehealth is not checked.

Closed task-7 coverage gaps by extracting an injectable pricing catalog refresher. New coordinator tests prove connected-provider filtering across disjoint price/model provider sets, prices-before-model publication, failed refresh preservation, no model publication after price replacement failure, cancellation during fetch, lock-wait cancellation, and background refresher shutdown. Gateway test now proves canceled request-context forwarding and success JSON payload. Final verification passed: `go test ./backend/... -count=1`, `go vet ./backend/...`, and `git diff --check`. Final focused test review found no issues. Code Health remains the only unchecked DoD item.

Code Health refactor completed: extracted Build pricing callback wiring and model-price layers, model merge phases, shared dynamic/fetched model storage copies, models.dev projection/cost helpers, and focused test assertions. Left gateway/admin.go production unchanged per approved scope. Verification passed: gofmt, go test ./backend/internal/app ./backend/internal/connectors ./backend/internal/pricing ./backend/internal/gateway, git diff --check. Code Health safeguard improved app.go and cleared models.go/modelsdev.go complexity findings, but remains failed for prohibited gateway/admin.go change and dynamic.go module-level string-heavy-arguments threshold.

Moved fetched-model storage from `connectors/dynamic.go` into `connectors/fetched_models.go`. Per-provider updates now use `FetchedModelUpdate`; snapshot replacement, shared locking, and defensive copies are preserved. Verified with `gofmt`, `make vet`, `make test`, `git diff --check`, CodeHealth reviews (10.0 for both connector files), and the pre-commit safeguard (passed).

Post-review cache isolation fix: `pricing.Fetcher.CachedPrices` and `CachedModels` now return deep copies, preventing callers from mutating fetcher-owned snapshots. Focused pricing tests and Code Health safeguard passed afterward. Custom-model read failures remain fail-closed by design so unknown custom prices cannot silently fall back to lower-precedence rates.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Completed models.dev catalog hardening and Code Health cleanup. Fetched models now use an isolated, lock-protected store with defensive copies, atomic snapshot replacement, and a struct-based per-provider update API. Catalog refresh behavior remains covered by backend tests.

Verification: `gofmt`, `make vet`, `make test`, `git diff --check`, per-file Code Health reviews, and CodeHealth pre-commit safeguard all passed.
<!-- SECTION:FINAL_SUMMARY:END -->
