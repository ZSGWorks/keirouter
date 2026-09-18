---
id: TASK-043
title: Remove redundant Headroom full-payload serialization
status: Done
assignee: []
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 17:17'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/headroom/compressor.go
  - backend/internal/headroom/mapping.go
modified_files:
  - backend/internal/headroom/compressor.go
  - backend/internal/headroom/mapping.go
priority: high
type: enhancement
ordinal: 48000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Headroom compressor serializes the full request JSON three times per request, discarding two of the results, and materializes the full message set four times. Peak transient heap is several multiples of the serialized body, which is significant for large coding-agent turns before any upstream call.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The OpenAI message mapping is built once and reused for the POST body and both size measurements
- [x] #2 Bytes-before and bytes-after values are unchanged from current behavior
- [x] #3 An allocation or benchmark test demonstrates the reduction
- [x] #4 Existing Headroom tests pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Removed redundant Headroom serialization. Compress now maps the request to the OpenAI shape exactly once (mapped := toOpenAIMessages(req)); the same slice feeds both the bytesBefore measurement and the POST body via callCompress(ctx, req, mapped, cfg). bytesAfter is measured directly from resp.Messages instead of re-mapping the mutated request. Per request this drops from 3 full JSON serializations + 4 message-set materializations to 2 serializations (before/after measurement share marshal output with the POST body via reuse) + 2 materializations (mapped once, response once). AC#2 equivalence covered by TestCompress_BytesMeasurementsUnchangedBySinglePassMapping (jsonBytes(toOpenAIMessages(req-after)) == recorded BytesAfter). Allocation benchmarks added: BenchmarkCompressAllocation (end-to-end) and BenchmarkToOpenAIMessagesMapping (isolation; 13.2µs, 153KB, 65 allocs per mapping — previously run 3x per request). All 16 existing Headroom tests pass; gofmt/make vet/full go test clean; CodeHealth compressor.go 10/10.
<!-- SECTION:FINAL_SUMMARY:END -->
