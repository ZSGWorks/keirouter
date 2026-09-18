---
id: TASK-041
title: Bound Cursor connector body read and gunzip inflation
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 17:01'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/connectors/cursor.go
  - backend/internal/connectors/cursor_protobuf.go
modified_files:
  - backend/internal/connectors/cursor.go
  - backend/internal/connectors/cursor_protobuf.go
priority: high
type: bug
ordinal: 46000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Cursor connector reads the entire upstream response with io.ReadAll and no limit, then gunzips decompressed frames with an unbounded io.ReadAll. A large or crafted compressed frame can inflate to an arbitrary size, creating an out-of-memory spike per request.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The upstream response read is bounded by a maximum size limit
- [x] #2 Decompression output is bounded and oversized input is rejected without unbounded allocation
- [x] #3 A test covers the oversized/inflated case
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. cursor.go: replace io.ReadAll(resp.Body) with LimitReader-capped read at maxCursorBodyBytes+1; over-limit → ErrUpstream provider error (AC#1)\n2. cursor_protobuf.go gunzip: wrap gzip reader with LimitReader at maxCursorInflatedBytes+1, reject over-limit with clear error (AC#2) — callers already treat gunzip error as skip-frame\n3. Test: oversized body rejected without allocating beyond cap; gzip bomb (small input, huge output) rejected\n4. gofmt, make vet, go test ./..., CodeHealth cursor*.go
<!-- SECTION:PLAN:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Bounded Cursor memory exposure. Upstream response body is read through io.LimitReader at maxCursorBodyBytes (128 MiB) with an explicit over-cap rejection (new core.ProviderError) instead of buffering an arbitrary stream. gunzip() in cursor_protobuf.go caps decompressed output at maxCursorInflatedBytes (64 MiB) via LimitReader and returns a size error when exceeded — existing callers already treat gunzip failure as skip-frame, so oversized frames are dropped without unbounded allocation. Tests: TestGunzipRejectsBombOutput (64MiB+ of 'A' compressed small → rejected, output bounded), TestGunzipAcceptsNormalFrame (no regression), TestCursorBodyReadIsBounded (oversize body → ProviderError). gofmt/make vet/full go test clean; CodeHealth cursor.go 7.74 and cursor_protobuf.go 7.62, both identical to baseline.
<!-- SECTION:FINAL_SUMMARY:END -->
