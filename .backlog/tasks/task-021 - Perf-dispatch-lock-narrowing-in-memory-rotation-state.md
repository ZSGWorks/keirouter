---
id: TASK-021
title: 'Perf: dispatch lock narrowing + in-memory rotation state'
status: To Do
assignee: []
created_date: '2026-09-08 03:05'
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
