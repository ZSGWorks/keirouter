---
id: TASK-027.01
title: Cooldown-reset API endpoints
status: Done
assignee: []
created_date: '2026-09-13 10:49'
updated_date: '2026-09-13 10:53'
labels: []
dependencies: []
parent_task_id: TASK-027
priority: medium
type: feature
ordinal: 32000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The frontend reset buttons need HTTP endpoints to call. No route today clears model cooldowns, so operators have no programmatic path to unpark targets deliberately.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A per-account reset clears that account's dispatcher cooldown and its model cooldowns, scoped to the requesting tenant
- [x] #2 A tenant-wide reset clears all account and model cooldowns for the requesting tenant
- [x] #3 Resetting when nothing is parked succeeds with zero counts and changes nothing
- [x] #4 Health history, probe results, reconnect flags, and disabled state are untouched
- [x] #5 Handler and store tests cover the reset behavior including cross-tenant rejection
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Backend cooldown-reset API. Research: single-tenant admin API (adminTenant const, admin.go:143); routes in mountAdmin (admin.go:24); Server has s.db + s.accounts (server.go:47); handlers call repos directly; tests build Server{db,...} with in-memory sqlite and call handlers directly (provider_health_chains_test.go). model_cooldowns keyed by account_id only (no tenant column) so tenant-wide model clear needs account-subquery. ClearAccountModelCooldowns has zero callers — safe to extend signature with row count. Steps: 1) store: add AccountRepo.ClearTenantCooldowns + RoutingRepo.ClearTenantModelCooldowns (tenant-scoped, return counts); extend ClearAccountModelCooldowns to return (int64, error). 2) gateway: new admin_cooldowns.go with adminResetAccountCooldown (GET→404 on missing/foreign tenant, compose ResetBackoffLevel + ClearAccountModelCooldowns, honest cleared_accounts 1/0 from pre-state) + adminResetTenantCooldowns; register POST /accounts/{id}/cooldown/reset and POST /health/cooldowns/reset in mountAdmin; response {cleared_accounts, cleared_models}. 3) tests: new admin_cooldowns_test.go via chi router (covers per-account clear, tenant-wide clear, empty→zeros, cross-tenant 404 with data intact, health/needs_reconnect/disabled untouched). 4) verify: gofmt, go vet, focused + full make test, git diff --check, CodeHealth safeguard. Response schema contract for 027.02: POSTs return {cleared_accounts: number, cleared_models: number}.

Executed with additions: shared clearCooldowns helper + mountAdmin route-group split (both driven by CodeHealth findings, no behavior change); test seed struct instead of 5-arg helper. Response schema contract for 027.02 unchanged: POSTs return {cleared_accounts: number, cleared_models: number}.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented: store ClearTenantCooldowns (accounts) + ClearTenantModelCooldowns (model_cooldowns via account subquery) + ClearAccountModelCooldowns now returns row count; gateway admin_cooldowns.go with adminResetAccountCooldown (404 on missing/foreign tenant, honest cleared_accounts from pre-state) + adminResetTenantCooldowns; routes POST /accounts/{id}/cooldown/reset and POST /health/cooldowns/reset registered in mountAdmin; response {cleared_accounts, cleared_models}.

Refactors (DoD#3 + CodeHealth gate): extracted shared AccountRepo.clearCooldowns behind ClearExpired/ClearProvider/ClearTenant cooldown paths; split mountAdmin into mountAccountRoutes + mountCatalogRoutes (Large Method fixed, module mean complexity 7.08->6.27); test seed helper takes cooldownSeed struct (arg-count finding fixed).

Verification: 4 new tests pass (per-account clear with untouched guarantees, empty zeros, unknown+foreign 404 with data intact, tenant-wide with counts + idempotent second run); gofmt/vet clean; full make test 46 ok exit 0; git diff --check clean; CodeHealth safeguard quality_gates passed.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Cooldown-reset API endpoints. New POST /api/accounts/{id}/cooldown/reset clears one account's dispatcher cooldown (cooldown_until, backoff, credits-exhausted) plus all its model cooldowns, 404ing on unknown or foreign-tenant ids; new POST /api/health/cooldowns/reset clears all parked accounts and model cooldowns tenant-wide. Both return {cleared_accounts, cleared_models}, are idempotent (empty reset yields zeros), and leave health history, probe results, needs_reconnect, and disabled state untouched. Store: ClearTenantCooldowns + ClearTenantModelCooldowns (account-subquery scoping) added, ClearAccountModelCooldowns now returns its row count. Tests: 4 new cases in admin_cooldowns_test.go through a chi router. Verified: focused tests pass, full make test 46 ok, gofmt/vet/diff-check clean, CodeHealth safeguard passed (also fixed its Large Method + duplication findings via shared clearCooldowns helper and mountAdmin route-group split).
<!-- SECTION:FINAL_SUMMARY:END -->
