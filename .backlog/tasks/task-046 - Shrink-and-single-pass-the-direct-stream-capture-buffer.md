---
id: TASK-046
title: Shrink and single-pass the direct-stream capture buffer
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
- [ ] #1 The tail capture buffer is reduced and/or pooled
- [ ] #2 Usage extraction and character counting run in a single pass with an explicit scanner buffer
- [ ] #3 Usage is still parsed for frames larger than 64KB
- [ ] #4 Tests cover usage parsing and the large-frame case
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
