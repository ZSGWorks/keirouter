---
id: TASK-031
title: Apply repo-wide gofmt to backend Go sources
status: In Progress
assignee: []
created_date: '2026-09-17 13:29'
updated_date: '2026-09-17 13:44'
labels:
  - chore
  - formatting
dependencies: []
references:
  - backend/internal/health/probe.go
  - backend/internal/app/bootstrap.go
priority: low
type: chore
ordinal: 37000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
`gofmt -l backend` reports 35 dirty Go files, not just the two originally listed. All diffs are whitespace/alignment only (field/comment alignment), and nothing in scripts/, Makefile, or .github enforces gofmt, so the drift survived.

Scope (user-approved): repo-wide backend formatting. Goal: `gofmt -l backend` returns empty in one isolated, formatting-only commit, with a `git diff -w` empty check proving no behaviour change.

Notable dirty files include security-sensitive crypto/, vault/, and oauth/ sources, so the no-behaviour-change gate matters.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 gofmt -w applied across all dirty backend Go files (gofmt -l backend list)
- [ ] #2 gofmt -l backend prints nothing
- [ ] #3 git diff -w is empty (formatting only, no behaviour change)
- [ ] #4 make vet and make test pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
## Plan (approved)

Repo-wide backend gofmt, formatting-only.

1. Rewrite this task to the repo-wide scope (done via CLI).
2. `gofmt -l backend | xargs gofmt -w`.
3. Prove formatting-only: `git diff -w` must be empty (any output = real change, stop and inspect).
4. Confirm `gofmt -l backend` prints nothing.
5. `make vet` and `make test`.
6. Commit one isolated `style(backend): apply gofmt across backend Go sources` commit.
7. Finalize with checked AC + final summary.

Verification: gofmt -l backend empty; git diff -w empty; make vet; make test. Frontend typecheck not required (Go-only).
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Starting implementation. Scope expanded from 2 to 35 files per user-approved plan.
<!-- SECTION:NOTES:END -->
