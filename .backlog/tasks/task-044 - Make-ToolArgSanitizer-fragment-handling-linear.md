---
id: TASK-044
title: Make ToolArgSanitizer fragment handling linear
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 17:28'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/transform/toolsanitize.go
modified_files:
  - backend/internal/transform/toolsanitize.go
priority: high
type: enhancement
ordinal: 49000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The tool-argument sanitizer copies the entire accumulated arguments buffer into a string on every streamed fragment and re-parses complete objects. For fragmented tool calls with large arguments this is O(N times k) allocation and JSON parsing, causing heavy GC churn during code-writing streams.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Fragment append and comparison avoid materializing the full accumulated string per chunk
- [x] #2 Snapshot versus incremental detection behavior is preserved
- [x] #3 Existing sanitizer tests pass and a benchmark shows linear scaling
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. toolBuffer gains completeValid/complete cache of accumulated-buffer completeness\n2. appendToolArgs: HasPrefix path unchanged (no existing parse); two-complete-objects path uses cached completeness — set after replace (fragment completeness already computed), invalidated after append, lazily recomputed once only when actually needed\n3. Preserves exact snapshot-vs-delta semantics (AC#2): same predicates, just memoized\n4. Benchmark BenchmarkAppendToolArgsSnapshot (growing snapshot stream) + delta-stream variant; scaling check\n5. gofmt, make vet, go test ./..., CodeHealth toolsanitize.go
<!-- SECTION:PLAN:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
ToolArgSanitizer no longer re-parses the accumulated arguments buffer per fragment. toolBuffer memoizes buffer completeness (completeValid/complete): set after a replace (buffer == fragment, whose completeness was just computed), invalidated on append, lazily recomputed via bufferComplete() only when a fragment is itself a complete object and the buffer grew by appends. strings.Builder.String() is already O(1), so no full-buffer materialization remains on the hot path. Semantics preserved exactly (same HasPrefix / both-complete predicates, memoized only) — pinned by TestAppendToolArgs_SnapshotVsDeltaEquivalence covering deltas, prefix-extend snapshots, correcting snapshots, {} skips, and repeated-prefix fragments. Benchmarks: BenchmarkAppendToolArgsDelta (linear common case), BenchmarkAppendToolArgsSnapshot (prefix-extending), BenchmarkAppendToolArgsReorderedSnapshot (non-prefix shape that previously re-parsed per chunk). ponytail note: residual cost is the inherent O(input) prefix/replace scan — memcmp-bound; rolling-hash detection only if that ever shows up in profiles. ponytail comment added in toolsanitize.go. gofmt/make vet/full go test clean; CodeHealth 8.63 (== baseline).
<!-- SECTION:FINAL_SUMMARY:END -->
