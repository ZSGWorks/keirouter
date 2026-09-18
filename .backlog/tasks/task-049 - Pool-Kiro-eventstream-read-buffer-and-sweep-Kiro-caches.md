---
id: TASK-049
title: Pool Kiro eventstream read buffer and sweep Kiro caches
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
  - backend/internal/connectors/kiro.go
modified_files:
  - backend/internal/connectors/kiro.go
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
- [ ] #1 The eventstream read buffer is pooled or reused
- [ ] #2 Account-slot, model, and quota caches are swept for stale entries
- [ ] #3 Tests cover the parser and cache sweeps
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
