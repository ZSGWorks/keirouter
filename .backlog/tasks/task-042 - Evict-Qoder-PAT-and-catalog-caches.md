---
id: TASK-042
title: Evict Qoder PAT and catalog caches
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
  - backend/internal/connectors/qoder.go
modified_files:
  - backend/internal/connectors/qoder.go
priority: high
type: bug
ordinal: 47000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Qoder connector stores PAT sessions keyed by raw PAT and a per-user model catalog, and never deletes entries. Expiry is checked only on read, so deleted or re-authenticated accounts retain token, identity, and raw model-config data indefinitely. This is both a memory and a secret-retention issue.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Expired PAT sessions are evicted on write and lookup
- [ ] #2 Catalog entries past TTL are pruned
- [ ] #3 A test asserts stale entries are removed
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
