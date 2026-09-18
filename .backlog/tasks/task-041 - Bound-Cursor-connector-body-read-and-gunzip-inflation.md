---
id: TASK-041
title: Bound Cursor connector body read and gunzip inflation
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
  - backend/internal/connectors/cursor.go
  - backend/internal/connectors/cursor_protobuf.go
modified_files:
  - backend/internal/connectors/cursor.go
  - backend/internal/connectors/cursor_protobuf.go
priority: high
type: bug
ordinal: 46000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Cursor connector reads the entire upstream response with io.ReadAll and no limit, then gunzips decompressed frames with an unbounded io.ReadAll. A large or crafted compressed frame can inflate to an arbitrary size, creating an out-of-memory spike per request.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 The upstream response read is bounded by a maximum size limit
- [ ] #2 Decompression output is bounded and oversized input is rejected without unbounded allocation
- [ ] #3 A test covers the oversized/inflated case
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
