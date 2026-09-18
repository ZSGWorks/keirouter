---
id: TASK-036
title: Fix unbounded growth in dashboard insights TTL cache
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 16:16'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/gateway/insights_cache.go
  - backend/internal/gateway/insights.go
  - backend/internal/gateway/usage_v2.go
modified_files:
  - backend/internal/gateway/insights_cache.go
  - backend/internal/gateway/insights.go
  - backend/internal/gateway/usage_v2.go
priority: high
type: enhancement
ordinal: 41000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The dashboard insights TTL cache keeps pre-marshaled JSON bodies keyed by caller-supplied strings but never deletes expired entries and has no size cap. Keys are built from raw `period` and `tz` query parameters, so each distinct combination creates a permanent entry holding a full dashboard payload. A client can grow process memory without bound.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Expired cache entries are removed when encountered on read
- [x] #2 The cache enforces a maximum entry count with oldest-expiry eviction
- [x] #3 Invalid or unbounded period/tz values cannot increase retained entries indefinitely
- [x] #4 Unit tests cover expiry deletion, cap eviction, and key-growth bound
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. insights_cache.go: get() deletes expired entry on read; set() enforces insightsCacheMaxEntries=64 with oldest-expiry eviction\n2. Add canonicalInsightsKey(prefix, period, tz): period canonicalized via same switch semantics as sinceForPeriod (bounded set), tz validated with time.LoadLocation → IANA name, fallback "local" → key space bounded\n3. Update 4 key sites (insights.go x2, usage_v2.go x2) to use helper\n4. Tests beside code: expiry deletion, cap eviction, key-growth bound (many distinct invalid tz/period inputs → entry count stays bounded)\n5. gofmt, make vet, go test ./..., CodeHealth on touched files
<!-- SECTION:PLAN:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Bounded the dashboard insights TTL cache. `get` now deletes expired entries on read; `set` enforces `insightsCacheMaxEntries=64` with soonest-expiry eviction. Added `canonicalInsightsKey` which folds `period` to its canonical value (matches `sinceForPeriod` semantics) and keeps `tz` only when it resolves via `time.LoadLocation`, else collapses to "local" — so arbitrary/invalid query strings can no longer mint permanent entries. All four cache sites (insights.go x2, usage_v2.go x2) use the helper. Tests: expiry deletion on read, cap eviction of soonest-expiring entry, key-growth bound (7 periods x 500 invalid tz values → bounded distinct keys). gofmt clean, make vet clean, full `go test ./...` pass. CodeHealth 10/10 on both touched files.
<!-- SECTION:FINAL_SUMMARY:END -->
