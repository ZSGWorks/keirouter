---
id: TASK-003
title: Replace CI workflow with local verification
status: Done
assignee:
  - OpenCode
created_date: '2026-08-31 19:20'
updated_date: '2026-08-31 19:44'
labels:
  - ci
  - developer-experience
dependencies: []
references:
  - .github/workflows/ci.yml
  - AGENTS.md
  - scripts/hooks/pre-push
  - Makefile
priority: medium
type: chore
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Remove GitHub Actions build validation because the available self-hosted runner cannot run public repositories. Give local agents one documented command that performs equivalent validation before handoff.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 No GitHub Actions build validation workflow remains
- [x] #2 Agents have documented local verification guidance
- [x] #3 One local command performs the former CI validation checks
- [x] #4 The local verification command reports failures clearly and exits nonzero
- [x] #5 The local verification command is itself validated
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Delete the unsupported GitHub Actions validation workflow.
2. Add an executable local verification script that installs locked dependencies, runs backend and frontend checks, starts a disposable PostgreSQL 16 Docker container for compatibility tests, validates Compose files, and builds the Docker image.
3. Use strict shell error handling, dependency checks, health polling, dynamic host-port discovery, and cleanup traps so failure messages and container cleanup are reliable.
4. Update AGENTS.md to make the local script the pre-handoff validation command, retain targeted component guidance, and document separate plugin validation.
5. Validate shell syntax and run the complete script locally.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Deleted GitHub Actions CI and added executable `scripts/verify.sh`. The verifier installs locked dependencies, starts and cleans up a temporary PostgreSQL 16 container on a dynamic localhost port, runs backend/frontend/Compose/Docker checks, and fails fast with prerequisite and command errors. `AGENTS.md` now requires it before cross-cutting handoff; plugin validation remains separate. Verified with `bash -n scripts/verify.sh` and a full successful `./scripts/verify.sh` run.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Replaced unsupported GitHub Actions validation with `scripts/verify.sh`, a self-contained local gate matching former CI coverage: Go dependency download, vet/tests, frontend install/typecheck/build, PostgreSQL compatibility, Compose validation, and Docker image build. It checks prerequisites, manages a disposable health-checked PostgreSQL container, and guarantees cleanup through a trap.

Updated agent guidance to require the verifier before cross-cutting handoff and retain separate plugin checks. Verified shell syntax and executed the full script successfully.
<!-- SECTION:FINAL_SUMMARY:END -->
