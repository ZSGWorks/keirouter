---
id: TASK-023
title: 'Perf: frontend SSE + poll re-render fixes (Quota, ConsoleLog, Guardrails)'
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
ordinal: 26000
---

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 QuotaPage countdown isolated so 1s tick no longer re-renders whole page
- [ ] #2 Quota SSE invalidation debounced (existing USAGE_REFRESH_DEBOUNCE_MS pattern)
- [ ] #3 ConsoleLog buffers incoming SSE lines (~250ms) before state flush
- [ ] #4 Guardrails SSE table updates throttled/batched
- [ ] #5 cd frontend && npm run typecheck && npm run build pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
