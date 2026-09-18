---
id: TASK-040
title: Add HTTP server connection timeouts and header cap
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 16:53'
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
- [x] #1 IdleTimeout and MaxHeaderBytes are configured on the HTTP server
- [x] #2 Streaming WriteTimeout remains unset and per-stream deadlines are preserved
- [x] #3 New values are configurable through existing server config
- [x] #4 A test asserts the server is constructed with the configured values
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. config.go ServerConfig: add IdleTimeout (koanf idle_timeout, default 120s) + MaxHeaderBytes (koanf max_header_bytes, default 1MiB) fields; normalize in Defaults like MaxRequestBodyBytes (zero/negative → default)\n2. app.go http.Server: set IdleTimeout + MaxHeaderBytes from cfg; keep WriteTimeout unset with existing comment (AC#2)\n3. Test in app package: build config → assert server fields carry configured + normalized values\n4. gofmt, make vet, go test ./..., CodeHealth app.go/config.go
<!-- SECTION:PLAN:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
HTTP server now sets IdleTimeout (default 120s, reaps dead keep-alive connections) and MaxHeaderBytes (default 1 MiB, net/http's own cap made explicit + tunable). Both flow through ServerConfig (`idle_timeout`, `max_header_bytes` koanf keys) with zero/negative normalization to defaults in validate(), mirroring MaxRequestBodyBytes. Server construction extracted into newHTTPServer(cfg, handler) so AC#4 is testable directly; WriteTimeout remains intentionally unset per the streaming design. Tests: TestNewHTTPServerCarriesTimeoutAndHeaderConfig (values + WriteTimeout unset) and TestServerConfigNormalizesIdleAndHeaderDefaults (in config package, uses cfg.validate). gofmt/make vet/full go test clean; CodeHealth app.go 8.33 (+0.02 vs base), config.go 9.44 (-0.04, added fields/consts).
<!-- SECTION:FINAL_SUMMARY:END -->
