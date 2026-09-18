---
id: TASK-047
title: Retain grown SSE scanner buffers and avoid double line copy
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
ordinal: 52000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The SSE scanner pool drops any buffer larger than 64KB and each scanned line is copied twice, once via Scanner.Text and once via a byte conversion. Streams with large frames repeatedly allocate fresh multi-megabyte buffers, defeating the pool.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Grown scanner buffers are returned to the pool up to the maximum cap
- [ ] #2 The scanned line reaches the codec without an extra copy where safe
- [ ] #3 A benchmark shows reduced allocations for large-frame streams
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
