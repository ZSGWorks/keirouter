---
id: TASK-050
title: Evict proxy transport cache and close idle connections
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 18:57'
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
  - backend/internal/connectors/httpclient_test.go
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
- [x] #1 Transports for stale proxy configurations are evicted
- [x] #2 Eviction calls CloseIdleConnections on the removed transport
- [x] #3 A test asserts eviction and idle-connection cleanup
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Replace proxy transport cache values with entries containing *http.Transport and atomic last-used timestamps.
2. In clientFor, refresh the selected entry timestamp and sweep entries idle beyond a bounded TTL before serving new proxy configurations.
3. On eviction, delete the cache entry and call CloseIdleConnections so its connection pool cannot outlive the configuration.
4. Add an HTTP server-backed test that creates an idle connection, ages its cache entry, then asserts cache removal and connection closure.
5. Run gofmt, connector tests, race detector, make vet, full Go tests, and CodeHealth.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Eviction runs at most once per minute from clientFor to avoid an unbounded cache Range on every proxied request. `CloseIdleConnections` is safe for an entry that races with a request because it closes only idle connections.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Proxy transport cache entries now carry last-used timestamps and expire after 10 minutes of inactivity. `clientFor` refreshes usage and triggers a throttled (once-per-minute) stale-entry sweep; eviction atomically removes each stale entry and calls `CloseIdleConnections`. Added an HTTP-server-backed regression test that observes a kept-alive connection reach idle, then verifies eviction removes its cache entry and closes that connection. Validation: targeted connector test, connector race detector, `make vet`, full `go test ./...`, and CodeHealth pre-commit safeguard passed; CodeHealth improved `httpclient.go`. IDE build endpoint was unavailable.
<!-- SECTION:FINAL_SUMMARY:END -->
