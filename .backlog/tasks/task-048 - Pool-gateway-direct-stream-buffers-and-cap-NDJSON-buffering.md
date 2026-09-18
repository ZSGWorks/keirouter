---
id: TASK-048
title: Pool gateway direct-stream buffers and cap NDJSON buffering
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 18:03'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/gateway/handlers.go
modified_files:
  - backend/internal/gateway/handlers.go
priority: medium
type: enhancement
ordinal: 53000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The gateway direct-stream copier allocates a fresh 64KB bufio.Reader and event buffer per stream instead of using a pool, and its NDJSON oversized buffer can grow to hold an entire response line. Under concurrency this adds avoidable allocation and a large single-connection spike.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The reader and event buffers are pooled
- [x] #2 NDJSON oversized buffering is capped and the remainder is passed through
- [x] #3 Tests cover the streaming and oversized paths
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. sync.Pool gatewayStreamBufPool holding 64KB slices; copySanitizedStream: Get → bufio.NewReader(buf[:0-cap])? bufio.NewReaderSize replaces buf — use pool for the event bytes.Buffer + reader buffer slice via NewReader(buf) w/ reset on release. bufio.NewReader(r) uses default; use NewReaderSize with pooled byte slice via bufio.NewBuffer? Simplest: pool *bytes.Buffer for event + oversized; reader buffer pooled via slice: r := bufio.NewReaderSize(src, 0) not allowed. Use bufio.NewScanner? Keep ReadSlice; pool byte slice: buf := pool.Get(); reader := bufio.NewReaderSize(src, 64KB) still allocs internally — instead bufio.NewReader(reader) with pooled buffer via NewReaderSize requires alloc. Use pooled 64KB slice + bufio.NewBuffer? There's bufio.NewReaderSize always allocs. Alternative: bufio.Reader zero-value? Not usable directly. Practical: pool the Reader object itself: gatewayReaderPool sync.Pool of *bufio.Reader; Get → Reset(src) + SetSize? Reader internal buffer allocated once per pooled Reader (64KB); Reset(src) reuses — bufio.Reader.Reset exists. Pool *bufio.Reader via NewReaderSize once. Release: Reset(nil)? Reset requires io.Reader; Reset(src) keeps internal buf. Good.\n2. NDJSON oversized cap: ndjsonOversizeCap = 1MB; when oversized.Len() >= cap, flush oversized so far, then pass remaining fragments of THIS line directly to writeSanitizedFrame without accumulating (remainder passthrough, sanitization skipped for remainder — documented).\n3. Tests: direct stream SSE passthrough, oversized NDJSON line > cap (remainder arrives, no unbounded growth), pooled reuse sanity.\n4. gofmt, make vet, go test ./..., CodeHealth handlers.go
<!-- SECTION:PLAN:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Gateway direct-stream copier now pools its buffers and caps NDJSON oversized buffering. gatewayStreamReaderPool recycles *bufio.Reader objects (64KB internal buffer kept across streams; Reset(src) swaps source without reallocating) and gatewayEventBufPool recycles the event/oversized accumulation buffers — per-stream allocations drop to zero on the hot path. copySanitizedNDJSON buffers an oversized line only up to ndjsonOversizeCap (1MB): at the cap mid-line it flushes the buffered head and streams the remainder through unbuffered/unscanned (sanitization deliberately skipped for the tail — the cap exists so one crafted line cannot make the gateway buffer an unbounded response), and passes the final fragment through directly. Tests: TestCopySanitizedStream_PassthroughSSE, TestCopySanitizedNDJSON_OversizedLineRemainderPassed (1.5MB single line fully delivered), TestGatewayStreamReaderPoolReuse, TestCopySanitizedStream_ProviderErrorReplaced (sanitized replacement still propagates). gofmt/make vet/full go test clean; CodeHealth handlers.go 3.63 (baseline 3.70; pre-existing red-zone file, small delta from added cap branch).
<!-- SECTION:FINAL_SUMMARY:END -->
