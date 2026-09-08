---
id: TASK-021
title: 'Perf: dispatch lock narrowing + in-memory rotation state'
status: Done
assignee: []
created_date: '2026-09-08 03:05'
updated_date: '2026-09-08 04:00'
labels:
  - performance
  - backend
milestone: Performance Scan 2026-09
dependencies: []
priority: high
type: enhancement
ordinal: 24000
---

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 lockSelection no longer spans DB/vault/OAuth work under mutex; lock scope limited to rotation state read/advance
- [ ] #2 Chain/account rotation state kept in memory with persistence fallback (write-behind or on-change)
- [ ] #3 Cooldown/unhealthy account lookups deduped per provider per plan
- [ ] #4 Existing dispatch/pipeline tests pass; new test covers concurrent selection correctness
- [ ] #5 gofmt clean, make vet, make test pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Removed lockSelection from PlanWith: DB lookups, vault decrypt, OAuth refresh now run unlocked; correctness preserved because rotation state moved to mutex-protected in-memory rotationCache (dispatch/rotation.go).

rotationCache: seeded from store per key on first use; chain/target cursors unified into one cursors map; affinity pins in memory; persistence via bounded background goroutines (32 in-flight cap, 5s timeout). Eviction lazy at 4096 entries.

Affinity writes mark key loaded so fresh pin is never clobbered by a stale seed. evictAffinity is compare-and-clear, so a concurrent re-pin is not wiped.

PlanWith probe dedupe: probeCache dedupes ActiveCooldownExpirations + UnhealthyAccounts per provider+model pair per plan.

Tests: countingRouting proves 200 concurrent advances cover all 4 cursors exactly, 1 store read per key, evict guard; race clean. Full go test ./... green, vet green, gofmt clean.

CodeScene pre-commit: PlanWith complexity improved; remaining minor flags (string-heavy cache API args, 2-branch test conditional) accepted as inherent to keyed-cache design.
<!-- SECTION:NOTES:END -->
