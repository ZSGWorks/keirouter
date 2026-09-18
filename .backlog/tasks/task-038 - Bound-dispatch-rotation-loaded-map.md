---
id: TASK-038
title: Bound dispatch rotation loaded map
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
- [ ] #1 The loaded map is bounded using the same cap strategy as the cursors map, or loaded state moves onto the cursor entry
- [ ] #2 A test inserts many distinct keys and asserts the loaded map stays bounded
- [ ] #3 Existing rotation and dispatch tests pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
