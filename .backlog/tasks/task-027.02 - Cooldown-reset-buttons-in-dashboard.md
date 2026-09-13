---
id: TASK-027.02
title: Cooldown-reset buttons in dashboard
status: To Do
assignee: []
created_date: '2026-09-13 10:49'
updated_date: '2026-09-13 10:49'
labels: []
dependencies:
  - TASK-027.01
parent_task_id: TASK-027
priority: medium
type: feature
ordinal: 33000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Operators manage accounts in one place and diagnose cooldown-affected routing in another. A reset action must be reachable from both so nobody has to hunt for it during an incident.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Each account row offers a reset action next to the existing manual actions
- [ ] #2 The health dashboard offers a tenant-wide reset action
- [ ] #3 Both actions confirm before clearing and report what was cleared afterwards
- [ ] #4 Frontend typecheck passes
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
