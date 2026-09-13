---
id: TASK-026
title: Exhausted GLM quota fails over to the next chain target without delay
status: Done
assignee: []
created_date: '2026-09-13 10:14'
updated_date: '2026-09-13 10:18'
labels: []
dependencies: []
priority: high
type: bug
ordinal: 29000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
GLM returns HTTP 429 with code 1310 and a weekly/monthly limit-exhausted message plus a reset timestamp, but KeiRouter classifies it as a transient per-minute rate limit because the body contains rate_limit text. The chain therefore waits and retries the dead quota for ~10s instead of failing over immediately, and keeps hammering an exhausted upstream quota every few seconds.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A 429 response carrying GLM code 1310 with weekly/monthly limit-exhausted wording is treated as quota exhaustion with an account-scoped cooldown
- [x] #2 The cooldown honors the reset timestamp carried in the provider message when present and valid, otherwise a conservative default applies
- [x] #3 A code 1310 body without limit-exhausted wording keeps the current transient rate-limit behavior
- [x] #4 Limit-exhausted wording without code 1310 keeps the current classification behavior
- [x] #5 Genuine depleted-balance signals still take precedence over the quota classification
- [x] #6 Unit tests cover the 1310 cases including both negative cases; the backend test suite passes
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
GLM 1310 reclassification (approved plan reclassify-glm-1310-limit-exha-2026-09-13): 1) Add dual-anchor 1310 branch in classify429 after credits check (code 1310 + calendar limit-exhausted wording → ErrQuotaExhausted). 2) Parse `reset at YYYY-MM-DD HH:MM:SS` (UTC) from message into RetryAfter, fallback 30m. 3) No pipeline/dispatch changes (quota already fallbackable/account-scoped, bypasses wait loops). 4) Tests in classify429_test.go incl. negatives. 5) Verify gofmt/vet/focused tests/full make test.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented dual-anchor 1310 branch in backend/internal/connectors/classify429.go (glmQuotaCode + quotaPatterns wording) with glmQuotaCooldown helper and parseGLMQuotaReset (UTC, 30m default, 31d cap). Refactored twice for CodeHealth: extracted helper (fixed Complex Method/Bumpy Road), then merged to single []byte entry (fixed String Heavy Args); safeguard now passes.

Verified: gofmt clean, go vet clean, go test ./internal/connectors green (incl. 8 new tests), full make test green with no FAIL, git diff --check clean.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Reclassify GLM code 1310 calendar quota exhaustion as quota_exhausted. Added a dual-anchored branch in classify429 (numeric 1310 code plus calendar limit-exhausted wording, credits check still first) plus message reset-timestamp parsing with 30m fallback and 31d sanity cap, consolidated into one []byte helper after a CodeHealth complexity finding. No pipeline/dispatch changes: quota kind is already fallbackable and account-scoped and bypasses both stream wait loops, so chains now fail over immediately instead of stalling ~10s. Tests: 8 new cases in classify429_test.go covering quota/reset/no-reset/bad-reset, both negative anchors, credits precedence, and end-to-end kind/scope/fallbackable. Verified with gofmt, go vet, focused package tests, full make test, git diff --check, and CodeHealth pre-commit safeguard (passed). Follow-up TASK-026.01 (cooldown-aware probes) remains open; the stale Ollama 24h parking row needs expiry or manual DB clear, unaffected by this change.
<!-- SECTION:FINAL_SUMMARY:END -->
