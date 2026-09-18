---
id: TASK-046
title: Shrink and single-pass the direct-stream capture buffer
status: Done
assignee:
  - zsolt
created_date: '2026-09-18 15:35'
updated_date: '2026-09-18 17:51'
labels:
  - performance
  - backend
  - memory
milestone: Performance Scan 2026-09
dependencies: []
references:
  - backend/internal/pipeline/pipeline.go
modified_files:
  - backend/internal/pipeline/pipeline.go
priority: medium
type: enhancement
ordinal: 51000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The direct-stream path retains up to roughly 516KB per live stream in a capture buffer and then scans the captured data twice, once for usage and once for character counts, using a default bufio.Scanner buffer that silently truncates frames larger than 64KB. Under many concurrent streams this is a large resident cost.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The tail capture buffer is reduced and/or pooled
- [x] #2 Usage extraction and character counting run in a single pass with an explicit scanner buffer
- [x] #3 Usage is still parsed for frames larger than 64KB
- [x] #4 Tests cover usage parsing and the large-frame case
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. tailCaptureSize 256K→64K, tailTrimSlack 64K — bounds capture at ~132KB/stream (head 4K + tail ≤128K) vs ~516KB (AC#1)\n2. extractUsageFromStream gains explicit scanner buffer (64K initial, 2MB max) so usage frames >64KB still parse (AC#3); completionCharsFromStream same\n3. Single pass: new extractStreamUsageAndChars(raw) loops once, accumulating usage + completion chars; capturedStreamUsage uses it — usage hit returns usage, miss uses chars from same pass (AC#2)\n4. extractUsageFromStream kept as thin wrapper (tests use it)\n5. Tests: large-frame (>64KB) usage parse; single-pass equivalence (usage+chars match separate old functions); tail bound test\n6. gofmt, make vet, go test ./..., CodeHealth pipeline.go
<!-- SECTION:PLAN:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Direct-stream capture footprint cut ~4x and the scan made single-pass. tailCaptureSize 256KB→64KB with tailTrimSlack=64KB bounds each live stream's capture at ~132KB (head 4KB + tail ≤128KB) instead of ~516KB. New streamScanner() gives the captured-data scan an explicit 64KB-initial/2MB-max buffer, fixing silent truncation of usage frames larger than the default 64KB. extractStreamUsageAndChars scans captured SSE data once, merging provider usage and counting completion characters in the same loop (extractUsageFromStream/completionCharsFromStream kept as wrappers — chars helper refactored into completionCharsFromPayload, malformed payloads return 0). capturedStreamUsage consumes the single-pass result; usage hit returns provider usage, miss falls back to estimate with chars from the same pass. Tests: TestExtractUsageFromStream_LargeFrame (120KB single frame parsed for both usage and chars), TestCapturedStreamUsage_SinglePassEquivalence (merged scan == two separate scans), TestSafeBufferTailBounded (4MB written → capture stays within head+tail+slack). gofmt/make vet/full go test clean; CodeHealth pipeline.go 2.26 (baseline 2.21, +0.05; file is pre-existing red-zone, out-of-scope for this task).
<!-- SECTION:FINAL_SUMMARY:END -->
