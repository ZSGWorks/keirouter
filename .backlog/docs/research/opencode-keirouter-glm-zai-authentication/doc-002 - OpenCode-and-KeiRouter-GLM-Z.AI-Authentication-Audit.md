---
id: doc-002
title: OpenCode and KeiRouter GLM Z.AI Authentication Audit
type: specification
created_date: '2026-09-06 16:22'
tags:
  - oauth
  - glm
  - zai
  - opencode
  - research
---
**Audited:** 2026-09-06  
**KeiRouter revision:** current workspace HEAD  
**OpenCode revision:** [`337fd144d2ba144743368f78d9579a99cce175bd`](https://github.com/anomalyco/opencode/commit/337fd144d2ba144743368f78d9579a99cce175bd) (`dev`)  
**Method:** source comparison plus public Z.AI documentation. No local credentials, tokens, or live authorization flow were inspected.

## Result

There is no GLM/Z.AI OAuth login flow to align.

KeiRouter's `glm` and `glm-cn` providers are API-key integrations. OpenCode's Z.AI and Z.AI Coding Plan onboarding is also API-key entry. At the frozen OpenCode revision, no Z.AI, Zhipu, or GLM provider-specific OAuth/login implementation exists; only generic provider/MCP OAuth infrastructure and Z.AI visual assets exist.

Z.AI's public OpenCode guide instructs users to select Z.AI or Z.AI Coding Plan and enter an API key. Its Coding Plan quick start likewise requires users to obtain an API key, then configure a supported coding tool. Public documentation reviewed for this audit does not document OAuth authorization-code, PKCE, token-refresh, or device-authorization support for Z.AI API/Coding Plan access.

**Recommendation:** retain KeiRouter's API-key-only GLM integration. Do not add or infer OAuth endpoints, client IDs, scopes, token semantics, or browser/device flows from OpenCode's generic facilities.

## Evidence

| Concern | OpenCode | KeiRouter | Assessment |
| --- | --- | --- | --- |
| Provider sign-in | [Z.AI OpenCode guide](https://docs.z.ai/devpack/tool/opencode) says `opencode auth login`, select Z.AI or Z.AI Coding Plan, then enter API key | `backend/internal/connectors/catalog.go:235-242` declares both `glm` and `glm-cn` with `AuthKind: "api_key"` | Aligned API-key onboarding |
| Coding Plan credential | [Z.AI Coding Plan quick start](https://docs.z.ai/devpack/quick-start) requires a plan-specific API key | `glm` uses the documented Anthropic-compatible `https://api.z.ai/api/anthropic/v1` base URL | Aligned credential model and endpoint family |
| Provider-specific OAuth code | No Z.AI/Zhipu/GLM authentication implementation found in OpenCode tree at frozen revision | No `glm`, `zai`, or `zhipu` entry in `backend/internal/oauth/providers.go` | No OAuth parity gap |
| Generic OAuth implementation | `packages/opencode/src/provider/auth.ts` dispatches plugin-provided OAuth; `packages/opencode/src/auth/index.ts` persists generic OAuth records | `backend/internal/oauth/providers.go`, `flow.go`, and `manager.go` support configured providers | Shared generic capability is not provider evidence |
| Credential storage | OpenCode `packages/opencode/src/auth/index.ts` writes `auth.json` with mode `0600` | `backend/internal/vault/vault.go:31-78` envelope-encrypts API keys and OAuth tokens | KeiRouter protects stored API keys more strongly |

## Current Behavior

### OpenCode

Z.AI documentation identifies OpenCode as a supported GLM Coding Plan tool, but documents API-key entry rather than browser or device authorization. The guide's explicit sequence is:

1. Obtain an API key from Z.AI API Console.
2. Run `opencode auth login`.
3. Choose Z.AI or Z.AI Coding Plan.
4. Enter API key.

This matches generic OpenCode API authentication storage. Its generic `ProviderAuth` service can execute plugin-supplied OAuth, but no provider-specific Z.AI plugin/auth hook was present in the audited tree. Generic OAuth and MCP OAuth modules therefore do not establish a Z.AI OAuth flow.

### KeiRouter

`backend/internal/connectors/catalog.go` defines:

- `glm`: GLM Coding, Anthropic dialect, `https://api.z.ai/api/anthropic/v1`, API key.
- `glm-cn`: GLM (China), OpenAI dialect, `https://open.bigmodel.cn/api/coding/paas/v4`, API key.

The configured OAuth provider map has no GLM/Z.AI entry. KeiRouter's OAuth framework can support authorization-code and device-code providers, with encrypted credential storage and refresh coordination, but it must not be enabled for Z.AI without official flow parameters and an explicit product decision.

## Public Z.AI Documentation

The reviewed documentation provides only API-key authentication evidence:

- [Z.AI documentation quick start](https://docs.z.ai/) instructs users to create an API key and submit `Authorization: Bearer YOUR_API_KEY`.
- [GLM Coding Plan quick start](https://docs.z.ai/devpack/quick-start) instructs individual and team users to obtain plan-specific API keys and lists supported API base URLs.
- [Z.AI OpenCode setup](https://docs.z.ai/devpack/tool/opencode) explicitly requests API-key entry after provider selection.

Absence from public documentation is not proof that Z.AI has no internal OAuth system. It is sufficient evidence that KeiRouter must not implement an undocumented OAuth flow for this public API/Coding Plan integration.

## Follow-ups

### 1. Keep API-key support current

**Priority:** Low.  
Review Z.AI endpoint, protocol, model, and API-key guidance when provider documentation changes. This is routine provider-catalog maintenance, not OAuth work.

### 2. Create a separate OAuth feature task only on evidence

**Priority:** Deferred.  
Open a feature task only if Z.AI publishes OAuth/device-flow documentation or product requirements demand a supported subscription-login experience. Require official authorization, token, refresh, scope, redirect, and entitlement semantics before implementation.

### 3. Validate before any future OAuth rollout

**Required gates:** official Z.AI documentation or written provider authorization; disposable-account live validation; callback/state/PKCE and refresh-race tests; encrypted token persistence; no token/claim logging; authorization and credential-forwarding review.

## Verification Boundaries

This audit establishes current implementation and public documentation facts at cited revisions. It does not establish undocumented Z.AI web-session behavior, internal application APIs, subscription terms beyond published guides, or acceptance of any inferred OAuth/device flow.
