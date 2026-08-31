---
id: TASK-005
title: Enforce source-only deployments
status: Done
assignee:
  - '@OpenCode'
created_date: '2026-08-31 19:51'
updated_date: '2026-08-31 19:53'
labels:
  - deployment
  - coolify
  - ownership
dependencies: []
references:
  - compose.yaml
  - compose.coolify-postgres.yaml
  - deploy/README.md
  - AGENTS.md
priority: medium
type: chore
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Ensure KeiRouter deployment configuration and documentation support source builds only, especially Coolify. Prevent registry-target tagging or pre-built image guidance that could lead to accidental GitHub Container Registry publication.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Default deployment configuration has no GitHub Container Registry image target
- [x] #2 Coolify continues to build deploy/Dockerfile from source without an image target
- [x] #3 Deployment documentation describes source builds and does not instruct users to pull pre-built GitHub images
- [x] #4 Repository documentation accurately states no image publishing workflow exists
- [x] #5 Rendered Compose configurations validate successfully
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Remove the root Compose `image:` default so `docker compose up --build` creates only a local source-built image with no registry target.
2. Rewrite deployment documentation to remove pre-built GHCR pull/tag instructions and make source-based Docker Compose, local installer, and Coolify paths explicit; retain existing source-build update commands.
3. Update the repository map in `AGENTS.md` to state no GitHub workflow or Docker publishing automation remains.
4. Verify no registry/push publication paths remain, render root and PostgreSQL Compose configurations, and confirm Coolify Compose retains only its source build.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Removed the root Compose GHCR `image:` target while retaining its local `build` definition. Replaced pre-built GHCR deployment documentation with explicit source-build guidance and clarified that Coolify builds `deploy/Dockerfile` directly. Updated repository guidance to state no CI, release, or Docker publishing automation exists. Verified `docker compose -f compose.yaml config`, `docker compose -f compose.yaml -f compose.postgres.yaml config`, and Coolify Compose rendering with required test-only environment values. Rendered services contain source builds and no image target; tracked-file scan found no GHCR reference, Docker push, Buildx push, or Compose push command outside excluded history/task records.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Enforced source-only deployment configuration. Root Docker Compose no longer assigns a GHCR image target, so local Docker and Coolify deployments build from the repository without a registry destination. Deployment documentation now removes pre-built-image pull/tag claims and explains source build behavior; agent guidance records that no image publishing automation exists.

Verified root, PostgreSQL, and Coolify Compose rendering. The Coolify configuration builds `deploy/Dockerfile` without an `image:` target. Repository scan found no GHCR or image-push command in current tracked operational files. Existing remote package inventory remains unavailable to this token because GitHub requires `read:packages`; that does not affect future publication prevention.
<!-- SECTION:FINAL_SUMMARY:END -->
