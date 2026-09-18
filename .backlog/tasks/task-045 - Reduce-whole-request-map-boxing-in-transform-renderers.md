---
id: TASK-045
title: Reduce whole-request map boxing in transform renderers
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
  - backend/internal/transform/kiro.go
  - backend/internal/transform/openai_responses.go
  - backend/internal/transform/anthropic.go
  - backend/internal/transform/openai.go
modified_files:
  - backend/internal/transform/kiro.go
  - backend/internal/transform/openai_responses.go
  - backend/internal/transform/anthropic.go
  - backend/internal/transform/openai.go
priority: high
type: enhancement
ordinal: 50000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Kiro and OpenAI Responses renderers box every message and part into map[string]any and then reflection-marshal, while the Anthropic and OpenAI codecs redundantly re-decode and re-encode message content. Each large request is materialized two to four times, dominated by tool histories.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Renderers build typed structures or json.RawMessage and marshal once where practical
- [ ] #2 Golden-payload equivalence tests prove the emitted upstream JSON is unchanged
- [ ] #3 An allocation or benchmark test measures the reduction
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->
