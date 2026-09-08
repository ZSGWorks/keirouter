---
id: TASK-022
title: 'Perf: frontend memoization pass (Usage cards, System chart, Toast)'
status: To Do
assignee: []
created_date: '2026-09-08 03:05'
labels:
  - performance
  - frontend
milestone: Performance Scan 2026-09
dependencies: []
priority: medium
type: enhancement
ordinal: 25000
---

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Usage.tsx SummaryCard items memoized, card components memoized
- [ ] #2 System.tsx chartData via useMemo
- [ ] #3 Toast host memoized with stable API identity
- [ ] #4 cd frontend && npm run typecheck && npm run build pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
