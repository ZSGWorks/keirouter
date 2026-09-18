---
id: TASK-050
title: Evict proxy transport cache and close idle connections
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
  - backend/internal/connectors/httpclient.go
modified_files:
  - backend/internal/connectors/httpclient.go
priority: medium
type: enhancement
ordinal: 55000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The proxy transport cache keeps one transport per distinct proxy, relay, and no-proxy configuration and never evicts. Each transport retains an idle connection pool, so proxy configuration churn or account deletion permanently leaks transports and idle connections.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Transports for stale proxy configurations are evicted
- [ ] #2 Eviction calls CloseIdleConnections on the removed transport
- [ ] #3 A test asserts eviction and idle-connection cleanup
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
