---
id: TASK-001
title: Add Coolify external Postgres deployment
status: Done
assignee: []
created_date: '2026-08-31 14:40'
updated_date: '2026-08-31 15:21'
labels: []
dependencies: []
references:
  - compose.postgres.yaml
  - /Users/zsolt.gajdacs/Zs/Programming/kairouter-vps/compose.yaml
documentation:
  - deploy/README.md
modified_files:
  - compose.coolify-postgres.yaml
  - .env.example
  - deploy/README.md
  - .github/workflows/ci.yml
priority: medium
type: enhancement
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Provide a standalone Docker Compose deployment for Coolify that uses a separately managed PostgreSQL resource, without changing existing local bundled-Postgres deployment behavior.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A standalone Coolify Compose file configures KeiRouter to use an external PostgreSQL DSN
- [x] #2 Existing compose.postgres.yaml local bundled-Postgres workflow remains unchanged
- [x] #3 Environment and deployment documentation distinguish Coolify external Postgres from local bundled Postgres
- [x] #4 CI validates both local Postgres and standalone Coolify Compose configurations
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Update compose.coolify-postgres.yaml to build source with deploy/Dockerfile, forwarding KEIROUTER_VERSION and Coolify SOURCE_COMMIT while preserving external Postgres, network, volume, and no host port.
2. Remove image-pull configuration from .env.example and document optional version stamping.
3. Update Coolify deployment guide to describe source build before container start.
4. Validate rendered Compose and full Docker source build with SOURCE_COMMIT.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Added standalone Coolify Compose file with external Postgres DSN and network. Preserved existing local Postgres overlay; updated environment example, deployment guide, and CI validation.

User selected Coolify source-build deployment. Reopened task to replace the prebuilt-image pull with a Docker Compose build from repository source.

Rendered Coolify Compose and built deploy/Dockerfile successfully with SOURCE_COMMIT=test-sha. Docker output confirmed the binary uses the injected commit stamp.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Added `compose.coolify-postgres.yaml` for Coolify source deployments with external PostgreSQL. Coolify now builds `deploy/Dockerfile` from repository source before starting KeiRouter; build args use `KEIROUTER_VERSION` or Coolify's `SOURCE_COMMIT` for version stamping. The Compose file retains the external Postgres DSN, persistent `/data` volume, external Coolify network, and no host port. Existing local bundled-Postgres deployment remains unchanged.

Updated environment and Coolify deployment instructions, including domain target-port syntax. CI continues to validate both Compose modes.

Validation: rendered standalone Coolify Compose with dummy required values; built `deploy/Dockerfile` using `SOURCE_COMMIT=test-sha` and confirmed stamped build output; `git diff --check`; final diff review clean.
<!-- SECTION:FINAL_SUMMARY:END -->
