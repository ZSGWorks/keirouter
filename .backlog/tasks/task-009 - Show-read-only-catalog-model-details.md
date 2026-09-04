---
id: TASK-009
title: Show read-only catalog model details
status: Done
assignee: []
created_date: '2026-09-04 04:41'
updated_date: '2026-09-04 08:03'
labels:
  - providers
  - models
  - dashboard
dependencies: []
references:
  - frontend/src/pages/ProviderDetail.tsx
  - frontend/src/components/CustomModelsSection.tsx
  - frontend/src/lib/api.ts
priority: medium
type: feature
ordinal: 9000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Make provider catalog model cards open a read-only detail modal so operators can inspect each model's supported capabilities and token pricing without entering an edit flow.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Clicking a catalog model card opens a read-only model detail modal
- [x] #2 Modal lists every supported model capability as readable text
- [x] #3 Modal displays available model pricing in clear per-million-token units
- [x] #4 Modal does not allow catalog model metadata to be edited
- [x] #5 Modal is accessible by keyboard and closes through established modal controls
- [x] #6 Relevant backend and frontend verification passes
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Extend connector pricing catalog support with an atomic snapshot of successful models.dev prices, updated together with fetched model specs by the pricing refresher. Add a display-price resolver that returns provider/static catalog data first and falls back to the matching models.dev snapshot only when provider data is absent.
2. Extend `adminProviderModels` in `backend/internal/gateway/admin.go` to attach optional resolved pricing to every catalog or discovered model. Serialize per-million USD rates, long-context metadata, estimate/free status, and source provenance; omit pricing only when neither source has an unambiguous price so unknown cost is never rendered as free.
3. Add connector and gateway coverage for precedence (provider data wins), models.dev fallback, and no-match omissions. Preserve capability, custom, and discovery behavior.
4. Extend `ProviderModel` API contract in `frontend/src/lib/api.ts` with optional pricing metadata matching response shape.
5. In `frontend/src/pages/ProviderDetail.tsx`, track clicked catalog model and render a new read-only detail dialog using shared `Modal`. Make each catalog model card keyboard- and pointer-activatable; keep checkbox, enable/disable, and copy controls independent by preventing card activation from their events.
6. Populate modal with model identity/path, kind, context/output limits, a readable list of all enabled capabilities, and a pricing section with named available per-million-token rates. Identify estimated rates, support explicit-free models, link source when supplied, and show an explicit unavailable state only when both provider and models.dev data are unavailable. Provide only a Close action.
7. Run `gofmt` and targeted Go tests for gateway/connectors, then `make vet`, `make test`, and `cd frontend && npm run typecheck`; run `./scripts/verify.sh` before handoff because change spans backend and frontend.

Continuation approved 2026-09-04: add atomic fetched catalog regression coverage, finish non-nested card keyboard semantics, expand pricing response assertions, then rerun backend/frontend/project verification before finalization.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented read-only catalog/discovered model details with capability, pricing, free/estimated, source, and unavailable states. Fetched models.dev models and prices now publish and read as one snapshot; provider catalog prices remain authoritative. Added connector, app refresher, and gateway response coverage. Verification passed: targeted connector/gateway/app tests, make vet, make test, frontend typecheck/build, git diff --check, Code Health safeguard, final diff review, and ./scripts/verify.sh including PostgreSQL, Compose, and Docker. UI click/keyboard/modal dismissal behavior requires browser exercise before acceptance criteria 1-5 can be checked.

User manually verified catalog-card activation, read-only details, capabilities, pricing, and modal keyboard/close behavior.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Implemented read-only provider catalog and discovered-model details. Model cards open an accessible shared modal showing identity, kind, context/output limits, capabilities, pricing per million tokens, source provenance, estimates, free status, and unavailable pricing. Added atomic models.dev model/price snapshots with provider-catalog pricing precedence, API payload typing, and connector/app/gateway regression coverage. Nested model controls remain independent. Verification passed: targeted Go tests, make vet, make test, frontend typecheck/build, Code Health safeguard, git diff check, and ./scripts/verify.sh including PostgreSQL, Compose, and Docker checks. User manually verified UI behavior and modal controls.
<!-- SECTION:FINAL_SUMMARY:END -->
