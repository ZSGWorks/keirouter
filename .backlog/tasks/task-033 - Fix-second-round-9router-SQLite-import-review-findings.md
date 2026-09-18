---
id: TASK-033
title: Fix second-round 9router SQLite import review findings
status: In Progress
assignee: []
created_date: '2026-09-17 19:56'
updated_date: '2026-09-18 06:19'
labels:
  - security
  - import
  - 9router
  - bugfix
dependencies: []
references:
  - TASK-032
  - backend/internal/gateway/admin_foreign_import_sqlite.go
  - backend/internal/gateway/sqlite_backup.go
  - frontend/src/pages/Settings.tsx
priority: high
type: bug
ordinal: 38000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
9router SQLite import's section controls, upload handling, restore behavior, repeatability, result reporting, and dashboard maintainability need correction before branch can safely merge. The current feature can delete user-selected data without restoring it, exceed its intended upload limit, or leave duplicate custom resources after repeat imports.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 SQLite import enforces its configured upload limit before parsing multipart form data
- [ ] #2 Each selected import section imports only its documented resources and destructive modes never delete an unselected section
- [ ] #3 Windows SQLite restore preserves or restores the live database when replacement fails
- [ ] #4 Repeated merge and overwrite imports do not accumulate imported custom providers or custom models
- [ ] #5 Import results distinguish inserted usage records from skipped existing records and display totals consistently
- [ ] #6 SQLite import UI is split into maintainable components and frontend type checking passes
- [ ] #7 Targeted backend tests cover upload parsing section selection restore failure repeat imports and usage counters
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Approved plan: harden multipart order; dispatch import/deletion by selected section; give custom provider/model imports deterministic provenance and cleanup; make Windows restore rollback-safe; correct usage counters; extract SQLite wizard from Settings.tsx; add focused tests, run full verification, and rerun Code Health.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented bounded multipart parsing, option-specific import/deletion dispatch, deterministic 9router custom-provider/model identity, rollback-safe Windows replacement, accurate usage counters, and extracted SQLite wizard UI. Focused gateway/crypto tests, frontend typecheck, and ./scripts/verify.sh passed. Code Health improved Settings.tsx but still reports complexity degradation in gateway import/restore helpers; task remains In Progress pending follow-up refactoring/review.

Committed implementation as 31acc8e (fix(import): harden 9router SQLite workflow). Full ./scripts/verify.sh passed. SQLite wizard extraction lowered ForeignImportSettings complexity from 53 to 23; Code Health still flags inherited complexity in large gateway importer/restore methods, so task remains In Progress.
<!-- SECTION:NOTES:END -->
