---
id: TASK-018
title: Fix Compose Headroom compression access
status: Done
assignee:
  - '@codex'
created_date: '2026-09-07 07:34'
updated_date: '2026-09-07 08:45'
labels: []
dependencies: []
references:
  - >-
    https://github.com/headroomlabs-ai/headroom/blob/main/docs/content/docs/proxy.mdx
modified_files:
  - compose.yaml
  - compose.coolify-postgres.yaml
  - backend/internal/headroom/compressor.go
  - backend/internal/headroom/probe.go
  - backend/internal/headroom/compressor_test.go
  - backend/internal/pipeline/headroom_e2e_test.go
  - scripts/verify.sh
priority: high
type: bug
ordinal: 21000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The bundled Headroom v0.36.0 sidecar is healthy, but KeiRouter calls POST /v1/compress from a separate container. Headroom deliberately returns HTTP 404 for non-loopback callers unless remote compression access is explicitly enabled, leaving the enabled feature permanently fail-open in Compose and Coolify. The integration also decodes an obsolete nested metrics shape while the documented v0.36.0 endpoint returns top-level token metrics.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 The bundled Compose and Coolify Headroom sidecars accept KeiRouter requests to POST /v1/compress and the dashboard connection test succeeds.
- [x] #2 The Headroom sidecar remains unpublishable from the host and is reachable only through a dedicated private network shared with KeiRouter.
- [x] #3 Native macOS/Linux operation remains loopback-only and does not enable remote compression access.
- [x] #4 KeiRouter records the documented flat v0.36.0 compression metrics while retaining valid fail-open behavior for unavailable or malformed responses.
- [x] #5 Automated unit, settings, Compose smoke, typecheck, build, and repository verification checks pass.
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Confirm the pinned v0.36.0 endpoint contract against the official proxy documentation and the local container, including its intentional 404 behavior for non-loopback /v1/compress calls.
2. Update compose.yaml and compose.coolify-postgres.yaml to set HEADROOM_COMPRESS_ALLOW_REMOTE=1 only on the sidecar. Give the sidecar and KeiRouter a dedicated, service-membership-private network with no Headroom host port. Do not mark the network Docker-internal: Headroom needs controlled outbound egress for a cold local model download; persist its Hugging Face cache under the existing private state volume.
3. Align backend/internal/headroom response decoding with the documented flat tokens_before, tokens_after, tokens_saved, and compression_skipped fields. Preserve the user-message request config; treat skipped, malformed, unavailable, or unchanged failure responses as fail-open rather than successful compression.
4. Keep native runtime startup and readiness bound to 127.0.0.1; add regression tests proving Compose resolution targets the sidecar while native behavior remains loopback-only.
5. Extend the Compose smoke check to make the actual KeiRouter-to-sidecar connection probe after Headroom readiness, reject skipped compression, and verify the dashboard settings API reports success while the sidecar has no host port.
6. Run gofmt, make vet, make test, frontend typecheck/build, Compose configuration validation, Docker build/smoke, and scripts/verify.sh before review.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Review corrected the initial internal-network assumption: container membership and absent host ports preserve inbound privacy, while egress is required for a cold Headroom model download. The probe will also surface compression_skipped as fail-open.

Implemented the Compose/Coolify remote-access opt-in on the sidecar only; preserved no host port and a dedicated router/Headroom network. Parsed v0.36 flat metrics and made skipped 2xx responses fail open. Validation: full ./scripts/verify.sh passed after final refactors, including backend vet/tests, frontend typecheck/build, Compose config checks, image build, and fresh isolated dashboard probe. Independent tester repeated the live Compose probe. CodeScene pre-commit quality gate passed; branch-wide CodeScene analysis was not run because it requires explicit authorization to export the private diff.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed Compose and Coolify Headroom 404s by allowing remote compression only inside the sidecar's dedicated network. KeiRouter now parses Headroom v0.36 flat metrics and rejects skipped compression as fail-open. Verified by full repository verification and an independent isolated Compose dashboard probe.
<!-- SECTION:FINAL_SUMMARY:END -->
