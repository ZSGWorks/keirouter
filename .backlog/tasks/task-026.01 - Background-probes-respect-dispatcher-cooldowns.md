---
id: TASK-026.01
title: Background probes respect dispatcher cooldowns
status: Done
assignee: []
created_date: '2026-09-13 10:14'
updated_date: '2026-09-13 10:35'
labels: []
dependencies: []
parent_task_id: TASK-026
priority: medium
type: enhancement
ordinal: 30000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The background health checker probes every account/model with real upstream Chat completions every 30s, including targets the dispatcher has parked on cooldown. That burns quota on exhausted accounts, inflates consecutive-failure counters, and paints targets green in the dashboard that routing will never select, which is exactly what hid the dead second chain target in the GLM 1310 incident.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Background probes do not send billable upstream requests to targets under an active dispatcher cooldown (account or model level)
- [x] #2 Skipped probes leave the existing health state untouched instead of flipping healthy/unhealthy
- [x] #3 Successful real traffic still recovers account health and clears cooldowns as it does today
- [x] #4 Unit tests cover the skip behavior; the backend test suite passes
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Cooldown-aware probes (approved plan implement-task-02601-cooldown--2026-09-13): 1) Add CooldownSource iface + Checker field/ctor arg, wire db.Routing() in app.go. 2) CheckOnce: skip billable model probes on account CooldownUntil or batched ActiveCooldowns; __all__ Validate keeps running; skips write nothing. 3) Force flag on CheckOnce + check-now force=true. 4) New healthcheck package tests with fakes. 5) Verify gofmt/vet/focused/full tests/safeguard.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented: CooldownSource iface + Checker.cools + SetCooldownSource; CheckOnce(ctx, tenant, force) skips billable model probes on account CooldownUntil or batched ActiveCooldowns (helpers groupProbeModels/snapshotCooldowns/skipProbe/probeCooling); __all__ Validate always runs; check-now ?force=true bypasses. Wired in startBackgroundWorkers (kept Build untouched). New healthcheck_test.go: 5 tests (table-driven skip, Validate-under-cooldown, force, no-source fail-open, healthy path).

Verification: gofmt clean (bootstrap.go pre-existing), go vet clean, package + full make test green, diff-check clean. CodeHealth safeguard: CheckOnce improved 16->15, no duplication, app.go clean; sole remaining finding is module-mean complexity 6.27 (baseline 8.83, threshold 5) labeled introduced — mean improved substantially, residual is pre-existing CheckOnce/probe/record bulk. Awaiting user call on gate (accept exception vs deeper simplification vs rules tune).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Cooldown-aware background probes. Checker gains a CooldownSource (wired to db.Routing() in startBackgroundWorkers); CheckOnce skips billable per-model Chat probes when the account or model sits on a dispatcher cooldown, writing nothing to account_health. Cheap __all__ credential validations keep running; manual POST /health/check-now accepts ?force=true to bypass skips. Real-traffic NoteSuccess recovery untouched. Tests: 5 new cases in healthcheck_test.go (table-driven account/model skip with untouched-state assert, Validate-under-cooldown, force bypass, no-source fail-open, healthy path). Verified: gofmt/vet/package + full make test/diff-check clean. CodeHealth DoD checked as accepted exception per user call: safeguard's sole finding is module-mean complexity 6.27 vs threshold 5, improved from baseline 8.83 with CheckOnce itself reduced 16->15; residual is pre-existing bulk.
<!-- SECTION:FINAL_SUMMARY:END -->
