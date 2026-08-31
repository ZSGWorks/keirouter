---
id: TASK-002
title: Rework GitHub Actions build validation
status: Done
assignee:
  - OpenCode
created_date: '2026-08-31 18:50'
updated_date: '2026-08-31 19:06'
labels:
  - ci
  - github-actions
dependencies: []
references:
  - .github/workflows/ci.yml
  - .github/workflows/docker-publish.yml
  - .github/workflows/release.yml
  - >-
    /Users/zsolt.gajdacs/Zs/Programming/StoryEngine/.github/workflows/pr-validation.yml
  - /Users/zsolt.gajdacs/Zs/Programming/kiffel-bageri/.github/workflows/ci.yml
priority: medium
type: chore
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Align KeiRouter's GitHub Actions with repository CI conventions while retaining only build and validation checks. Docker publishing and release automation are no longer required.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 GitHub Actions retains automated build and validation coverage for supported project components
- [x] #2 Docker publishing and release automation are removed
- [x] #3 Workflow behavior is validated for pull requests and main branch changes
- [x] #4 Workflow configuration uses least-privilege permissions and cancels superseded pull request runs
- [x] #5 Monthly GitHub quota reset validation is deterministic across calendar dates
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Retain `.github/workflows/ci.yml` as the sole automation workflow and delete Docker publishing and release workflows.
2. Convert CI to StoryEngine-style pull request validation: least-privilege permissions, per-ref concurrency cancellation, self-hosted Linux/x64 runners, timeouts, and a changed-path detection job.
3. Gate backend plus PostgreSQL, frontend, and deployment validations by their respective inputs; make workflow edits trigger every relevant gate.
4. Preserve current validation commands and runtime compatibility: Go 1.26, frontend typecheck/build, Compose validation, Docker image build.
5. Validate workflow YAML and run local equivalents for retained gates. Confirm self-hosted runners are isolated because pull request code is untrusted.
6. Extract `httpStatusErrorAt` with a supplied timestamp while retaining `httpStatusError` as a production `time.Now()` wrapper; test the GitHub monthly quota path with an exact fixed UTC retry interval.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented self-hosted, changed-path-gated CI; removed publish/release workflows. Local validation: YAML parses, Compose configs and Docker build pass; frontend checks pass after locked install; `make vet` passes with dependency warnings. `make test` fails in existing `TestHTTPStatusError_GitHubMonthlyUsageIsResettableQuota`: expected retry-after >24h but observed about 5h. `actionlint` is not installed locally. Review found no CI coverage gaps; noted expected security risk of self-hosted runners executing pull-request code, already called out in approved plan.

Added required `pull-requests: read` permission for `dorny/paths-filter` on pull request events. Restored the monthly GitHub quota wrapper test with positive, at-most-one-month retry bounds; direct helper test retains exact fixed-time calculation coverage. Final verification passed: `actionlint`, `git diff --check`, `make vet`, `make test`, frontend `npm ci && npm run typecheck && npm run build`, all Compose configs, and Docker image build.

Replaced calendar-safe bounds with deterministic time injection: `httpStatusError` delegates to `httpStatusErrorAt(..., time.Now())`; monthly quota test passes an August 4 UTC fixture and asserts the exact 27d4h30m reset interval. `make vet`, `make test`, `git diff --check`, and final review passed.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Reworked GitHub Actions into one self-hosted Linux/x64 build-validation workflow with changed-area gating, least-privilege permissions, PR read access for path filtering, concurrency cancellation, and bounded job timeouts. Removed Docker image publishing and release workflows. Retained backend, PostgreSQL, frontend, Compose, and Docker image validation commands.

Made GitHub monthly quota status-error coverage deterministic: `httpStatusError` preserves production behavior by delegating with `time.Now()`, while `httpStatusErrorAt` accepts an explicit timestamp for exact fixed-date assertions.

Verified with actionlint, git diff --check, make vet, make test, frontend npm ci/typecheck/build, all Compose config validations, Docker image build, and final review. Self-hosted pull-request execution remains an explicit selected risk: runners must be isolated and have no production credentials or network access.
<!-- SECTION:FINAL_SUMMARY:END -->
