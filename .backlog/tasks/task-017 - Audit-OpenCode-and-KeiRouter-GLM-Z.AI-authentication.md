---
id: TASK-017
title: Audit OpenCode and KeiRouter GLM Z.AI authentication
status: Done
assignee:
  - OpenCode
created_date: '2026-09-06 16:21'
updated_date: '2026-09-06 16:24'
labels: []
dependencies: []
references:
  - >-
    https://github.com/anomalyco/opencode/tree/337fd144d2ba144743368f78d9579a99cce175bd
  - 'https://open.bigmodel.cn/'
priority: medium
type: spike
ordinal: 20000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
KeiRouter currently connects GLM Coding with an API key, while requested comparison may reveal a supported subscription-login flow or confirm that no compatible OAuth integration exists. A durable evidence-based audit prevents implementing undocumented authentication behavior or incorrectly treating generic OAuth code as Z.AI support.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Compare current OpenCode and KeiRouter Z.AI GLM authentication implementations from source
- [x] #2 Identify publicly documented Z.AI OAuth or device-login support if present
- [x] #3 Publish complete findings as a Backlog research document
- [x] #4 Do not inspect local credentials or execute a live authorization flow
- [x] #5 Record whether production changes are justified and gated follow-ups if not
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are validated by tests
- [x] #3 Always look for refactorings in the touched areas and make the code more maintainable as you go
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Freeze evidence at current KeiRouter HEAD and OpenCode commit 337fd144d2ba144743368f78d9579a99cce175bd. 2. Compare provider catalog/auth registrations, generic OAuth facilities, vault storage, and existing tests without opening credentials. 3. Check official Z.AI public documentation for OAuth, device authorization, or other non-API-key login evidence; distinguish documented facts from absent evidence. 4. Publish the complete source-only gap audit as a Backlog research document, not a repository Markdown file. 5. Validate Backlog artifact and run git diff check; make no production OAuth change unless official support and product need are established.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Completed source-only comparison. OpenCode tree at 337fd144d2ba144743368f78d9579a99cce175bd has no GLM/Z.AI OAuth implementation; official Z.AI OpenCode and Coding Plan guides require API-key entry. Published Backlog document doc-002. Verified document retrieval, `git diff --check`, and Code Health safeguard (passed; no applicable source files). Focused evidence review found no issues. No local credentials were opened and no live authorization was performed.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Published Backlog research document doc-002, `OpenCode and KeiRouter GLM Z.AI Authentication Audit`. Evidence confirms KeiRouter and OpenCode both use API-key onboarding for Z.AI/GLM; neither has a provider-specific OAuth login flow at the audited OpenCode revision. Official Z.AI documentation also requires API keys for OpenCode and GLM Coding Plan. No production OAuth changes were made: adding undocumented endpoints, client identities, or token behavior would be unsupported. Verified document retrieval, `git diff --check`, and Code Health safeguard (passed with no applicable source changes); focused audit review found no issues. Future OAuth work requires official Z.AI documentation or authorization plus disposable-account validation.
<!-- SECTION:FINAL_SUMMARY:END -->
