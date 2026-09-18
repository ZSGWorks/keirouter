---
id: TASK-039
title: Fix stream-pump goroutine and connection leak on terminal sends
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
  - backend/internal/connectors/qoder.go
  - backend/internal/connectors/cloudcode.go
  - backend/internal/connectors/openai_compatible.go
modified_files:
  - backend/internal/connectors/httpclient.go
  - backend/internal/connectors/qoder.go
priority: high
type: bug
ordinal: 44000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Several streaming connectors send the terminal error chunk without selecting on context cancellation. When the consumer has stopped reading and the output channel is full, the send blocks forever, so the deferred response body close never runs and the goroutine, socket, and scanner buffer leak. Triggered whenever a client disconnects during an errored stream.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Every chunk send in the affected stream pumps is context-guarded
- [ ] #2 A test cancels the consumer mid-stream and asserts the response body closes and the goroutine exits
- [ ] #3 All terminal and secondary stream sends are audited
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
