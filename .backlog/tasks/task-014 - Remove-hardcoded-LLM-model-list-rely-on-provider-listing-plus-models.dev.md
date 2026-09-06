---
id: TASK-014
title: Remove hardcoded LLM model list; rely on provider listing plus models.dev
status: Done
assignee:
  - opencode
created_date: '2026-09-06 05:36'
updated_date: '2026-09-06 05:47'
labels: []
dependencies: []
priority: medium
type: feature
ordinal: 14000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Hardcoded LLM model IDs rot on every upstream release and mask the live provider listing plus the models.dev fallback that already exist. Removing LLM-kind entries while keeping non-LLM hardcoded entries makes discovery dynamic where it can be and stable where no listing endpoint exists.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 LLM model listings come from live provider GET /models, cached models.dev snapshot, and custom/dynamic models; no hardcoded LLM entries remain
- [x] #2 Non-LLM listings (embedding, TTS, STT, image, video, imageToText, search, fetch) keep existing hardcoded behavior unchanged
- [x] #3 Validate chat probe resolves its model dynamically with skip fallback; no firstCatalogModel static dependency
- [x] #4 codexModelKnown no longer reads the static map
- [x] #5 Connectors and gateway test suites pass; LLM test fixtures seed fetched catalog instead of asserting static entries
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Refactorings (behavior-preserving): split non-LLM static map to models_static.go; add LLMModelsForProvider/StaticNonLLM accessors; split merge policy; extract ProbeModelFor to validate_probe.go; decouple codexModelKnown; add registry live-source helper; add SeedLLMCatalog test seam. 2. Behavior change: delete m() LLM entries; ModelsByKind(LLM)/FindModel read fetched+dynamic only. 3. Validation probe + codex + tests + docs. Verify with make vet/test connectors+gateway after each step.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Split providerModels (71 providers) into models_static.go providerStaticModels (35 providers, all k()/emb() calls preserved verbatim, verified by script) and deleted all m() LLM entries plus the m() helper.

Added LLMModelsForProvider / StaticNonLLMModelsForProvider / mergeLLMModels / mergeNonLLMModels; ModelsForProvider composes both. ModelsByKind/FindModel unchanged in shape.

Extracted ProbeModelFor (validate_probe.go); OpenAICompatible + Anthropic Validate use it; deleted firstCatalogModel. codexModelKnown reads LLMModelsForProvider(codex).

Registry: consolidated live-source registration into registerLiveModelSource helper; added Anthropic-dialect live sources; added ListProvidersWithoutLiveSource coverage reporter.

Test seam SeedLLMCatalog + llmSpec fixture; rewrote TestFindModel/TestModelsByKind/TestCommandCodeCatalogVisible/TestOpenCodeProbeModelResolvesDynamically/TestCatalogHasNewProviders/kiro price coverage/ThreeProviders/antigravity catalog test; gateway models_test + admin_provider_test seed fetched snapshots.

Verification: go vet ./backend/... clean; go test -count=1 connectors + gateway green; full go test ./backend/... green; codehealth pre-commit safeguard quality_gates=passed. Reverted incidental gofmt churn in 6 unrelated files.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Removed the hardcoded LLM model list (TASK-014). backend/internal/connectors/models.go lost the 71-provider providerModels map and m() helper; new models_static.go keeps providerStaticModels with all 35 non-LLM provider entries verbatim (every k()/emb() call preserved, verified by script). LLM discovery is now live GET /models via LiveModelSource, then the models.dev snapshot (fetchedModels), then custom/dynamic models; empty is valid. Added kind-aware accessors (LLMModelsForProvider, StaticNonLLMModelsForProvider), split merge policy, ProbeModelFor replacing firstCatalogModel in both Validate paths, codexModelKnown reading dynamic Codex models, consolidated registerLiveModelSource helper plus Anthropic-dialect live sources and a ListProvidersWithoutLiveSource reporter. Tests rewritten to seed discovery via SeedLLMCatalog (connectors) and seedDiscoveryLLMs (gateway); the Antigravity curated-gate test became a dynamic passthrough test. Verified: go vet clean, go test -count=1 connectors+gateway green, full backend suite green, codehealth safeguard passed. Follow-up worth considering: extend modelsDevToKeirouter mapping and live sources for proprietary OAuth providers (kiro/codex/cursor), which currently resolve LLM models only via custom registration.
<!-- SECTION:FINAL_SUMMARY:END -->
