---
id: TASK-015
title: Resolve Sonar findings
status: In Progress
assignee:
  - '@opencode'
created_date: '2026-09-06 10:01'
updated_date: '2026-09-06 10:15'
labels: []
dependencies: []
modified_files:
  - backend/internal/gateway/admin.go
  - backend/internal/app/app.go
  - backend/internal/connectors/openai_compatible.go
  - backend/internal/transform/anthropic.go
  - backend/internal/capability/tables.go
  - frontend/src/pages/ProviderDetail.tsx
  - frontend/src/components/ModelDetailsModal.tsx
  - frontend/src/lib/api.ts
priority: high
type: chore
ordinal: 18000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Sonar findings cover security-sensitive API boundaries, dashboard accessibility, and accumulated maintainability debt across admin, connector, application, and catalog code. Split delivery prevents high-risk fixes from being obscured by broad refactoring.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 Security and API-boundary findings are resolved with regression coverage
- [ ] #2 Dashboard accessibility and user-facing correctness findings are resolved
- [ ] #3 Backend complexity and duplication findings are reduced without route or protocol regressions
- [ ] #4 Capability table and API type findings are addressed without changing resolution precedence
- [ ] #5 All affected verification suites and Sonar analysis pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [ ] #2 Changes are validated by tests
- [ ] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Fix security/API boundary defects first: safe proxy URL error handling, OAuth postMessage origin/source verification, export passphrase non-URL transport, dynamic API segment/query encoding, import validation parity, and safe upstream error exposure. Add focused regression tests and commit this unit.
2. Fix dashboard accessibility and observable correctness: accessible modal interaction, native model-card activation, zero metric display, mutation/cache recovery, pagination, clipboard handling, then localized TypeScript style cleanup. Typecheck/build and commit this unit.
3. Refactor backend handlers and startup/connectors in independently tested groups. Extract helpers from high-complexity functions without altering routes, initialization order, provider validation, streaming, or request rendering. Commit each group separately.
4. Apply capability-table and API type/literal cleanup while preserving table ordering and resolver precedence. Add resolution tests where declarative helpers replace repeated values.
5. After each commit run targeted tests; finish with gofmt, go vet ./backend/..., go test ./backend/..., frontend typecheck/build, ./scripts/verify.sh, Sonar, Code Health, and final diff review.

Focused unit: refactor only backend/internal/gateway/admin.go adminUpdatePlan. Move its PATCH-field validation and mutation into a typed helper, preserving every existing error literal, status, payload, persistence call, and timestamp behavior. Add focused handler tests for successful multi-field updates and rejected invalid fields, then run gofmt and gateway package tests.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Focused adminUpdatePlan unit complete: extracted typed PATCH validation/mutation into applyPlanUpdate, preserving route/status/error literals/payload/persistence behavior. Added success and invalid-field persistence regression tests. Verified `go test ./backend/internal/gateway -count=1`; Code Health confirms admin.go has pre-existing broader Brain Class and complex-method findings outside this intentionally scoped unit.
<!-- SECTION:NOTES:END -->
