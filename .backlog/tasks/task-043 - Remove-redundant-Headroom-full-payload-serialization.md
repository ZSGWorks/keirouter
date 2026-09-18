---
id: TASK-043
title: Remove redundant Headroom full-payload serialization
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
- [ ] #1 The OpenAI message mapping is built once and reused for the POST body and both size measurements
- [ ] #2 Bytes-before and bytes-after values are unchanged from current behavior
- [ ] #3 An allocation or benchmark test demonstrates the reduction
- [ ] #4 Existing Headroom tests pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
