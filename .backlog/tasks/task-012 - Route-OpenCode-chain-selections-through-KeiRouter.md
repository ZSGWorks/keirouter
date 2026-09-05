---
id: TASK-012
title: Route OpenCode chain selections through KeiRouter
status: Done
assignee: []
created_date: '2026-09-05 18:08'
updated_date: '2026-09-05 18:20'
labels:
  - opencode
  - plugin
  - routing
dependencies: []
references:
  - '@keirouter-opencode-plugin/src/index.ts'
  - backend/internal/gateway/resolve.go
modified_files:
  - '@keirouter-opencode-plugin/src/index.ts'
  - '@keirouter-opencode-plugin/tests/index.test.ts'
  - build-opencode-plugin.sh
priority: high
type: bug
ordinal: 12000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
OpenCode prefixes gateway chain names with the KeiRouter provider ID before sending requests. KeiRouter then interprets the prefixed identifier as an upstream provider/model target and fails because no upstream connector named keirouter exists. Preserve chain selection through the plugin so OpenCode users can run listed KeiRouter chains.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Selecting a listed KeiRouter chain in OpenCode sends its bare chain name to the gateway
- [x] #2 Explicit provider/model identifiers remain unchanged
- [x] #3 Requests using a selected KeiRouter chain resolve to its configured upstream steps
- [x] #4 Plugin tests cover model identifier rewriting and unchanged provider/model requests
- [x] #5 Plugin test suite and build pass
- [x] #6 Root build script rebuilds the plugin and installs it at OpenCode's configured flat plugin path
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 Codehealth is green
- [x] #2 Changes are fork maintenance compatible
- [x] #3 Changes are validated by tests
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Keep OpenCode model keys provider-qualified for selection, but adapt only outbound KeiRouter OpenAI-compatible request JSON.
2. Extend the authenticated fetch wrapper with the configured provider ID; when a same-origin KeiRouter request has a JSON body whose model starts with `keirouter/`, replace that prefix with the bare model name before forwarding.
3. Preserve all provider-qualified upstream models such as `glm/glm-5.3-flash` and OpenRouter vendor namespaces unchanged.
4. Add plugin unit coverage for rewritten chain payloads and unchanged explicit provider/model payloads, including Request/init body handling used by the SDK.
5. Add a root executable build script that builds the plugin and installs its distributable at the configured flat OpenCode plugin path.
6. Run the build script, plugin tests and build, then validate OpenCode model discovery and a streaming chain request against the local gateway.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implemented provider-prefix normalization in the plugin fetch wrapper. Only same-origin KeiRouter JSON requests whose model begins with the configured plugin provider ID are rewritten to a bare model name; explicit upstream provider/model IDs remain unchanged.

Added build-opencode-plugin.sh at repository root. It builds the plugin and installs dist/index.js to ${OPENCODE_CONFIG_DIR:-$HOME/.config/opencode}/plugins/keirouter-plugin.js.

Validation: plugin tests 13/13 pass; plugin build passes; IDE compile and error inspections pass; CodeHealth pre-commit safeguard passes with stable verdict; live request using keirouter/GLM-5-3-flash-chain returned HTTP 200 from ollama/glm-5.3-flash.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Fixed OpenCode chain routing through KeiRouter. The plugin now removes only its own provider prefix from outbound gateway JSON model IDs, allowing bare chain names to reach the existing chain resolver while preserving explicit upstream provider/model identifiers.

Added build-opencode-plugin.sh to build and install the plugin at OpenCode's flat configured plugin path. Added fetch-wrapper tests for prefixed chains, direct provider models, and Request-body input.

Validated with npm test (13 passing), npm run build, IDE compile/error checks, CodeHealth pre-commit safeguard (passed), and a live local gateway request to keirouter/GLM-5-3-flash-chain returning HTTP 200 via ollama/glm-5.3-flash.
<!-- SECTION:FINAL_SUMMARY:END -->
