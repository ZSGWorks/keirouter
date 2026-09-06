---
id: TASK-014.03
title: Merge multimodal catalog model records
status: Done
assignee:
  - '@opencode'
created_date: '2026-09-06 08:18'
updated_date: '2026-09-06 08:38'
labels: []
dependencies: []
references:
  - backend/internal/pricing/modelsdev.go
  - backend/internal/connectors/models.go
  - backend/internal/gateway/admin.go
  - backend/internal/gateway/models.go
  - '@keirouter-opencode-plugin/src/index.ts'
parent_task_id: TASK-014
priority: high
type: bug
ordinal: 17000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Models.dev projects one multimodal upstream model into separate catalog records per service kind. Provider pages display duplicate cards and OpenCode receives repeated model choices. Preserve all supported modalities while exposing each provider/model identity once.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Each provider model ID appears once in provider catalog responses regardless of supported modalities
- [x] #2 Multimodal models retain LLM and non-LLM service capabilities
- [x] #3 LLM and per-service model discovery return each targetable model once
- [x] #4 OpenCode model discovery receives no duplicate provider/model identities
- [x] #5 Regression tests cover a multimodal models.dev model and backend validation passes
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Extend the catalog model representation to carry every supported service kind while retaining one primary kind for existing dispatch metadata; normalize model collections by provider-local ID, prefer LLM as primary when present, and union capabilities/kinds rather than duplicating records. Preserve custom-model override precedence.
2. Change models.dev projection so one multimodal model emits one merged record instead of one record per modality. Update snapshot, merge, lookup, and probe helpers to use membership in the merged kind set.
3. Update gateway catalog and live-model append paths to test service-kind membership, emit one OpenAI `/v1/models` entry per provider/model identity, and expose merged kinds in provider/admin and model metadata responses without breaking the existing primary `kind` field.
4. Update dashboard types and model detail/card rendering to show merged modalities once; retain media route filtering through the API kind filter.
5. Add focused pricing/connectors/gateway/frontend tests for LLM-plus-vision models, custom override behavior, one `/v1/models` entry, and per-kind discovery. Run gofmt, targeted and full backend tests, vet, frontend typecheck, Code Health, and final diff review.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Canonical ModelSpec records now merge duplicate provider-local IDs, preserve all supported service kinds, and prefer LLM as primary. Custom entries retain metadata override precedence while kinds are unioned.

Gateway catalog, live discovery, admin responses, and model info emit one identity with `kinds`. Unfiltered LLM listings merge capability profiles so multimodal inputs/outputs remain advertised; per-service listings retain requested `kind`.

Added canonical models.dev image input/output projection, catalog merge, provider-admin response, and OpenAI discovery tests. Added canonical `image_to_text` service capability spelling while retaining legacy `imageToText`.

Verification: targeted and full backend tests, go vet ./backend/..., frontend typecheck/build, ./scripts/verify.sh, Code Health safeguard, diff check, and final reviewer all passed.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Merged multimodal catalog entries into a single canonical provider/model record. ModelSpec now carries all supported service kinds while retaining an LLM primary kind where available; merges union kinds and custom metadata overrides remain intact. Models.dev projection no longer creates separate LLM, image-to-text, and image records. Gateway provider, OpenAI-compatible, live discovery, and model-info responses now emit one identity with `kinds`; default model metadata merges all modality capabilities while per-service listings report their requested kind. Dashboard cards and details show every service-kind badge once. Added the canonical `image_to_text` capability mapping alongside legacy spelling. Regression coverage proves catalog deduplication, input/output modality preservation, LLM/image/image-to-text discovery, API metadata, and custom merging. Verified gofmt, full backend tests/vet, frontend typecheck/build, complete ./scripts/verify.sh, Code Health, and final diff review.
<!-- SECTION:FINAL_SUMMARY:END -->
