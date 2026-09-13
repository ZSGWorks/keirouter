---
id: TASK-027.03
title: Harden cooldown-reset review findings
status: Done
assignee: []
created_date: '2026-09-13 10:58'
updated_date: '2026-09-13 11:00'
labels: []
dependencies: []
parent_task_id: TASK-027
priority: medium
type: feature
ordinal: 34000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Code review of the cooldown-reset endpoints found correctness gaps worth closing before the dashboard buttons land: expired cooldowns inflate cleared counts, the two-step reset can half-apply on mid-handler failure, and the tests bypass the real route registration while leaving the probe-history guarantee unverified.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Expired cooldowns no longer count as cleared in reset responses
- [x] #2 A failed reset leaves no half-applied state behind
- [x] #3 Tests exercise the real route registration and verify probe history survives
- [x] #4 Full backend suite, vet, gofmt, and CodeHealth safeguard pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Harden cooldown-reset per review. 1) admin_cooldowns.go: time-aware parked calc (BackoffLevel/CreditsExhausted/active CooldownUntil) + time import. 2) Atomicity via Tx: add OnTx variants (ResetBackoffLevelOnTx, ClearAccountModelCooldownsOnTx, ClearTenantCooldownsOnTx, ClearTenantModelCooldownsOnTx) sharing sqlExec internals per repo_apikeys.go precedent; handlers BeginTx/commit, existing funcs become wrappers. 3) Tests: serve via s.mountAdmin (fallback: two routes + registration assert if nil-dep panic); seed provider_probe_results row via InsertProbeResult and assert unchanged; Empty test gains state asserts + expired-cooldown case. 4) Nits: op param on clearCooldowns error wrap; active-only deletes (cooldown_until > now) in both model-clear paths, expired rows left for ExpireModelCooldowns GC. 5) Verify: gofmt/vet/focused/full make test/diff-check/safeguard. Public API + response schema unchanged.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Fixes applied: cooldownParked time-aware helper; withTx transaction in both handlers with 4 new OnTx store variants (existing funcs are thin wrappers); tests via real mountAdmin + probe-history assert + empty/expired asserts; cooldownClear struct; op-labeled error wraps; active-only model deletes.

Verification: focused gateway+store tests pass; full make test 46 ok exit 0; gofmt/vet/diff-check clean; safeguard quality_gates passed (arg-count + complexity findings resolved, mountAdmin Large Method stays fixed).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Hardened cooldown-reset per review. Time-aware parked counting (cooldownParked helper: expired cooldown_until no longer inflates cleared_accounts; expired model rows excluded from deletes and counts, left for ExpireModelCooldowns GC). Atomic resets: OnTx store variants (ResetBackoffLevelOnTx, ClearAccount/AccountModel/Tenant(Model)CooldownsOnTx) sharing sqlExec internals, handlers run both writes in one withTx transaction. Tests now serve through real mountAdmin under /api, seed and verify a provider_probe_results row survives, assert empty-reset state, and cover the expired-cooldown zero-count case. Gate-driven refactors: cooldownClear struct (arg-count findings fixed), split resetAccountCooldown helper (complexity finding fixed). Verified: 4 focused tests pass, full make test 46 ok, gofmt/vet/diff-check clean, CodeHealth safeguard passed. Public API and response schema unchanged.
<!-- SECTION:FINAL_SUMMARY:END -->
