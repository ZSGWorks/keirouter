---
id: TASK-048
title: Pool gateway direct-stream buffers and cap NDJSON buffering
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
  - backend/internal/gateway/handlers.go
modified_files:
  - backend/internal/gateway/handlers.go
priority: medium
type: enhancement
ordinal: 53000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The gateway direct-stream copier allocates a fresh 64KB bufio.Reader and event buffer per stream instead of using a pool, and its NDJSON oversized buffer can grow to hold an entire response line. Under concurrency this adds avoidable allocation and a large single-connection spike.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 The reader and event buffers are pooled
- [ ] #2 NDJSON oversized buffering is capped and the remainder is passed through
- [ ] #3 Tests cover the streaming and oversized paths
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
