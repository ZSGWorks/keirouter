---
id: TASK-020
title: >-
  Perf: streaming + buffer efficiency (SSE pool, SQL precompute, log gate,
  insights pool)
status: Done
assignee: []
created_date: '2026-09-08 03:05'
updated_date: '2026-09-08 04:41'
labels:
  - performance
  - backend
milestone: Performance Scan 2026-09
dependencies: []
priority: medium
type: enhancement
ordinal: 23000
---

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 SSE scan buffers use sync.Pool; unused duplicate scanner variant removed
- [ ] #2 Usage insert statement strings precomputed package-level (or prepared)
- [ ] #3 consoleLog level gate applied before formatting/fan-out work
- [ ] #4 Insights refresh goroutines bounded via worker pool/errgroup
- [ ] #5 gofmt clean, make vet, make test pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
## Plan: streaming + buffer efficiency

1. httpclient.go sseScanner: allocate 64KB per stream via make(); replace with sync.Pool of buffers; remove dead sseScannerPooled duplicate. Verify release path (deferred buffer return) on all exits.
2. repo_usage.go insertUsage/insertUsageBatch: precompute SQL placeholder strings package-level (sqlite + postgres dialect variants) instead of strings.Repeat per call.
3. consolelog: gate DEBUG formatting before Sprintf/fan-out — check how handlers pass pre-formatted strings; move min-level check into Buffer.Log entry and make handlers skip building multi-line detail when level filtered. Inspect consolelog.go API first.
4. insights.go:473 unbounded goroutine fan-out per dashboard refresh: bound via existing pattern model_cache.go:168-185 (semaphore/errgroup).
5. Tests for scanner pooling + SQL precompute; gofmt/vet/test full suite.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
SSE scanner: sseScanner now returns (*bufio.Scanner, func()) with 64KB buffer from sync.Pool (sseBufPool); grown buffers (>64KB) dropped on release; removed dead sseScannerPooled duplicate; all 15 connector call sites updated with defer sseRelease().

repo_usage.go: precomputed usageInsertSqlite + usageRowPlaceholdersSqlite/Postgres at package init via usageArgsPerRow; no more strings.Repeat per insert/batch.

consolelog.Buffer.SetMinLevel(config level): levelRank gate in add() before ring append/fan-out; gateway.Server wires d.Config.Log.Level (debug keeps all; info drops DEBUG; warn drops LOG/INFO; error ERROR-only). New consolelog_test.go.

insights.go: bounded quota probe fan-out via quotaProbes chan struct{} (4) mirroring model_cache pattern; extracted probeAccountQuota — adminQuotaUsage complexity 28→18 (CodeScene improved).

Full backend suite green, vet green, gofmt clean on touched pkgs (repo-wide gofmt -l shows pre-existing unrelated files).
<!-- SECTION:NOTES:END -->
