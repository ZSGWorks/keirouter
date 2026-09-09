---
id: TASK-023
title: 'Perf: frontend SSE + poll re-render fixes (Quota, ConsoleLog, Guardrails)'
status: Done
assignee: []
created_date: '2026-09-08 03:05'
updated_date: '2026-09-08 04:47'
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

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
## Plan: frontend SSE + poll re-render fixes

1. Quota.tsx:81-86 countdown: extract into leaf component with own state so 1s tick re-renders only that leaf.
2. Quota.tsx:73-75 SSE invalidation: debounce quota invalidation using USAGE_REFRESH_DEBOUNCE_MS pattern from Usage.tsx:112-122.
3. ConsoleLog.tsx:195-208: buffer incoming SSE lines ~250ms (accumulate in ref, flush via interval/rAF) before setEntries.
4. Guardrails.tsx:680 SSE: throttle/batch table updates similarly.
5. Verify: cd frontend && npm run typecheck && npm run build.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Quota.tsx: countdown moved to AutoRefreshCountdown leaf component with own 1s state; page keeps countdownReset signal only. SSE invalidation debounced via QUOTA_REFRESH_DEBOUNCE_MS (8s) with timer cleanup.

ConsoleLog.tsx: SSE lines accumulate in pendingRef, flushed to setEntries once per 250ms window; clear/error paths flush immediately; timer cleaned up on unmount.

Guardrails.tsx LogsTab: live SSE rows batched in pending array, flushed per 250ms with batch-aware dedupe (id set), sliced to 200.

npm run typecheck + npm run build green.
<!-- SECTION:NOTES:END -->
