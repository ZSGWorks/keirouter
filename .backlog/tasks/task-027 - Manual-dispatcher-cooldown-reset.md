---
id: TASK-027
title: Manual dispatcher-cooldown reset
status: Done
assignee: []
created_date: '2026-09-13 10:49'
updated_date: '2026-09-13 11:37'
labels: []
dependencies: []
priority: medium
type: feature
ordinal: 31000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
There is currently no manual way to clear dispatcher cooldowns: model cooldowns clear only on successful traffic or time expiry, and reconnect flows clear account-level fields only. After incidents like the GLM 1310 quota exhaustion, operators cannot unpark targets deliberately and must wait or hand-edit the database.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Operators can manually clear dispatcher cooldowns without waiting for expiry or running traffic
- [x] #2 Health history and probe results are left untouched by the reset
- [x] #3 Reset actions are available both where accounts are managed and where cooldown-affected state is visible
- [x] #4 Destructive clears require an explicit confirmation step
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Manual dispatcher-cooldown reset delivered via 4 subtasks. Endpoints: POST /api/accounts/{id}/cooldown/reset (per-account, 404 on unknown/foreign tenant) + POST /api/health/cooldowns/reset (tenant-wide), both returning {cleared_accounts, cleared_models}, idempotent zeros, health history/probe results/reconnect flags untouched (test-asserted). Consistent parked predicate (cooldown_until > now) across both endpoints with row-affected counting; expired rows left for ClearExpiredCooldowns GC. Dashboard: TimerOff reset buttons on ProviderDetail (per-account) + ProviderHealth (tenant-wide) via shared CooldownResetModal with explicit confirmation. Tests: 8 gateway reset tests + GLM hint-precedence pin; full make test 46 ok; vet/gofmt/diff-check clean; CodeHealth safeguard passed each pass.
<!-- SECTION:FINAL_SUMMARY:END -->
