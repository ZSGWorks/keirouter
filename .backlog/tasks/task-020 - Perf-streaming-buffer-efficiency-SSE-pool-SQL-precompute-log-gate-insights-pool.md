---
id: TASK-020
title: >-
  Perf: streaming + buffer efficiency (SSE pool, SQL precompute, log gate,
  insights pool)
status: To Do
assignee: []
created_date: '2026-09-08 03:05'
labels:
  - performance
  - backend
milestone: Performance Scan 2026-09
dependencies: []
priority: medium
type: enhancement
ordinal: 23000
---

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 SSE scan buffers use sync.Pool; unused duplicate scanner variant removed
- [ ] #2 Usage insert statement strings precomputed package-level (or prepared)
- [ ] #3 consoleLog level gate applied before formatting/fan-out work
- [ ] #4 Insights refresh goroutines bounded via worker pool/errgroup
- [ ] #5 gofmt clean, make vet, make test pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
