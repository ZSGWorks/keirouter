---
id: TASK-008
title: Fix Ollama model discovery
status: Done
assignee:
  - Zsolt Gajdács
created_date: '2026-09-03 19:12'
updated_date: '2026-09-04 03:52'
labels: []
dependencies: []
references:
  - 'https://docs.ollama.com/api/tags'
  - 'https://docs.ollama.com/api'
  - 'https://models.dev/providers/ollama-cloud'
  - 'https://models.dev/api.json'
modified_files:
  - backend/internal/connectors/ollama.go
  - backend/internal/connectors/registry.go
  - backend/internal/connectors/connectors_test.go
  - backend/internal/gateway/models.go
  - backend/internal/gateway/models_test.go
priority: high
type: bug
ordinal: 8000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Ollama provider discovery currently exposes only a small static model list instead of the models available from the configured Ollama deployment. Make provider model discovery reflect the connected local or cloud Ollama catalog, while retaining catalog metadata as a fallback when upstream discovery is unavailable.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Configured local Ollama accounts expose all models available in that Ollama instance.
- [x] #2 Configured Ollama Cloud accounts expose all models available through the supported Ollama model catalog endpoint.
- [x] #3 Ollama authentication headers and custom base URLs continue to work for discovery.
- [x] #4 Static and models.dev catalog entries remain available when live Ollama discovery fails or returns no models.
- [x] #5 Duplicate model IDs are removed without losing existing model names or service kind information.
- [x] #6 Automated tests cover local responses, cloud responses, authentication, URL construction, malformed or empty responses, and fallback behavior.
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Add Ollama LiveModelSource using documented GET /api/tags, account base URL, bearer headers, and name/model parsing.
2. Register source for ollama and ollama-local.
3. Generalize gateway fetchLiveModels to enumerate all registered live sources while preserving account and timeout behavior.
4. Preserve static, custom, and models.dev additive merge/fallback behavior.
5. Add connector and gateway tests for URLs, auth, payloads, errors, duplicates, and fallback.
6. Run gofmt, focused tests, make vet, and make test.

7. Remediate CodeScene gate: extract chain, static-catalog, and live-catalog model-entry assembly from handleListModels to remove nested conditional chunks and reduce gateway/models.go mean complexity without changing discovery behavior.

8. Collapse duplicated live-success/fallback gateway tests into table-driven cases, then rerun CodeScene safeguard plus gofmt, focused tests, make vet, make test, and git diff --check.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented Ollama native live discovery through GET /api/tags for both ollama and ollama-local, including bearer auth, custom account base URLs, identifier fallback, malformed/error handling, and deduplication.

Registered Ollama live sources and generalized gateway live discovery beyond Kiro. Static catalog and models.dev projections remain additive fallbacks; tests cover live success, empty/error responses, global discovery, and static fallback.

Verification passed: gofmt, make vet, make test, focused connectors/gateway/pricing/app tests, and git diff --check. CodeScene pre-commit safeguard still reports gateway/models.go module mean complexity 5.625 above threshold 5 (file score 8.74); fetchLiveModels function complexity was reduced and no functional test failures remain.

CodeHealth remediation completed. Refactored model-list assembly into chain, static-catalog, and live-catalog helpers; collapsed duplicated discovery tests; CodeScene pre-commit safeguard now reports quality_gates: passed and gateway/models.go score is 9.38.

Review-driven hardening added: live models are present in /v1/models/llm and excluded from mismatched kinds; discovery probes are limited to four concurrent requests with one request-wide 10-second deadline; cancellation returns even if an upstream source ignores context. Tests cover kind filtering, concurrency cap, and cancellation safety.

Final verification passed: gofmt, focused gateway/connectors tests, make vet, make test, git diff --check, CodeScene safeguard, and final diff review with no findings.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Implemented native Ollama catalog discovery for local and cloud accounts through GET /api/tags. Discovery honors account custom base URLs and authorization headers, normalizes name/model identifiers, skips malformed/empty entries, and deduplicates IDs. Registered Ollama live sources and generalized gateway discovery across registered providers while retaining static and models.dev catalog entries as fallbacks.

Hardened model-list discovery: live models now appear in the global and kind-specific LLM endpoints, kind mismatches are excluded, live probes run with a four-request concurrency cap and one request-wide 10-second deadline, and cancellation safely returns when an upstream source ignores context.

Refactored gateway model assembly to clear CodeScene complexity and duplication findings. Added connector and gateway tests for URL/auth handling, parsing, errors, fallback behavior, kind filtering, probe concurrency, and cancellation. Verified with gofmt, focused tests, make vet, make test, git diff --check, CodeScene pre-commit safeguard (passed), and final review (no findings).
<!-- SECTION:FINAL_SUMMARY:END -->
