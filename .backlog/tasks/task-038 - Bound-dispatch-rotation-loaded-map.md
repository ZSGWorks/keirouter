---
id: TASK-038
title: Bound dispatch rotation loaded map
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 16:32'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/dispatch/rotation.go
  - backend/internal/dispatch/dispatch.go
modified_files:
  - backend/internal/dispatch/rotation.go
priority: high
type: enhancement
ordinal: 43000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The rotation cache trims its cursors and affinity maps but its loaded map has no cap or prune. Affinity scope keys embed a request-content fingerprint, so with smart round-robin enabled a new loaded entry is added per distinct request fingerprint and retained for the process lifetime.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The loaded map is bounded using the same cap strategy as the cursors map, or loaded state moves onto the cursor entry
- [x] #2 A test inserts many distinct keys and asserts the loaded map stays bounded
- [x] #3 Existing rotation and dispatch tests pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Drop the separate loaded map entirely: "loaded" becomes map presence in r.cursors / r.affinity (AC#1 moves loaded state onto cursor entry)\n2. Add isLoaded(key) helper (kind-aware: affinity key vs cursor id); trySeed + commitSeed use it; seeding dedup map unchanged\n3. advance/setAffinity/re-pin/unpin drop loaded writes — affinity writes already set r.affinity so presence follows; advance trim already replaces cursors+sizes maps which now also bounds loaded-ness\n4. Test: advance N > rotationEntryMax distinct targets, assert len(r.cursors) <= rotationEntryMax and subsequent advance still works (re-seed path); countingRouting stub reused\n5. gofmt, make vet, go test ./..., CodeHealth rotation.go
<!-- SECTION:PLAN:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Removed the unbounded `loaded` map from rotationCache. Loaded-ness now lives in the state maps themselves: isLoaded(kind-aware) checks presence in r.cursors or r.affinity, so the existing lazy trims (cursor/sizes replacement in advance, expired-eviction + halving in setAffinity) bound seeded state too. An evicted key simply re-seeds from the store on its next use — same zero-cursor behavior as before, with at most an extra store read after a trim. seeding dedup map unchanged. New TestRotationCacheLoadedStateStaysBounded advances rotationEntryMax+512 distinct targets and asserts cursors stay at cap while an evicted key still advances via the re-seed path. All existing rotation/dispatch tests pass; gofmt/make vet/full go test clean; CodeHealth 10/10.
<!-- SECTION:FINAL_SUMMARY:END -->
