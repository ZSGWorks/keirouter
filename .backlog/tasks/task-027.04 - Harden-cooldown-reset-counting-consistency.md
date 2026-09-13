---
id: TASK-027.04
title: Harden cooldown-reset counting consistency
status: Done
assignee: []
created_date: '2026-09-13 11:29'
updated_date: '2026-09-13 11:35'
labels: []
milestone: Cooldown reset UX
dependencies: []
parent_task_id: TASK-027
priority: medium
type: bug
ordinal: 35000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Review findings on 026/027 commits: tenant predicate counts expired cooldowns (mismatch vs per-account cooldownParked), TOCTOU on parked pre-read, unconditional ResetBackoffLevelOnTx bumping updated_at, missing hint-precedence test for GLM 1310. Plan: repo_accounts parkedWhere + tenantCooldownClear helper + ClearAccountCooldownsOnTx; admin handler counts UPDATE rows (drop cooldownParked); classify429 hint-wins test; extend tenant reset test. Verify: gofmt/vet/focused/full make test/diff-check/safeguard.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Tenant-wide reset counts only accounts with active (future) cooldown_until, matching the per-account endpoint
- [x] #2 cleared_accounts on both endpoints derives from row-affected counts, not a pre-read state check
- [x] #3 ResetBackoffLevelOnTx call in admin reset path replaced by predicate-gated account clear; success-on-request path stays unconditional
- [x] #4 GLM hint-beats-reset precedence pinned by test
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Predicate semantics: parked now requires cooldown_until > now on both endpoints; stale expired rows left for ClearExpiredCooldowns (startup + periodic), matching cooldownParked behavior previously used by the per-account path.

Dropped: cooldownParked helper and resetAccountCooldown wrapper (both unused after row-count unification). ResetBackoffLevel/OnTx kept unconditional by design (success path).

Verification: TestAdminReset* 4/4 pass including new expired-tenant case; TestClassify429_GLM1310Quota_HintWinsOverReset passes; connectors suite 223 tests all pass; full make test 46 ok.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Unified parked-counting across both reset endpoints. repo_accounts.go: shared cooldownResetSet/cooldownParkedWhere constants (parked = future cooldown_until OR backoff OR credits flags; expired rows excluded, left for ClearExpiredCooldowns GC); tenantCooldownClear/accountCooldownClear builders dedupe the duplicated literal; new ClearAccountCooldownsOnTx. admin_cooldowns.go: per-account handler counts cleared_accounts from UPDATE rows affected, dropping the cooldownParked pre-read (kills TOCTOU count race and the unconditional updated_at bump on unparked accounts); ResetBackoffLevelOnTx unchanged for the success-on-request path. classify429_test.go: pinned hint-beats-reset precedence (retry-after 120s wins over 36h calendar stamp). Tenant test extended: expired-cooldown account not counted and its row survives reset. Verified: focused gateway/connectors/store tests pass, full make test 46 ok exit 0, gofmt/vet clean, diff-check clean, safeguard passed (Primitive Obsession ratio 40.91% vs 40% threshold, verdict stable, no degradation gate tripped). Response contract unchanged.
<!-- SECTION:FINAL_SUMMARY:END -->
