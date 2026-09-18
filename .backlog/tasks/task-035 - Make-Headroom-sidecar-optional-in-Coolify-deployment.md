---
id: TASK-035
title: Make Headroom sidecar optional in Coolify deployment
status: Done
assignee: []
created_date: '2026-09-18 14:55'
updated_date: '2026-09-18 14:58'
labels: []
dependencies: []
references:
  - .plannotator/plans/make-headroom-sidecar-optional-2026-09-18-approved.md
  - compose.coolify-postgres.yaml
  - deploy/README.md
  - scripts/verify.sh
modified_files:
  - compose.coolify-postgres.yaml
  - deploy/README.md
  - scripts/verify.sh
priority: medium
type: feature
ordinal: 40000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The Coolify Compose stack (compose.coolify-postgres.yaml) hard-requires the Headroom sidecar: keirouter has depends_on headroom with condition service_healthy, so the deployment will not start unless the ~1-1.5 GB sidecar is healthy. Operators who do not want the memory cost need a way to skip the sidecar while keeping KeiRouter's fail-open compression behavior. Docker Compose cannot conditionally omit a service from a custom boolean env var; only profiles do that, activated by the built-in COMPOSE_PROFILES var.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 compose.coolify-postgres.yaml headroom has profiles: ["headroom"] and keirouter depends_on.headroom is required: false
- [x] #2 Default docker compose -f compose.coolify-postgres.yaml config exits 0 with no headroom service
- [x] #3 With COMPOSE_PROFILES=headroom both services are active and the dependency is health-gated
- [x] #4 deploy/README.md Coolify env-var list documents optional COMPOSE_PROFILES=headroom with fail-open note
- [x] #5 scripts/verify.sh validates both disabled and enabled compose paths
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Approved plan: .plannotator/plans/make-headroom-sidecar-optional-2026-09-18-approved.md

1. Add profiles: ["headroom"] to the headroom service in compose.coolify-postgres.yaml.
2. Set keirouter depends_on.headroom required: false (profile-disabled dependency otherwise hard-errors; verified in compose-go graph/services.go:63-67).
3. Document optional COMPOSE_PROFILES=headroom in deploy/README.md Coolify env-var block and Headroom Sidecar Resources section.
4. Add a second config validation in scripts/verify.sh with COMPOSE_PROFILES=headroom.
5. Validate both compose paths read-only with docker compose config; run ./scripts/verify.sh.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Verified Compose v5.1.2: default config exit 0 with only keirouter; COMPOSE_PROFILES=headroom yields headroom+keirouter with depends_on required:false and condition service_healthy. Confirmed in compose-spec/compose-go graph/services.go:63-67 that a profile-disabled required dependency hard-errors, so required:false is mandatory. Considered refactoring the duplicated env block in verify.sh; 4-line duplication mirrors the existing pattern and a helper would add indirection for no real gain.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Made the Headroom sidecar optional in compose.coolify-postgres.yaml via profiles: [headroom] and required: false on keirouter's dependency. Setting COMPOSE_PROFILES=headroom in Coolify loads the sidecar (health-gated); unsetting it skips the sidecar and KeiRouter stays fail-open. Documented the optional variable in deploy/README.md and added an enabled-path compose validation to scripts/verify.sh. Verified with both read-only docker compose config paths and a full ./scripts/verify.sh pass; pre-commit Code Health gate passed.
<!-- SECTION:FINAL_SUMMARY:END -->
