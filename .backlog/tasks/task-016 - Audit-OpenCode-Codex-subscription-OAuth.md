---
id: TASK-016
title: Audit OpenCode Codex subscription OAuth
status: Done
assignee: []
created_date: '2026-09-06 16:04'
updated_date: '2026-09-06 16:07'
labels: []
dependencies: []
references:
  - 'https://github.com/anomalyco/opencode/tree/dev'
priority: medium
type: spike
ordinal: 19000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
KeiRouter's Codex subscription connection should be compared against OpenCode's current implementation so compatibility, security, and product differences are understood before any protocol changes are considered.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Compare browser OAuth parameters and callback handling
- [x] #2 Compare token persistence and refresh behavior
- [x] #3 Compare Codex inference endpoint and identity headers
- [x] #4 Document verified differences and ranked follow-up recommendations
- [x] #5 Do not inspect credentials or run live authorization
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Freeze upstream evidence at OpenCode dev commit 337fd144d2ba144743368f78d9579a99cce175bd and current KeiRouter HEAD 864ada6. 2. Compare browser auth, callback, storage/refresh, and inference request behavior from source only. 3. Produce a versioned comparison report with risks and minimal, evidence-backed follow-up recommendations. 4. Do not read credential files or perform live OAuth.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Source-only audit completed without opening local auth files or running a live OAuth flow. OpenCode evidence frozen at 337fd144d2ba144743368f78d9579a99cce175bd; report added at docs/research/opencode-codex-oauth-audit.md. Verified relevant Go packages with `go test ./internal/oauth ./internal/connectors ./internal/gateway`; `git diff --check` passed. Code Health found no applicable source changes. IDE Markdown inspection reports weak table-format warnings on valid GFM tables; no rendered-content defect identified.

Correction: `docs/` is ignored by this repository. Moved audit artifact to tracked root path `OPENCODE-CODEX-OAUTH-AUDIT.md`.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Added tracked `OPENCODE-CODEX-OAUTH-AUDIT.md`, a versioned source-only audit comparing OpenCode Codex subscription OAuth with KeiRouter. The report records aligned browser OAuth parameters, deliberate gateway security/reliability differences, request-header behavior, and three gated follow-ups: account-ID claim fallback policy, compute-residency forwarding, and headless device authentication. No production OAuth behavior changed because source evidence alone cannot validate undocumented OpenAI behavior. Verified with `go test ./internal/oauth ./internal/connectors ./internal/gateway`, `git diff --check`, and Code Health (no applicable source changes).
<!-- SECTION:FINAL_SUMMARY:END -->
