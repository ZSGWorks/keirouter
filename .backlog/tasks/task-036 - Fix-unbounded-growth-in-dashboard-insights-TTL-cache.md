---
id: TASK-036
title: Fix unbounded growth in dashboard insights TTL cache
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
- [ ] #1 Expired cache entries are removed when encountered on read
- [ ] #2 The cache enforces a maximum entry count with oldest-expiry eviction
- [ ] #3 Invalid or unbounded period/tz values cannot increase retained entries indefinitely
- [ ] #4 Unit tests cover expiry deletion, cap eviction, and key-growth bound
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
