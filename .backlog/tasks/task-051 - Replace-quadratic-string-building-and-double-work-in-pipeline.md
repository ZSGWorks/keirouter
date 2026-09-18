---
id: TASK-051
title: Replace quadratic string building and double work in pipeline
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
  - backend/internal/pipeline/pipeline.go
  - backend/internal/normalizer/normalizer.go
modified_files:
  - backend/internal/pipeline/pipeline.go
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
- [ ] #1 The stream drain accumulates through a builder written once at the end
- [ ] #2 The normalizer runs once unless a compression actually removed parts
- [ ] #3 Existing pipeline tests pass and a test covers the second-pass condition
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
