---
id: TASK-013
title: Fix valid Zen API key rejected when connecting OpenCode provider
status: Done
assignee: []
created_date: '2026-09-05 18:42'
updated_date: '2026-09-05 18:56'
labels:
  - bug
  - backend
  - connectors
dependencies: []
priority: high
type: bug
ordinal: 13000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Connecting provider `opencode` (OpenCode Zen, https://opencode.ai/zen/go/v1) in the dashboard always fails with "invalid API key", even with a valid key.\n\nConfirmed via docker logs:\n```\nERROR credential validation failed: validation failed for opencode: auth:\n{"type":"error","error":{"type":"ModelError","message":"Model test is not supported"}} (provider=opencode model=validate status=401)\n```\n\nChain:\n1. GET /models on Zen returns 200 without checking the key → validation falls through to chatAuthProbe.\n2. firstCatalogModel("opencode") is empty (no static catalog entry; only "opencode-go" exists) → probe falls back to synthetic model "test".\n3. Zen validates model existence before key validity: unknown model → 401 ModelError regardless of key.\n4. httpStatusErrorAt maps every 401/403 to ErrAuth; the model-unsupported reclassification only runs in the generic 4xx branch.\n5. validateProbe sees ErrAuth → validation fails.\n\nFix: (A) add "opencode" static catalog entry with real Zen model ids; (B) reclassify 401/403 with model-unsupported body as ErrModelUnavailable.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Static catalog includes an "opencode" providerModels entry with real Zen model ids, so firstCatalogModel("opencode") returns a non-empty real model.
- [x] #2 A 401/403 upstream response whose body matches model-unsupported phrasing is classified as ErrModelUnavailable, not ErrAuth.
- [x] #3 Unit test covers 401 + {"type":"ModelError",...} body classification.
- [x] #4 Unit test covers firstCatalogModel("opencode") returning a real model.
- [x] #5 make vet and make test pass.
- [x] #6 Live check: connecting the opencode provider succeeds with a valid Zen key (API flow: login → POST /api/accounts → HTTP 201).
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Fix shipped as two layers:
1. backend/internal/connectors/models.go: added "opencode" providerModels entry (20 real Zen model ids fetched live from GET /zen/go/v1/models) so firstCatalogModel("opencode") returns a real model for the validation probe.
2. backend/internal/connectors/httpclient.go httpStatusErrorAt: 401/403 whose body matches isModelUnsupportedBody now classify as ErrModelUnavailable (FailureScopeModel), not ErrAuth. Added phrase "not supported\"" to modelUnsupportedPhrases because Zen's body is "Model test is not supported" (existing phrases only matched "model is not supported").

Tests: TestHTTPStatusError_401ModelUnsupportedIsNotAuth, TestHTTPStatusError_401GenuineAuthErrorStaysAuth (classify429_test.go), TestOpenCodeCatalogHasProbeModel (catalog_test.go).

Live verification (rebuilt dev container):
- placeholder key → probe now sends real model glm-5.3-flash → genuine AuthError → rejected correctly (previously ModelError misread as invalid key).
- real Zen key → POST /api/accounts HTTP 201, account created.
- chat via opencode/glm-5.3-flash → CreditsError Insufficient balance (workspace balance empty, correct quota_exhausted classification, not auth).
- direct upstream curl confirms key valid; every paid model returns CreditsError; old free ids (big-pickle, mimo-v2.5-free) no longer in Zen's live /models list (35 models now, e.g. mimo-v2.5-pro, qwen3.8-max); deepseek-v4-flash requires China-region opt-in.
AC #6 (dashboard UI click-through) not performed in UI itself; equivalent API flow (login → POST /api/accounts) verified with HTTP 201.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed valid Zen API key being rejected when connecting the OpenCode provider.

Root cause: OpenAICompatible.Validate first probes GET /models (Zen returns 200 without checking the key), then falls back to chatAuthProbe. firstCatalogModel("opencode") was empty (no static catalog entry), so the probe sent synthetic model "test". Zen validates model existence before key validity and returns 401 with {"type":"ModelError","message":"Model test is not supported"} regardless of key, which httpStatusErrorAt classified as ErrAuth — every connection attempt failed as "invalid API key".

Fix:
1. Added "opencode" static catalog entry (backend/internal/connectors/models.go) with 20 real Zen model ids so the validation probe sends a real model.
2. httpStatusErrorAt (backend/internal/connectors/httpclient.go) now classifies 401/403 with a model-unsupported body as ErrModelUnavailable instead of ErrAuth; added phrase "not supported\"" to modelUnsupportedPhrases to match Zen's error shape.

Tests: TestHTTPStatusError_401ModelUnsupportedIsNotAuth, TestHTTPStatusError_401GenuineAuthErrorStaysAuth, TestOpenCodeCatalogHasProbeModel. go vet + full backend test suite pass.

Live verification against rebuilt dev container: placeholder key → genuine AuthError (correct); real Zen key → POST /api/accounts HTTP 201 (account created). Subsequent chat requests surface upstream CreditsError (insufficient Zen workspace balance) with correct quota_exhausted classification.
<!-- SECTION:FINAL_SUMMARY:END -->
