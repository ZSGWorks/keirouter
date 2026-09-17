---
id: TASK-029
title: Harden cooldown card review findings
status: Done
assignee: []
created_date: '2026-09-17 11:58'
updated_date: '2026-09-17 11:58'
labels: []
dependencies: []
references:
  - backend/internal/gateway/admin_cooldowns.go
  - backend/internal/store/repo_routing.go
  - frontend/src/components/CooldownCard.tsx
priority: low
type: chore
ordinal: 36000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Post-PR review of TASK-028 cooldown card surfaced code-quality findings. Captures the follow-up fixes made before merge so the change set stays traceable.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 adminListCooldowns groups use a typed response struct instead of an unchecked map type assertion
- [x] #2 ListActiveModelCooldowns surfaces scan failures instead of skipping rows
- [x] #3 Cooldown card disables row Clear while any reset is pending, clamps long model lists, fixes sub-second retry copy, drops the unrelated providers invalidation
- [x] #4 gofmt/go vet/go test, frontend typecheck, ./scripts/verify.sh, and Code Health safeguard all pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Fixed review findings: (1) cooldownProviderGroup typed struct sort; (2) ListActiveModelCooldowns returns scan error; (3) seedCooldownAccount delegates to createCooldownAccount; (4) row Clear disabled during any pending reset; (5) fmtModels clamps to first 3 + "+N more" with full list in title; (6) fmtRetry "<1s" for parked sub-second; (7) dropped ["providers"] invalidation. Verified gofmt/vet/test, frontend typecheck, ./scripts/verify.sh exit 0, Code Health safeguard passed.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Follow-up hardening from the TASK-028 review. Backend: cooldownProviderGroups now returns a typed []cooldownProviderGroup (no unchecked type assertion); ListActiveModelCooldowns returns a wrapped error on scan failure instead of skipping the row; test seeding deduped. Frontend CooldownCard: row Clear buttons disable while any reset is pending; long model lists clamp to 3 + "+N more" (full list on hover); fmtRetry reports "<1s" for parked sub-second cooldowns; dropped the unrelated "providers" query invalidation. Verification: gofmt, go vet ./..., go test ./..., npm run typecheck, npm run build, ./scripts/verify.sh (exit 0), Code Health pre-commit safeguard passed.
<!-- SECTION:FINAL_SUMMARY:END -->
