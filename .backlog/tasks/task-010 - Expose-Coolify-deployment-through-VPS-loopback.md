---
id: TASK-010
title: Reach Coolify deployment through shared coolify network
status: Done
assignee:
  - OpenCode
created_date: '2026-09-05 11:53'
updated_date: '2026-09-08 08:03'
labels:
  - coolify
  - docker
  - networking
dependencies: []
references:
  - compose.coolify-postgres.yaml
documentation:
  - deploy/README.md
modified_files:
  - compose.coolify-postgres.yaml
  - .env.example
  - deploy/README.md
priority: medium
type: enhancement
ordinal: 10000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Allow Docker workloads on the Coolify VPS to reach KeiRouter using the shared coolify network they already join, without host ports or the VPS public address.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 KeiRouter deployed through Coolify is resolvable from other deployments through the shared coolify network by service name
- [ ] #2 Docker workloads can use the documented service endpoint without extra per-deployment Compose changes
- [x] #3 No public host port; loopback-only binding permitted
- [ ] #4 Coolify domain routing and external PostgreSQL connectivity continue to work
- [x] #5 Deployment documentation covers the shared coolify network access path
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Remove the Docker bridge port publishing from `compose.coolify-postgres.yaml`; KeiRouter stays internal to the shared `coolify` network, which Coolify proxy routing and external Postgres resolution already require.
2. Remove the bridge port setting from `.env.example`.
3. Update `deploy/README.md`: client containers on the `coolify` network call `http://keirouter:20180` by service name; no per-deployment changes needed. Note that container `localhost` remains self-only.
4. Revert `scripts/verify.sh` to plain Coolify Compose config validation.
5. Validate rendered Compose configuration, run full verification, and confirm final review passes. Live VPS verification of service-name resolution and Coolify domain/Postgres health remains deployment-time.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented Docker bridge access: Coolify Compose now binds KeiRouter only to 172.17.0.1 with configurable `KEIROUTER_DOCKER_PORT`. Deployment docs show `host.docker.internal:host-gateway` client setup and warn against public/wildcard binding. `scripts/verify.sh` now fails if rendered Coolify Compose no longer binds to the Docker bridge.

Validated locally: `bash -n scripts/verify.sh`, `./scripts/verify.sh`, rendered Coolify Compose with an overridden port, `git diff --check`, Code Health safeguard (passed; no eligible source files), and final diff review (no issues).

Pending VPS deployment: verify an independent client container reaches `http://host.docker.internal:<port>`, verify no VPS LAN/public access to port, and confirm Coolify domain plus external PostgreSQL remain healthy.

Pivot per user decision: every deployment already joins the shared coolify network, so service-name resolution replaces host/bridge publishing. Removed ports, KEIROUTER_DOCKER_PORT, host-gateway docs, and verifier bridge assertion.

Shared-network implementation complete: `compose.coolify-postgres.yaml` and `.env.example` restored to no host port; `deploy/README.md` documents `KEIROUTER_URL=http://keirouter:20180`, fixed default port note, localhost caveat, and duplicate service-name round-robin warning. `scripts/verify.sh` reverted to plain Coolify config validation.

Validated: `bash -n scripts/verify.sh`, rendered Coolify Compose (no ports, coolify network only), `./scripts/verify.sh`, `git diff --check`, Code Health safeguard, final reviews clean (warnings only).

Pending VPS deployment: confirm a client container resolves `http://keirouter:20180` via the coolify network and Coolify domain plus external PostgreSQL stay healthy.

Closure verification 2026-09-08: User confirmed the 127.0.0.1:20180:20180 loopback binding in compose.coolify-postgres.yaml:12-13 is intentional and needed. AC #3 reworded from "No host port is published" to "No public host port; loopback-only binding permitted" to match the intentional compose binding. Fixed deploy/README.md:145 contradiction — now documents loopback-only binding instead of claiming no host port. AC #3 and #5 (docs) checked as repo-verifiable met. AC #1, #2 (live service-name resolution/use) and AC #4 (Coolify domain + external Postgres connectivity) require VPS deployment verification — left unchecked, deferred to deployment-time.
<!-- SECTION:NOTES:END -->
