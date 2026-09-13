---
id: TASK-027.02
title: Cooldown-reset buttons in dashboard
status: Done
assignee: []
created_date: '2026-09-13 10:49'
updated_date: '2026-09-13 11:14'
labels: []
dependencies:
  - TASK-027.01
parent_task_id: TASK-027
priority: medium
type: feature
ordinal: 33000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Operators manage accounts in one place and diagnose cooldown-affected routing in another. A reset action must be reachable from both so nobody has to hunt for it during an incident.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Each account row offers a reset action next to the existing manual actions
- [x] #2 The health dashboard offers a tenant-wide reset action
- [x] #3 Both actions confirm before clearing and report what was cleared afterwards
- [x] #4 Frontend typecheck passes
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Review-fix pass per approved plan (plan-review-fixes-cooldown-mod-2026-09-13-approved.md). 1) New components/CooldownResetModal.tsx: shared confirm modal (Modal + info box + ErrorBanner + ui Buttons). 2) api.ts: move CooldownResetResult + formatCooldownReset next to reset clients (type-only). 3) ProviderDetail: replace inline modal with shared component, wire error + reset(). 4) ProviderHealth: runResetAll -> useMutation, replace modal, drop toast.error for inline banner. 5) Verify typecheck/build/diff-check/safeguard, then re-finalize.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented per approved plan; one deviation: extracted TestErrorBanner subcomponent after safeguard flagged the AccountRow LOC delta (+3 from onReset threading) — re-run passed with ProviderDetail stable and ProviderHealth improved.

Verification: npm run typecheck clean; npm run build succeeds (1.87s); git diff --check clean; safeguard quality_gates passed; frontend POST paths match backend routes (admin.go:46 tenant, admin.go:130 account) covered by 027.01/027.03 backend tests.

Review-fix pass: shared CooldownResetModal (both inline modals replaced, ~80 lines removed for ~70 added once); ProviderHealth runResetAll replaced with useMutation mirroring ProviderDetail; failures now inline via ErrorBanner with reset() on close; CooldownResetResult/formatCooldownReset moved next to reset clients.

Verification (post-fix): typecheck clean, build 1.66s ok, diff-check clean, safeguard quality_gates passed over 4 files. One accidental line-merge during editing caught via grep and restored before verification.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Dashboard cooldown-reset actions shipped, then deduplicated per review. api.ts: CooldownResetResult + resetAccountCooldown/resetAllCooldowns clients with formatCooldownReset reporter colocated in the health section. New components/CooldownResetModal.tsx: single shared confirm dialog (Modal + info box + inline ErrorBanner + ui Buttons) used by both placements. ProviderDetail: TimerOff row button, onReset threading, useMutation with accounts/providers invalidation and count toasts, stale-error reset() on close. ProviderHealth Overview: header reset button, useMutation (replacing hand-rolled async), inline error banner instead of toast.error. Verified: typecheck clean, vite build succeeds, diff-check clean, safeguard passed (ProviderDetail stable, ProviderHealth improved). No browser click-through (no harness); behavior verified via types, build, and route-contract match to tested backend endpoints.
<!-- SECTION:FINAL_SUMMARY:END -->
