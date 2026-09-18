---
id: TASK-040
title: Add HTTP server connection timeouts and header cap
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
  - backend/internal/app/app.go
  - backend/internal/config/config.go
modified_files:
  - backend/internal/app/app.go
  - backend/internal/config/config.go
priority: high
type: enhancement
ordinal: 45000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The HTTP server sets only ReadHeaderTimeout. Without IdleTimeout, idle keep-alive connections are never reaped by the server, and without MaxHeaderBytes a client can make oversized header reads. Once a handler calls io.ReadAll on the body there is no read deadline, so a slow client can hold a growing buffer and goroutine.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 IdleTimeout and MaxHeaderBytes are configured on the HTTP server
- [ ] #2 Streaming WriteTimeout remains unset and per-stream deadlines are preserved
- [ ] #3 New values are configurable through existing server config
- [ ] #4 A test asserts the server is constructed with the configured values
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
