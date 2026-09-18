---
id: TASK-049
title: Pool Kiro eventstream read buffer and sweep Kiro caches
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 18:54'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/connectors/kiro.go
modified_files:
  - backend/internal/connectors/kiro.go
  - backend/internal/connectors/kiro_test.go
priority: medium
type: enhancement
ordinal: 54000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Kiro eventstream parser allocates a fresh 32KB read buffer on every fill, which is heavy churn for large frames, and its account-slot, model, and quota caches are never fully evicted when an account is removed or idle. Expired entries linger indefinitely.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The eventstream read buffer is pooled or reused
- [x] #2 Account-slot, model, and quota caches are swept for stale entries
- [x] #3 Tests cover the parser and cache sweeps
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Pool fill()'s 32KB eventstream scratch buffer in kiroFillBufPool; return it immediately after each Read.
2. Add kiroSweepExpiredCaches: Range/Delete expired model and quota entries; delete account slots idle for more than 10 minutes only when they have no in-flight request.
3. Replace account-slot channel values with kiroSlot values carrying lastUsed. Serialize acquire and the slot sweep under kiroAccountSlotsMu so deletion cannot race with an acquire.
4. Add cache and account-slot sweep tests; existing eventstream parser tests cover parser behavior.
5. Run gofmt, make vet, full Go test suite, connector race test, and CodeHealth pre-commit safeguard.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Slot sweep uses a mutex shared with acquireAccountSlot. This prevents deleting an idle slot between map lookup and its non-blocking channel acquire, which would otherwise let concurrent requests bypass the per-account single-flight limit.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Pooled Kiro eventstream fill()'s 32KB scratch buffer, eliminating its fresh allocation on each read. Added event-driven sweeping of expired model/quota cache entries and account slots idle for over 10 minutes. Account-slot acquisition and sweeping share a mutex, preserving the one-in-flight-request-per-account guarantee during deletion. Added model/quota and idle/busy/fresh slot sweep tests; existing eventstream parser coverage remains in place. Validation: connector tests, connector race detector, `make vet`, full `go test ./...`, and CodeHealth pre-commit safeguard all passed. CodeHealth gate passed with both changed Go files stable. IDE build endpoint was unavailable.
<!-- SECTION:FINAL_SUMMARY:END -->
