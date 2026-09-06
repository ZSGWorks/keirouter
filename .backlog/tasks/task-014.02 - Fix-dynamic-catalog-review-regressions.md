---
id: TASK-014.02
title: Fix dynamic catalog review regressions
status: Done
assignee:
  - opencode
created_date: '2026-09-06 07:18'
updated_date: '2026-09-06 07:33'
labels: []
dependencies: []
parent_task_id: TASK-014
priority: high
type: bug
ordinal: 16000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Dynamic LLM catalog migration introduced three regressions: accounts created after the startup refresh lack fallback models, Jina Reader disappeared from fetch discovery, and model-gated providers can reject valid credentials when validation invents a model name. Restore immediate model availability and credential validation correctness without reintroducing a hardcoded LLM catalog.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Newly connected providers expose models.dev LLMs even when connected after application startup
- [x] #2 Jina Reader remains returned by fetch model discovery
- [x] #3 Credential validation never sends a synthetic model ID when no discovered model is available
- [x] #4 Targeted regression tests cover all three cases
- [x] #5 Backend formatting vet and tests pass
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Keep public models.dev model metadata unfiltered in pricingCatalogRefresher, while retaining one clearly named connected-price projection through filterFetchedPrices. Delete obsolete model filtering/combined provider helpers and revise unit fixtures to make model publication and price projection independently explicit. 2. Restore Jina Reader static fetch model and protect through catalog kind coverage. 3. In OpenAI-compatible validation, treat no ProbeModelFor result as insufficient data for a model-bound probe: do not call chat endpoint or fabricate test; preserve chat probe with discovered real ID. Narrow tests prove zero chat calls when catalog absent and exact discovered ID when present. 4. Simplify `models.go` refactor by removing the boolean exclude-mode filtering helper; leave two direct, readable kind filters rather than an opaque bool. 5. Validate format, targeted/full backend tests, vet and Code Health.

Review refinement: apply the empty-ProbeModelFor early return to Anthropic messagesAuthProbe too. This is within acceptance criterion #3 (never send a model-bound probe without discovery) and removes its canonical fallback. Add equivalent request-count regression coverage. Do not unhide Jina Reader: its hidden provider policy predates this diff; direct FindModel coverage verifies restored targetability without changing dashboard visibility.

Final-review refinement: an empty cached ProbeModelFor must first attempt the connector's existing live model source and use its first returned real ID for the authentication probe. Only if that live lookup fails or returns no models does validation skip the model-bound request. This preserves public-/models bad-key rejection without fabricating model IDs. Add tests for public model list with empty cache. The `/v1/models/info` live-discovery lookup gap is outside this task's acceptance criteria and will be presented for scope decision.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Pricing refresh now preserves full public models.dev metadata while filtering prices by connected account. Gateway remains the tenant authorization boundary through usableModelProviders.

ProbeModelForCredentials first uses cached discovery, then existing live model source. Both OpenAI-compatible and Anthropic validation never fabricate a model; no live ID means no model-bound probe.

Jina Reader remains hidden by pre-existing catalog policy; FindModel verifies its restored ServiceFetch targetability without changing dashboard visibility.

Verification: gofmt clean; targeted app/connectors/gateway tests passed; go vet ./backend/... passed; go test ./backend/... passed; Code Health safeguard passed; final diff reviewer reported no issues.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed dynamic catalog review regressions without restoring a hardcoded LLM fallback. Pricing refresh now retains full public models.dev model metadata, while connected-account filtering remains limited to prices and gateway listing enforces tenant-visible providers. Restored the Jina Reader ServiceFetch target model. Validation now resolves a real probe model from cached discovery or the existing live model source; OpenAI-compatible and Anthropic connectors skip model-bound probes only when neither source provides a real ID, preventing both synthetic-model rejections and public-/models bad-key false positives. Simplified kind filtering by removing the boolean exclude-mode helper. Added regression coverage for unconnected model snapshot publication, Jina targetability, empty/discovered OpenAI probes, public model lists, and empty Anthropic discovery. Verified full backend tests, vet, formatting, Code Health safeguard, and final diff review.
<!-- SECTION:FINAL_SUMMARY:END -->
