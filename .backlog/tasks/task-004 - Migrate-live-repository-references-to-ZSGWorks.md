---
id: TASK-004
title: Migrate live repository references to ZSGWorks
status: Done
assignee:
  - OpenCode
created_date: '2026-08-31 19:28'
updated_date: '2026-08-31 19:44'
labels:
  - ownership
  - repository-migration
dependencies: []
references:
  - README.md
  - compose.yaml
  - backend/internal/update/update.go
  - scripts/install.sh
  - scripts/install.ps1
  - scripts/quickstart.sh
  - skills/
  - Formula/keirouter.rb
  - bin/keirouter
priority: medium
type: chore
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Make operational URLs, install defaults, container image defaults, skills, and update checks use ZSGWorks/keirouter. Remove obsolete Homebrew formula and tracked binary because this repository does not publish releases. Preserve historical and legal attribution plus the existing Go module path and upstream remote.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Live repository URLs and container image defaults use ZSGWorks/keirouter
- [x] #2 Application update checks target ZSGWorks/keirouter
- [x] #3 Obsolete Homebrew formula tracked binary and release-only Windows installer are removed
- [x] #4 Historical changelog entries legal attribution Go module identity and upstream remote remain unchanged
- [x] #5 Relevant application and documentation validation passes
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Replace live ZSGWorks ownership URLs in documentation, skills, installers, frontend skill fetches, Compose image default, changelog-link generation, and update checker defaults/tests.
2. Remove the obsolete README CI badge because GitHub CI is being removed, plus the Homebrew formula and tracked binary because this repository has no releases.
3. Preserve Go module import path, upstream remote, LICENSE copyright, and historical CHANGELOG links as agreed.
4. Search tracked text after replacement, run targeted Go update tests and frontend checks, and verify removed artifacts are no longer tracked.

5. Remove the release-only PowerShell installer and Windows prebuilt-binary documentation because ZSGWorks has no release assets; do not retain an upstream runtime dependency.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Migrated live operational ownership to ZSGWorks across documentation, installers, Compose image default, skills, dashboard skill fetches, changelog issue links, application update checks, and tray documentation link. Deleted the obsolete Homebrew formula, tracked binary, release-only PowerShell installer, and unsupported Windows prebuilt documentation because ZSGWorks has no releases. Deliberately retained Go module/import paths, upstream remote, LICENSE attribution, and historical changelog links for mechanical upstream merges. Verified `go test ./internal/update`, frontend typecheck/build, `make vet`, `make test`, and `./scripts/verify.sh`; final review and Code Health gate passed.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Migrated all live operational KeiRouter ownership endpoints to `ZSGWorks/keirouter`: documentation, installers, GHCR default, skill distribution, dashboard fetches, update checker, tray link, and generated changelog issue links. Removed obsolete release-only artifacts and documentation: Homebrew formula, tracked binary, PowerShell installer, and Windows prebuilt path.

Preserved the Go module/import path, repository layout, upstream remote, LICENSE attribution, and historical changelog links to keep future upstream merges mechanical. Added durable `AGENTS.md` fork-maintenance guidance. Verified targeted update/frontend checks, full backend checks, local verifier, final review, and Code Health gate.
<!-- SECTION:FINAL_SUMMARY:END -->
