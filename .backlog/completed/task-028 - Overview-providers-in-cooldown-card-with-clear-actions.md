---
id: TASK-028
title: 'Overview: providers-in-cooldown card with clear actions'
status: Done
assignee: []
created_date: '2026-09-17 10:02'
updated_date: '2026-09-17 10:14'
labels: []
dependencies: []
references:
  - frontend/src/pages/Overview.tsx
  - frontend/src/components/CooldownResetModal.tsx
  - backend/internal/gateway/admin_cooldowns.go
  - backend/internal/store/repo_routing.go
priority: medium
type: feature
ordinal: 36000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The dashboard Overview page shows traffic, spend, token composition and recent requests, but gives no visibility into providers whose accounts or models are parked on a dispatcher cooldown. Operators must navigate to ProviderHealth or a ProviderDetail page to discover a parked provider and can only reset from there, so a rate-limit/quota outage is easy to miss at a glance. Add a card between Token Composition and Recent requests that lists providers in cooldown (grouped by provider, one row per parked account) and exposes clear-all plus per-account clear.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 GET /api/cooldowns returns active account- and model-level cooldowns grouped by provider, tenant-scoped, excluding disabled and expired entries
- [x] #2 Each account row reports scope, models, inferred reason, backoff level, expiry and retry_after_seconds
- [x] #3 Overview renders the cooldown card between Token Composition and Recent requests, hiding the empty case with a clear empty state
- [x] #4 Clear all uses the existing tenant reset; per-account Clear uses the existing per-account reset and both survive re-render without stale data
- [x] #5 Backend and frontend tests/typecheck cover the new endpoint and card
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Store: add `ListActiveModelCooldowns(ctx, tenantID, now)` in `backend/internal/store/repo_routing.go` joining model_cooldowns to accounts, tenant-scoped and unexpired.
2. Gateway: add `adminListCooldowns` in `backend/internal/gateway/admin_cooldowns.go`; aggregate accounts + model cooldowns per account, group by provider, infer reason, compute expiry/retry, drop disabled/expired; register `GET /cooldowns` in `mountAdmin`.
3. Backend tests: store list test + handler test (active/expired, tenant isolation, grouping, reason mapping, disabled omitted, retry clamp).
4. Frontend: add `CooldownOverview` types + `api.cooldowns()` in `frontend/src/lib/api.ts`.
5. Frontend: new `frontend/src/components/CooldownCard.tsx` with query, provider groups, per-account Clear, Clear all via CooldownResetModal, toasts, invalidations, empty state.
6. Frontend: render `<CooldownCard providers={providers} />` in `frontend/src/pages/Overview.tsx` between TokenComposition and RecentActivityTable.
7. Verify: gofmt, make vet, make test, cd frontend && npm run typecheck, make build, ./scripts/verify.sh.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Backend: added RoutingRepo.ListActiveModelCooldowns (tenant-scoped, unexpired) and adminListCooldowns handler (merges account + model cooldowns per account, groups by provider, infers reason, computes expiry/retry, drops disabled/expired); registered GET /api/cooldowns. Refactored to pass Code Health (indexModelCooldowns, newAccountCooldownState, inferCooldownReason, cooldownProviderGroups). Frontend: added cooldown types + api.cooldowns(), new components/CooldownCard.tsx (grouped rows, per-account Clear, Clear all via CooldownResetModal, toasts, invalidations, empty state), rendered between TokenComposition and RecentActivityTable in Overview. Tests: store tenant/expiry test + 4 gateway handler tests. Verified gofmt, go vet, go test ./..., frontend typecheck + build, and ./scripts/verify.sh (exit 0, all checks passed).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Added a "Providers in cooldown" card to the dashboard Overview between Token Composition and Recent requests.

Backend:
- `RoutingRepo.ListActiveModelCooldowns` lists unexpired model cooldowns for a tenant (joined to accounts).
- `adminListCooldowns` merges each account-level cooldown with its model cooldowns into one row, groups by provider, infers a reason (credits > model rate limit > rate limit), computes expiry and `retry_after_seconds`, and omits disabled/expired entries.
- `GET /api/cooldowns` registered in `mountAdmin`.

Frontend:
- New `frontend/src/components/CooldownCard.tsx`: provider-grouped rows (label, reason badge, models, backoff, retry), per-account **Clear** (existing `/accounts/{id}/cooldown/reset`), **Clear all** via `CooldownResetModal` (existing `/health/cooldowns/reset`), toasts + query invalidation, empty state.
- Types + `api.cooldowns()` in `frontend/src/lib/api.ts`; card rendered in `frontend/src/pages/Overview.tsx`.

Verification: gofmt clean, `go vet ./...`, `go test ./...`, `npm run typecheck`, `npm run build`, and `./scripts/verify.sh` (exit 0, all local checks passed). Code Health pre-commit safeguard passes after refactor.
<!-- SECTION:FINAL_SUMMARY:END -->
