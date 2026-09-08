---
id: DRAFT-001
title: Budget Confirm() wipes concurrent reservations (releaseAll race)
status: Draft
assignee: []
created_date: '2026-09-08 03:05'
labels:
  - bug
  - budget
dependencies: []
priority: high
type: bug
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
In backend/internal/budget/budget.go:312-325, Confirm() calls releaseAll() which releases ALL active reservations for the budget/scope, including reservations created concurrently by other in-flight requests. Effect: under concurrent load, a completed request's Confirm can wipe other live reservations → budget spend under-counted → limits not enforced correctly.

Suspected fix direction: Confirm should release only its own reservation (keyed by reservation ID/request ID), not sweep all. Needs repro test: two goroutines Reserve, one Confirm, assert other reservation still active.

Found during perf scan 2026-09 (dispatch hot-path review). Not part of perf series.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Draft task documents root cause, affected code paths, repro sketch
- [ ] #2 Repro sketch demonstrates concurrent Confirm wiping foreign reservation
- [ ] #3 No fix required in this task; fix tracked separately
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
