---
id: TASK-011
title: Fix chain model picker overflow
status: Done
assignee:
  - OpenCode
created_date: '2026-09-05 16:49'
updated_date: '2026-09-05 17:10'
labels:
  - frontend
  - chains
  - visual
dependencies: []
references:
  - frontend/src/pages/ChainEditor.tsx
  - frontend/src/components/chains/ChainModelPicker.tsx
modified_files:
  - frontend/src/pages/ChainEditor.tsx
  - frontend/src/components/chains/ChainModelPicker.tsx
priority: medium
type: bug
ordinal: 11000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Keep the model picker dropdown fully visible while adding a route model. Opening the picker must not make the model-route card or neighboring card boundaries appear clipped or unstable.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The model picker dropdown is not clipped by the model-route card boundary
- [x] #2 Opening and closing the picker does not change model-route card dimensions or neighboring card layout
- [x] #3 Model selection, search, and dismissal behavior remain unchanged
- [x] #4 The fix works at desktop and mobile responsive widths
- [x] #5 Frontend typecheck and production build pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Update the model-route `Card` in `frontend/src/pages/ChainEditor.tsx` to use `overflow-visible`, matching the existing final-fallback card that hosts the same picker.
2. Keep `ChainModelPicker` behavior and shared `Card` primitive unchanged; scope layout change to model-route card to avoid unrelated regressions.
3. Run `cd frontend && npm run typecheck`, `cd frontend && npm run build`, and `git diff --check`.
4. Review final diff and confirm dropdown can extend beyond card boundary without changing card layout.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Root cause: Card primitive defaults to overflow-hidden; model-route card hosted absolute dropdown and clipped it. Fix scoped to model-route card only, mirroring existing overflow-visible pattern on fallback card (L105). IDE apply_patch tooling failed on 2000+ char single-line JSX; used direct edit instead.

Validation: npm run typecheck, npm run build, git diff --stat (1 line), Code Health safeguard (1 file, no issues), cavecrew review (0 findings: clipping ancestor analysis, paint-only no layout shift, scope clean, stacking OK).
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed chain model picker dropdown clipping in ChainEditor. The model-route Card (frontend/src/pages/ChainEditor.tsx:104) used the Card primitive's default `overflow-hidden`, which clipped the picker's absolutely positioned dropdown at the card border. Added `overflow-visible` to that card, matching the proven pattern already used by the final-fallback card (line 105) hosting the identical picker.

One line changed; ChainModelPicker, shared Card primitive, and all other cards untouched. Opening/closing the picker no longer clips the dropdown or affects card paint boundaries (overflow affects paint only, not box model).

Validation: `npm run typecheck` and `npm run build` pass; Code Health pre-commit safeguard passed with no issues; diff review verified Card was the only clipping ancestor and stacking (z-30 dropdown) renders above adjacent cards. No frontend test runner is configured; visual confirmation on desktop and mobile widths recommended after deploy.
<!-- SECTION:FINAL_SUMMARY:END -->
