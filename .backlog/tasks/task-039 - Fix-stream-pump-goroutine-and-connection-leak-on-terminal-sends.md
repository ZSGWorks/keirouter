---
id: TASK-039
title: Fix stream-pump goroutine and connection leak on terminal sends
status: Done
assignee: []
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 16:43'
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
- [x] #1 Every chunk send in the affected stream pumps is context-guarded
- [x] #2 A test cancels the consumer mid-stream and asserts the response body closes and the goroutine exits
- [x] #3 All terminal and secondary stream sends are audited
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed stream-pump goroutine/connection leak: every terminal ChunkError send from streaming pump goroutines now selects on ctx.Done. 16 sites wrapped across httpclient.go (scanOpenAISSE scanner.Err path) plus github_copilot x2, cloudcode, openai_responses x2, commandcode, anthropic, openai_compatible, vertex, gemini, ollama, qoder x2 (incl. the one-line send+return), mimo_free. Previously, a consumer that stopped reading left the pump blocked on a full 16-slot channel forever, so the deferred resp.Body.Close never ran — leaking the goroutine, socket, and scanner buffer. Added regression test TestScanOpenAISSEPumpExitsWhenConsumerStopsAndContextCancels (full buffer + cancel → pump exits, channel closes). All connectors tests + full go test ./... pass; gofmt/make vet clean. CodeHealth: httpclient 5.81 (base 5.83), qoder 6.39 (base 6.44) — negligible delta from added selects; rest untouched.
<!-- SECTION:FINAL_SUMMARY:END -->
