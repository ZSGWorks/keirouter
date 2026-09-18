---
id: TASK-044
title: Make ToolArgSanitizer fragment handling linear
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
- [ ] #1 Fragment append and comparison avoid materializing the full accumulated string per chunk
- [ ] #2 Snapshot versus incremental detection behavior is preserved
- [ ] #3 Existing sanitizer tests pass and a benchmark shows linear scaling
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
