---
id: TASK-006
title: Capability checkboxes for custom models (override heuristic tables)
status: Done
assignee:
  - opencode
created_date: '2026-08-31 20:54'
updated_date: '2026-09-01 06:14'
labels: []
milestone: Custom model capabilities
dependencies: []
priority: medium
type: enhancement
ordinal: 6000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Add capability checkboxes to the custom model add/edit dialog so users can declare what a model supports (vision, tools, reasoning, etc.).

Resolve semantics: replace — when a custom model carries capability ticks, capability.Resolve skips the heuristic pattern tables and applies ticks over the default floor. This lets users force OFF wrong heuristics (e.g. *gemma* pattern claims Vision for text-only gemma3 on Ollama) and force ON caps for unknown fine-tunes.

Scope: custom model registry only (not live-discovered models). Must work for models registered on any provider, including ollama / ollama-local which resolve through the same heuristic tables (they are not IsCustomProviderID gateways).

Implementation notes:
- capability.Resolve is the single choke point consumed by dispatch eligibility (backend/internal/dispatch/dispatch.go:302), modality stripping (backend/internal/capability/strip.go:62), and capability payloads (backend/internal/gateway/model_capabilities.go:24) — override there fixes routing, stripping, and UI in one place.
- Ollama codec already carries message images[] (backend/internal/transform/ollama.go:213), so vision tick = images flow; no tick = StripUnsupportedModalities replaces with placeholder.
- Storage: capabilities TEXT column (JSON map cap->bool) on custom_models; package-level override registry in capability package keyed (provider, modelID), refreshed on startup and after mutations.

KEEP DIFF CLEAN FOR UPSTREAM MERGES: prefer isolated new files (capability/user_overrides.go + test), minimal touch points in existing files, no reordering/refactoring of unrelated code.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Custom model add/edit dialog shows capability checkboxes (vision, pdf, audio/video input, image/audio output, tools, reasoning, structured output, web search) that persist per model
- [x] #2 Ticked capabilities replace built-in heuristic resolution for that custom model only (true AND false enforce off), all providers including ollama and ollama-local
- [x] #3 Override applies to routing eligibility, modality stripping, and /v1/models capability payloads; capability_source reports "user"
- [x] #4 New migration adds nullable capabilities column; empty value preserves current heuristic behavior
- [x] #5 Frontend typecheck and backend vet/test pass
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Approved plan (Plannotator, 2026-08-31):

REPLACE semantics: ticks override heuristic tables (true AND false), scope = custom models only.

Upstream-merge cleanliness: new logic in ONE new file backend/internal/capability/user_overrides.go (+ test); Resolve hook ~6 lines at top of Resolve in profile.go; store = additive field/column; gateway = additive fields; migration = new 0030 file; api.ts additive; UI confined to CustomModelsSection.tsx.

Steps:
1. Migration 0030_custom_model_capabilities.sql: ALTER TABLE custom_models ADD COLUMN capabilities TEXT NOT NULL DEFAULT ''.
2. Store repo_custom_providers.go: Capabilities string field; extend customModelColumns, scanCustomModel, CreateModel, UpdateModel.
3. capability/user_overrides.go (new): UserCaps tri-state (Vision PDF AudioInput VideoInput ImageOutput AudioOutput Search Tools Reasoning StructuredOutput), parse from map[string]bool JSON; registry keyed (provider, modelID) + mutex; SetUserOverrides/AddUserOverride/RemoveUserOverride; SourceUser = "user".
4. capability/profile.go: step 0 in Resolve — registry hit starts from defaultProfile(), applies stated ticks (true and false), returns SourceUser + definite VisionState.
5. backend/internal/app/app.go loadCustomProviders: push DB overrides into registry.
6. gateway/admin_custom_providers.go: accept+validate capabilities map[string]bool in create/update, persist JSON; customModelJSON includes capabilities; reloadCustomModels refreshes registry too.
7. Tests capability/user_overrides_test.go: override beats pattern (gemma3:4b vision=false strips, true preserves), floor fills unstated, delete restores heuristic; store round-trip test.
8. frontend/src/lib/api.ts: CustomModel.capabilities?, CustomModelInput.capabilities? (Partial<ModelCapabilities>), CapabilitySource += "user".
9. CustomModelsSection.tsx: capability checkbox grid in modal (kind llm only), absent keys = inherit; CustomModelCell renders ModelCapabilityIcons from saved ticks.

Verify: make vet && make test; cd frontend && npm run typecheck.

Non-goals: no UI for live-discovered models; heuristic tables untouched; context_window separate.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementation complete 2026-09-01.

Evidence per AC:

#1: Checkbox grid in CustomModelModal (CustomModelsSection.tsx, kind=llm only) for all 10 caps; persistence proven by TestCustomModelCapabilityRoundTrip (HTTP create echoes capabilities, store round-trip via ListManualModelsByProvider/GetModel).

#2: Replace semantics verified by TestUserOverrideBeatsPattern (stated vision=false on ollama-local beats *gemma* pattern, other providers unaffected), TestUserOverrideForceVisionOn, TestUserOverrideRemoveRestoresHeuristic. ollama/ollama-local resolve through same tables (not IsCustomProviderID gateways).

#3: Override sits at step 0 of capability.Resolve — single choke point feeding dispatch eligibility (dispatch.go:302), StripUnsupportedModalities/ImagePolicy (strip.go), and /v1/models payloads (model_capabilities.go). capability_source="user" verified in tests; TestCustomModelCapabilityRoundTrip asserts SourceUser before and after patch-clear.

#4: Migration 0030 adds capabilities TEXT NOT NULL DEFAULT ''; empty value short-circuits ParseUserCaps -> no registry entry -> heuristic behavior unchanged (TestUserOverridesSetReplacesRegistry asserts empty decl clears override).

#5: make vet clean; make test 43 packages ok; frontend tsc -b --noEmit exit 0.

Upstream-merge hygiene held: new logic isolated in capability/user_overrides.go (+test); existing-file edits additive (profile.go +9 lines, store +column, gateway +fields, app.go +loader block, api.ts +types); migration is a new appended file; no renames/moves/refactors.

Design note: UserCaps uses nil-pointer tri-state so stated false survives JSON round-trip (map[string]bool storage); unknown keys ignored on parse for forward compatibility, rejected on admin input for typo protection.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
## Capability checkboxes for custom models (user-declared overrides)

### What
Users can now declare capabilities per custom model in the provider's custom-model dialog (10 checkboxes: vision, pdf, audio/video input, image/audio output, web search, tools, reasoning, structured output). Declared flags **replace** the built-in heuristic capability tables for that model — both forcing capabilities on (unknown fine-tunes) and off (e.g. text-only `gemma3:4b` on Ollama that the `*gemma*` pattern wrongly claims has vision).

### Why
Custom models on any provider (incl. `ollama` / `ollama-local`) resolved capabilities through heuristic glob tables that could not be corrected per model, causing image requests to be routed to text-only models instead of stripped.

### How
- **Storage:** `custom_models.capabilities` TEXT column (JSON `cap->bool`), migration `0030_custom_model_capabilities.sql` (default `''` = heuristic behavior unchanged).
- **Resolution:** new `capability/user_overrides.go` — `UserCaps` nil-pointer tri-state, in-memory registry keyed `(provider, modelID)`. `capability.Resolve` step 0: a registry hit applies stated true/false flags over the floor and returns `capability_source: "user"` with definite VisionState. Single choke point, so dispatch eligibility, modality stripping, and `/v1/models` payloads all honor ticks with zero changes to their code.
- **Wiring:** `app.go` loads DB overrides at startup; gateway `reloadCustomModels` re-syncs the registry after every create/update/delete/import; admin create/update validate capability names (unknown → 400) and persist JSON.
- **UI:** checkbox grid in the add/edit modal (LLM kind only). Tri-state semantics: tick = declare on; un-ticking a previously declared-on box declares off; untouched = inherit heuristics. Model cards show declared capability icons.
- **Upstream hygiene:** new logic isolated in one new file + test; all existing-file edits additive; migration appended; no renames/refactors — upstream merges stay mechanical.

### Tests
- New: `capability/user_overrides_test.go` (7 tests: parse, beats-pattern, force-on, vendor-prefix lookup, remove-restores-heuristic, SetReplaces, empty-removes), `gateway TestCustomModelCapabilityRoundTrip` (HTTP create on `ollama-local` → store → `capability.Resolve` → patch-clear → heuristics restored; unknown cap rejected).
- `make vet` clean; `make test` 43/43 packages ok; `frontend tsc -b --noEmit` exit 0.

### Risks / follow-ups
- Live-discovered models (e.g. from Ollama `/api/tags`) intentionally out of scope; could reuse the same registry later if needed.
- `context_window` remains a separate model field, not part of the override set.
<!-- SECTION:FINAL_SUMMARY:END -->
