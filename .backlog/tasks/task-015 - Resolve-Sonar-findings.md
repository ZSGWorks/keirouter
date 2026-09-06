---
id: TASK-015
title: Resolve Sonar findings
status: In Progress
assignee:
  - '@opencode'
created_date: '2026-09-06 10:01'
updated_date: '2026-09-06 10:31'
labels: []
dependencies: []
modified_files:
  - backend/internal/gateway/admin.go
  - backend/internal/app/app.go
  - backend/internal/connectors/openai_compatible.go
  - backend/internal/connectors/connectors_test.go
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

Focused OpenAI-compatible connector unit: extract small helpers from `headers`, `Chat`, and `Validate` only. Preserve provider-specific header literals/auth ordering, validation fallbacks, and stream-required retry behavior. Add or adjust focused connector tests, then run gofmt and `go test ./backend/internal/connectors -count=1`.

Focused cleanup: in backend/internal/capability/tables.go, declare shared constants for only repeated Claude thinking-format literals (claude-adaptive and claude-budget), replace their existing values in exact and ordered pattern entries without moving entries or changing fields. Retain existing resolution-chain tests; run gofmt and go test ./backend/internal/capability -count=1.

Focused Anthropic transform unit: limit changes to backend/internal/transform/anthropic.go plus transform tests. Extract request max-token/thinking setup, request metadata, and per-content-block rendering helpers from RenderRequest/renderAntBlocks without changing JSON output or thinking signature forwarding. Correct confirmed ParseResponse thinking extraction to use antBlock.Thinking, with a regression test. Run gofmt and focused transform tests.

Focused provider-model unit: refactor only `backend/internal/gateway/admin.go` `adminProviderModels` and relevant gateway tests. Extract response-model construction, catalog assembly, and credential/public live-discovery helpers while preserving static catalog then live then custom model ordering, custom/database markers, kind filtering/response kind, price/capability payload fields, credential iteration, and public fallback conditions. Run gofmt and `go test ./backend/internal/gateway -count=1`.

6. Focused app startup unit: extract ordered database bootstrap, cache, persisted endpoint settings, and guardrail setup helpers from Build. Preserve every constructor dependency, migration/tenant/cooldown sequence, cleanup ownership, and returned App fields. Run gofmt and go test ./backend/internal/app -count=1.

Focused export-passphrase security unit: replace the GET `/settings/database?passphrase=...` contract with POST JSON `{passphrase}`. Decode only this field server-side, preserve portable-export behavior and response payload/download flow, update frontend `api.exportDatabase`, and add focused gateway regression coverage proving body passphrase enables portable export while query passphrase is ignored. Run gofmt, focused gateway tests, and frontend typecheck.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Focused adminUpdatePlan unit complete: extracted typed PATCH validation/mutation into applyPlanUpdate, preserving route/status/error literals/payload/persistence behavior. Added success and invalid-field persistence regression tests. Verified `go test ./backend/internal/gateway -count=1`; Code Health confirms admin.go has pre-existing broader Brain Class and complex-method findings outside this intentionally scoped unit.

Starting requested focused connector refactor limited to `backend/internal/connectors/openai_compatible.go` and its tests.

Research confirmed Anthropic ParseResponse currently maps thinking blocks from b.Text even though antBlock uses the thinking JSON key. Scoped correction and regression coverage are within requested transform-only unit.

Focused capability-table literal cleanup complete: added shared constants for claude-adaptive and claude-budget, replaced only repeated values across existing exact and ordered pattern entries. Table order and fields remain unchanged; existing resolution-chain coverage passed with `go test ./backend/internal/capability -count=1`. Ran gofmt on backend/internal/capability/tables.go.

Focused Anthropic transform unit complete. Extracted token/thinking reconciliation, request metadata, message aggregation, and content-block rendering helpers from RenderRequest/renderAntBlocks. Existing thinking signature round-trip coverage remains passing; added ParseResponse regression confirming the Anthropic thinking field populates canonical thinking content. Verified gofmt, `go test ./backend/internal/transform -count=1`, IDE error lint, and Code Health 9.09 (only pre-existing parseAntMessage finding remains). No commit created.

Focused OpenAI-compatible connector refactor complete. Extracted provider header builders with named User-Agent constants, shared stream fallback helpers, and validation success/error branches. Added header regression table covering Azure, Cline, CodeBuddy, AgentRouter, Kimchi, and header override behavior. Verified `gofmt`, `go test ./backend/internal/connectors -count=1`, `git diff --check`, and IDE error lint. Code Health remains 8.1; remaining findings are pre-existing `drainStreamToResponse`, `Stream`, `ListModels`, and `validateProbe` work outside requested Headers/Chat/Validate scope.

Added a dynamic-provider regression test for HTTP 400 `Stream must be set to true`: Chat retries once using SSE and returns the drained response. Re-ran `gofmt`, `go test ./backend/internal/connectors -count=1`, and `git diff --check`.

Research: Build has Code Health cyclomatic complexity 34. Scoped minimal refactor to named startup lifecycle helpers only; Run, runtime behavior, and App fields remain unchanged.

Focused `adminProviderModels` refactor complete. Extracted pricing/response construction, custom-model lookup, catalog assembly, authenticated discovery, and public fallback helpers. Preserved validation, static catalog ordering, custom/database markers, live append behavior, credential iteration, public fallback gate, and response `kind`/`kinds` payloads. Existing focused provider-model pricing and multimodal-kind tests cover preserved response fields; verified `gofmt`, `go test ./backend/internal/gateway -count=1`, `git diff --check`, IDE compile, and error lint. Code Health improved from 2.18 to 2.25; remaining warnings are unrelated legacy methods/module size. No commit created.

Focused app startup unit complete. Extracted openDatabase, buildSemanticCache, configureEndpointSettings, and buildGuardrails lifecycle helpers from Build. Preserved database open → migrate → default tenant → cooldown cleanup ordering; cache/endpoint/guardrail constructor order; retention ownership; and every returned App field. Verified gofmt, go test ./backend/internal/app -count=1, git diff --check, and IDE error lint. Code Health: app.go 7.13 → 7.50; Build cyclomatic complexity 34 → 13. No commit created.

Focused export-passphrase security unit complete. Replaced GET `/settings/database?passphrase=...` with POST `/settings/database/export` and JSON `{passphrase}`; retained POST `/settings/database` import. The frontend download flow now posts body data, so passphrases cannot appear in request URLs. Added gateway regression coverage proving only JSON-body passphrases are used and unknown fields fail. Verified `gofmt`, `go test ./backend/internal/gateway -count=1`, `npm run typecheck` in `frontend`, IDE error lint, IDE targeted build, and `git diff --check`. No commit created.
<!-- SECTION:NOTES:END -->
