---
id: TASK-027
title: Manual dispatcher-cooldown reset
status: To Do
assignee: []
created_date: '2026-09-13 10:49'
labels: []
dependencies: []
priority: medium
type: feature
ordinal: 31000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
There is currently no manual way to clear dispatcher cooldowns: model cooldowns clear only on successful traffic or time expiry, and reconnect flows clear account-level fields only. After incidents like the GLM 1310 quota exhaustion, operators cannot unpark targets deliberately and must wait or hand-edit the database.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Operators can manually clear dispatcher cooldowns without waiting for expiry or running traffic
- [ ] #2 Health history and probe results are left untouched by the reset
- [ ] #3 Reset actions are available both where accounts are managed and where cooldown-affected state is visible
- [ ] #4 Destructive clears require an explicit confirmation step
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
