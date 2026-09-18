---
id: TASK-051
title: Replace quadratic string building and double work in pipeline
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 19:01'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/pipeline/pipeline.go
  - backend/internal/normalizer/normalizer.go
modified_files:
  - backend/internal/pipeline/pipeline.go
  - backend/internal/pipeline/token_saving_test.go
priority: medium
type: enhancement
ordinal: 56000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The stream drain path accumulates text by repeated string concatenation, which is quadratic in response size, and the normalizer is applied twice per request even when the first pass removed nothing. Both add avoidable allocation on hot paths.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The stream drain accumulates through a builder written once at the end
- [x] #2 The normalizer runs once unless a compression actually removed parts
- [x] #3 Existing pipeline tests pass and a test covers the second-pass condition
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Replace repeated text/thinking concatenation in drainStream with strings.Builder and materialize each value once after the stream closes.
2. Count request content parts before compression. Run final normalizer reconciliation only when Slimmer or Headroom leaves fewer parts, since only removal can orphan a tool call/result pair.
3. Add focused unit coverage for unchanged and reduced content-part conditions; existing pipeline tests cover stream draining.
4. Run gofmt, pipeline tests, full Go suite, vet, and CodeHealth.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Second normalizer pass is guarded by content-part count rather than byte savings: textual compression need not reconcile tool pairs, while a removed part can orphan one. This preserves repair behavior only when needed.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Replaced quadratic streamed text and thinking concatenation with `strings.Builder`, materializing each content value once after drain completion. The final normalizer pass now runs only if Slimmer or Headroom reduced the request content-part count; unchanged or text-only compressed histories avoid a redundant full normalization. Added regression tests for both second-pass outcomes and concatenated stream text/thinking behavior. Validation: targeted pipeline tests, pipeline race detector, `make vet`, full `go test ./...`, and CodeHealth pre-commit safeguard passed; CodeHealth is stable. IDE build endpoint was unavailable.
<!-- SECTION:FINAL_SUMMARY:END -->
