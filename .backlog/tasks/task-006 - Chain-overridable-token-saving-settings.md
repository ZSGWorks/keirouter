---
id: TASK-006
title: Chain-overridable token saving settings
status: In Progress
assignee: []
created_date: '2026-09-01 10:05'
updated_date: '2026-09-01 12:21'
labels: []
dependencies: []
priority: medium
type: feature
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Allow per-chain tri-state overrides of the five global token-saving features (RTK/slimmer, Caveman, Terse, Headroom, Ponytail), including level overrides (rtk_filter_level, caveman_level, terse_level, ponytail_level). null = inherit global; set = force for requests routed through that chain only. Global settings and other chains unaffected.

Approved plan: /Users/zsolt.gajdacs/.plannotator/plans/chain-overridable-token-saving-2026-09-01-approved.md

Key touchpoints:
- Migration 0030: chains.token_saving TEXT NOT NULL DEFAULT ''
- store.Chain.TokenSaving + repo_budgets.go CRUD
- gateway/resolve.go: resolveResult carries overrides from chainResult
- gateway settings helper: applyChainOverrides on EndpointSettings before slimmer/terse/caveman/headroom/ponytail config resolvers (handlers.go:268, gemini.go:85)
- admin.go chains: validate + persist token_saving; export/import round-trip
- frontend api.ts Chain type; ChainEditor Token saving card (3-way selects + level selects, global value shown)
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 chains table gains token_saving column; store Chain struct and CRUD round-trip the value
- [x] #2 chainResult carries the chain's token_saving blob; non-chain resolutions carry none
- [x] #3 EndpointSettings overlay helper applies 5 toggles + 4 levels per request in handleChat and gemini handler
- [x] #4 Admin API: list/create/update/export/import chains accept and validate token_saving (tri-state, level enums, caveman/terse exclusion)
- [ ] #5 Frontend Chain type extended; ChainEditor gains Token saving card with Inherit/On/Off per feature and level selects
- [x] #6 make vet && make test pass; frontend npm run typecheck passes
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Migration 0030: chains.token_saving TEXT NOT NULL DEFAULT '' (JSON blob, '' = inherit).
2. store.Chain.TokenSaving + CRUD threading (repo_budgets.go).
3. resolve.go: resolveResult.TokenSaving from chainResult; non-chain = empty.
4. gateway settings: ChainTokenSaving tri-state type, parseChainTokenSaving (fail-open), applyChainOverrides (toggles + levels, caveman/terse runtime safety net with explicit-override-wins), effectiveTokenSaving; resolvers refactored to *From(es) variants.
5. handlers.go + gemini.go: build pipeline.Options from effectiveTokenSaving(ctx, resolved.TokenSaving).
6. admin.go: validate + persist token_saving on list/create/update/export/import; level enums mirrored from global rules; save-time caveman/terse exclusion.
7. Tests: override apply, validation, store round-trip, resolve carry-through.
8. Frontend: api.ts ChainTokenSaving/Chain types + payload; ChainTokenSavingCard component (Inherit/On/Off + level selects showing global value); ChainEditor hydration + conflict validation + payload.

Executed as planned; deviations: level overrides added per user feedback (approved mid-plan); runtime exclusion refined so an explicitly overridden feature wins over inherited one.

Regression fix (pending plan approval): modify ChainEditor save payload so edit-mode clearing all token-saving controls sends an explicit empty `token_saving` object. Preserve omitted field for new chains with no overrides. Validate with frontend typecheck/build and manually confirm JSON PATCH clears a pre-existing override.

Regression plan approved and implemented: edit mode now emits `{}` for cleared overrides; creation still omits an absent override.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Frontend edit payload explicitly transmits an empty override object only when clearing an existing chain's overrides. Backend already validates and persists `{}` as full inheritance; no backend change required.

Regression fix: ChainEditor now sends `token_saving: {}` on edit when every token-saving override is inherited. New-chain behavior remains omission of the field. Verified `cd frontend && npm run typecheck` and `cd frontend && npm run build`; `git diff --check` passed. Interactive save/reload verification remains unperformed because no browser test harness is configured.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
## Chain-overridable token saving settings

Per-chain tri-state overrides of the five global token-saving features (RTK/slimmer, Caveman, Terse, Headroom, Ponytail), including level overrides (rtk_filter_level, caveman_level, terse_level, ponytail_level). `null`/absent = inherit global; set = forced for requests routed through that chain only. Global Settings and other chains unaffected.

### Backend
- Migration `0030_chain_token_savings.sql`: `chains.token_saving TEXT NOT NULL DEFAULT ''` (JSON blob, '' = no overrides). SQLite + Postgres.
- `store.Chain.TokenSaving` threaded through ChainRepo Create/Get/ListByTenant/Update.
- `gateway/resolve.go`: `resolveResult.TokenSaving` populated by `chainResult`; direct provider/model and alias resolutions carry none.
- `gateway/settings.go`: `ChainTokenSaving` type (tri-state pointers), `parseChainTokenSaving` (fail-open: malformed blob → inherit), `applyChainOverrides` (overlays toggles + levels onto `EndpointSettings`; runtime caveman/terse safety net — explicitly overridden feature wins), `effectiveTokenSaving` resolver. Existing config resolvers refactored into `*From(es)` variants.
- `handlers.go` / `gemini.go`: pipeline Options now built from `effectiveTokenSaving(ctx, resolved.TokenSaving)` so chain overrides apply per request.
- `admin.go`: list/create/update/export/import chains accept `token_saving`; `validateChainTokenSaving` enforces level enums (mirrors global rules) + caveman/terse mutual exclusion at save time; shared `applyTokenSavingToChain` / `chainExportEntry` / `chainFromImport` helpers.

### Frontend
- `api.ts`: `ChainTokenSaving` type, `Chain.token_saving`, chain create/update payload fields.
- New `components/chains/ChainTokenSavingCard.tsx`: Token saving card with per-feature Inherit/On/Off segmented controls, level override selects (incl. "Global (x)" option), live global-value display; pure helpers `buildTokenSaving`/`savedTokenSavingState`/`isTokenSavingConflict`.
- `ChainEditor.tsx`: card wired after Final fallback; hydration from saved blob; payload includes `token_saving` only when overrides set; client-side caveman+terse conflict validation.

### Tests
- `gateway/chain_token_saving_test.go`: parse (empty/invalid/valid), apply (nil/partial/levels/exclusion), effective, validation (levels, conflict, nil), resolve carries chain overrides.
- `store/repo_chain_token_saving_test.go`: DB round-trip (set, empty, update-clears).
- Full suite: 43 packages ok, 0 fail; `make vet` clean; `gofmt` clean; frontend `typecheck` + `build` pass.

### Behavior notes
- Requests hitting `provider/model` directly: global only, unchanged.
- Chain with empty blob: 100% inherit — zero behavior change for existing chains.
- Headroom URL/timeout always global (headroom is fail-open and no-ops without a global URL).
- Settings read per request already; one extra JSON unmarshal per chain-routed request is negligible.

### Risks/follow-ups
- Per-key or per-model overrides could reuse the guardrails-style scope pattern later.
- admin.go remains a large file (pre-existing); new code kept in small helpers to avoid worsening complexity.
<!-- SECTION:FINAL_SUMMARY:END -->
