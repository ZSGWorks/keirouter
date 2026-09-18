---
id: TASK-037
title: Prune health telemetry states and chains maps
status: To Do
assignee: []
created_date: '2026-09-18 15:35'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/health/telemetry.go
modified_files:
  - backend/internal/health/telemetry.go
priority: high
type: enhancement
ordinal: 42000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Health telemetry creates a permanent keyState per (provider, account, model, capability) and a chainState per chain in maps that are never pruned. The snapshot/prune passes delete expired buckets but never the map entry itself, so every key ever seen retains history up to the 24h window. On a busy gateway with account/model churn this grows without bound.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Key states with no buckets inside the history window are deleted during the prune pass
- [ ] #2 Chain states with no buckets inside the history window are deleted during the prune pass
- [ ] #3 flushCurrent no longer iterates dead keys indefinitely
- [ ] #4 A test grows distinct keys over time and asserts the maps stay bounded
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
