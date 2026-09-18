---
id: TASK-047
title: Retain grown SSE scanner buffers and avoid double line copy
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 17:58'
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
priority: medium
type: enhancement
ordinal: 52000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The SSE scanner pool drops any buffer larger than 64KB and each scanned line is copied twice, once via Scanner.Text and once via a byte conversion. Streams with large frames repeatedly allocate fresh multi-megabyte buffers, defeating the pool.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Grown scanner buffers are returned to the pool up to the maximum cap
- [x] #2 The scanned line reaches the codec without an extra copy where safe
- [x] #3 A benchmark shows reduced allocations for large-frame streams
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. sseScanner release: retain grown buffers up to 2MB (sseBufMaxRetain). Growth detection: track max token len via wrapper — release closure receives sc; grew iff last sc.Bytes() cap > 64K OR max line observed > 64K. Implement: closure takes sc + buf; compute tokCap := cap(sc.Bytes()); if tokCap > cap(buf) → grew → put tok[:tokCap] AND original buf (disjoint backings when grown). Retain up to 2MB cap.\n2. parseSSEData → parseSSEDataBytes(line []byte) (string, bool): TrimRight/HasPrefix on bytes, returns string(payload) once (single copy) instead of Text()+[]byte double copy. Callers: httpclient scanOpenAISSE, anthropic, github_copilot x2 use scanner.Bytes() path; cloudcode uses bytes too (isSSEKeepAlive gets []byte variant); commandcode already zero-copy.\n3. Codec aliasing safety: chunks are processed immediately, codecs copy into outputs (documented in comment).\n4. Benchmark large-frame stream: BenchmarkSSEScannerLargeFrames comparing allocs.\n5. gofmt, make vet, go test ./..., CodeHealth httpclient.go
<!-- SECTION:PLAN:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
SSE scanner pool now retains grown buffers and the scan path avoids the double line copy. sseScanner's release func detects scanner growth via cap(sc.Bytes()) > cap(original buf) — bufio allocates a fresh backing on overflow, leaving the original puttable — and returns grown buffers to the pool up to sseBufMaxRetain (2MB, the scanner cap), so large-frame streams reuse multi-hundred-KB buffers instead of allocating fresh ones per stream. parseSSEData rewritten as parseSSEDataBytes([]byte): scanner.Bytes() (zero-copy) flows through byte trims/prefix checks into one string(payload) copy handed to the codec — previously Scanner.Text() copy + []byte(payload) copy. Callers migrated: scanOpenAISSE, anthropic, github_copilot x2, cloudcode (plus isSSEKeepAliveBytes); commandcode was already zero-copy. Codec aliasing is safe: payloads are parsed immediately, codecs copy into emitted chunks (documented). Tests: TestSSEScannerPoolRetainsGrownBuffers (grown buffer reused, race-clean) + golden stream tests still pass. Benchmarks: BenchmarkSSEScannerLargeFrames (8x80KB frames: 12 allocs/op) and BenchmarkSSEScannerSmallFrames baseline. gofmt/make vet/full go test clean; CodeHealth httpclient.go 5.81 (== baseline).
<!-- SECTION:FINAL_SUMMARY:END -->
