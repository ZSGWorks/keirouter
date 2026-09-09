---
id: doc-001
title: OpenCode Codex OAuth Audit
type: specification
created_date: '2026-09-06 16:15'
tags:
  - oauth
  - codex
  - opencode
  - research
---
**Audited:** 2026-09-06  
**KeiRouter revision:** `864ada6`  
**OpenCode revision:** `337fd144d2ba144743368f78d9579a99cce175bd` (`dev`)  
**Method:** source comparison only. No local auth files, credentials, or live OpenAI authorization flow were inspected.

## Result

KeiRouter's browser-based Codex login is protocol-aligned with OpenCode for the public client identity, authorization-code PKCE flow, callback URI, scopes, and Codex Responses endpoint. KeiRouter deliberately differs in ways suited to a multi-account gateway: encrypted persistent credentials, pre-expiry refresh, and CLI identity headers.

No production OAuth change is recommended from source evidence alone. Two follow-ups need an explicit product decision or live, disposable-account validation before implementation.

## Evidence

| Component | OpenCode | KeiRouter |
| --- | --- | --- |
| Codex OAuth implementation | [`codex.ts`](https://github.com/anomalyco/opencode/blob/337fd144d2ba144743368f78d9579a99cce175bd/packages/opencode/src/plugin/openai/codex.ts) | `backend/internal/oauth/providers.go`, `backend/internal/oauth/flow.go` |
| Token storage | [`auth/index.ts`](https://github.com/anomalyco/opencode/blob/337fd144d2ba144743368f78d9579a99cce175bd/packages/opencode/src/auth/index.ts) | `backend/internal/vault/vault.go`, `backend/internal/store/repo_accounts.go` |
| OpenCode tests | [`codex.test.ts`](https://github.com/anomalyco/opencode/blob/337fd144d2ba144743368f78d9579a99cce175bd/packages/opencode/test/plugin/codex.test.ts) | `backend/internal/oauth/oauth_test.go`, `backend/internal/connectors/codex_test.go` |

## Browser OAuth

| Concern | OpenCode | KeiRouter | Assessment |
| --- | --- | --- | --- |
| Client ID | `app_EMoamEEZ73f0CkXaXp7hrann` | Same | Aligned |
| Authorization endpoint | `https://auth.openai.com/oauth/authorize` | Same | Aligned |
| Token endpoint | `https://auth.openai.com/oauth/token` | Same | Aligned |
| Redirect URI | `http://localhost:1455/auth/callback` | Same | Aligned |
| Scope | `openid profile email offline_access` | Same | Aligned |
| PKCE | S256; 43-character verifier | S256; 32 random bytes base64url encoded | Both satisfy RFC 7636 length and entropy requirements |
| State | 32 random bytes base64url; exact callback match | Same size; server-side session lookup | Aligned |
| Extra parameters | `id_token_add_organizations=true`, `codex_cli_simplified_flow=true`, `originator=opencode` | Same first two; `originator=codex_cli_rs`; `prompt=login` | Intentional identity and multi-account behavior difference |
| Callback server | Process-local server on port 1455; five-minute timeout | Gateway-owned listeners on IPv4 and IPv6 loopback; five-minute timeout | KeiRouter handles browser/dashboard separation and dual-stack localhost |
| Callback completion | Resolves CLI authorization promise | Exchanges server-side, persists account, and dashboard polls status because OpenAI COOP can sever `window.opener` | KeiRouter gateway improvement |

`prompt=login` is important in KeiRouter's account-pool model. Its source comment documents that it prevents a second browser sign-in from silently reusing an existing Auth0 session and invalidating the first account's refresh token family. Do not remove it to match OpenCode without validating concurrent account behavior.

## Token Lifecycle

| Concern | OpenCode | KeiRouter | Assessment |
| --- | --- | --- | --- |
| Stored fields | Access token, refresh token, expiry, optional account ID | Access and refresh tokens, expiry, provider metadata, account record | Equivalent essential OAuth state |
| Storage | XDG `auth.json`, mode `0600` | Per-field envelope encryption through vault | KeiRouter security improvement |
| Refresh point | Missing or expired access token | Within 60 seconds of expiry, or forced after upstream token rejection | KeiRouter avoids in-flight expiry races |
| Concurrent refresh | One promise per loaded plugin | `singleflight` per account | Both avoid refresh-token rotation races |
| Permanent failure | Propagates refresh failure | Marks account `NeedsReconnect` and lets dispatcher fall back | KeiRouter gateway improvement |
| Account ID extraction | ID token, then access token: root claim, namespaced claim, then first organization | ID token only: namespaced claim, then legacy `account_id` | See follow-up 1 |

## Inference Requests

Both implementations send Codex OAuth traffic to `https://chatgpt.com/backend-api/codex/responses` and set bearer authorization plus `ChatGPT-Account-Id` when present.

| Concern | OpenCode | KeiRouter | Assessment |
| --- | --- | --- | --- |
| Identity | `originator=opencode`; OpenCode user agent | `originator=codex_cli_rs`; Codex CLI user agent/version | KeiRouter intentionally emulates Codex CLI |
| Session header | OpenCode conversation session ID | Stable gateway account ID | Different session model; neither is proven preferable from source alone |
| Model handling | Filters OAuth model catalog | Normalizes model aliases and reasoning effort | Different product responsibilities |
| Compute residency | Sends `x-openai-internal-codex-residency` parsed from JWT, excluding `no_constraint` | Does not send it | See follow-up 2 |
| Streaming transport | Optional WebSocket transport | HTTP/SSE Responses connector | Feature difference, not OAuth compatibility defect |

## Follow-ups

### 1. Decide account-ID claim fallback policy

**Priority:** Medium.  
**Why:** OpenCode accepts root `chatgpt_account_id`, namespaced `https://api.openai.com/auth.chatgpt_account_id`, and first `organizations[].id`; it also falls back from ID token to access token. KeiRouter currently accepts only namespaced ID-token metadata and legacy `account_id`. Accounts with only an OpenCode-supported shape could omit `ChatGPT-Account-Id`, which may bind a multi-workspace token incorrectly.

**Small implementation if approved:** extend `applyTokenMetadata` in `backend/internal/oauth/flow.go` to retain existing precedence, then add root claim, first-organization, and access-token fallback. Add table-driven tests covering each accepted shape and precedence. Do not trust JWT claims for authorization decisions; this metadata only selects the account header after the upstream has issued the token.

**Validation required:** a disposable multi-workspace subscription account or captured, sanitized token payload fixtures. Source code alone cannot prove which claims OpenAI currently emits.

### 2. Decide whether to forward compute residency

**Priority:** Low.  
**Why:** OpenCode forwards non-`no_constraint` compute residency from OAuth JWT claims only to the Codex backend. KeiRouter does not parse or forward this header. This may affect region routing, but its semantics are undocumented.

**Small implementation if approved:** parse only the allowlisted claim from an access token, forward it only to the fixed Codex backend, omit empty and `no_constraint` values, and add connector header tests. Never log the token or claim value.

**Validation required:** OpenAI documentation or a live test showing a behavioral difference. Do not forward an undocumented internal header merely to copy a client.

### 3. Add Codex headless login only when product requires it

**Priority:** Deferred.  
**Why:** OpenCode supports an undocumented custom device-auth flow. KeiRouter has no Codex equivalent, but its dashboard browser flow works for its current product model. Introducing a second flow adds sensitive endpoint and polling behavior without evidence of user need.

**Decision gate:** create a separate feature task if unattended/headless gateway enrollment is required. Validate endpoint stability, cancellation, expiry, storage, and account-pool interactions with disposable credentials.

## Verification Boundaries

This report establishes implementation differences at the cited revisions. It does not prove current OpenAI acceptance of public client identifiers, redirects, undocumented device-auth endpoints, internal request headers, model entitlements, or subscription terms.
