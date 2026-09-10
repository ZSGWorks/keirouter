---
id: doc-003
title: Security Scan Findings 2026-09-09
type: other
created_date: '2026-09-09 08:14'
updated_date: '2026-09-09 20:01'
tags:
  - security
  - audit
  - findings
---
# Security Scan Findings

Standard source-based security scan completed 2026-09-09 against revision 4bd79c28f34c201ff1382dba1127bde676a67d2e. Coverage was partial: five risk-focused surfaces were reviewed from a 698-file inventory. No live deployment or dynamic exploit testing was performed.

## Re-verification 2026-09-09 (HEAD `39e9f15347ea3b5d9d3a304e3b0489e49a864538`)

All eight findings were re-verified line-by-line against HEAD `39e9f15` (two commits after the scan revision: `eea462b` chain/cache work, `39e9f15` plugin provisioning — neither remediates a cited surface). **Status: all findings still present; none fixed.** Evidence line numbers below are current for HEAD `39e9f15`.

Additional context confirmed during re-verification:

- The binary's own default is safe — `config.go:311` defaults `BindLoopbackOnly: true`; the `compose.yaml` override is what exposes the dashboard (finding 1).
- Media endpoints already enforce key policy parity via `mediaOptions` (`media.go:31,40` calls `filterAllowedTargets` and `effectiveLimits`); the protocol-bypass gap is Gemini-only (finding 2).
- Gemini body reads are capped through `readRequestBody` (`handlers.go:39-41`, `http.MaxBytesReader` → 413); the missing controls are target filtering and effective limits, not the body cap (finding 2).
- `sessionCookieSecure` (`auth_handlers.go:130-155`) is correct TLS/forwarded-proto-aware code; finding 3 is a deployment-level residual (no TLS terminator in `compose.yaml`, direct origin-IP HTTP).
- Deployment split: `compose.coolify-postgres.yaml:13` binds the host port to `127.0.0.1:20180`, so the origin is not directly exposed there, but it keeps `BIND_LOOPBACK_ONLY:-false` — required, because the Coolify proxy reaches the gateway over the docker network (non-loopback `RemoteAddr`). Findings 1/3/6 apply to the default `compose.yaml` port binding, not the Coolify template's host port.

Remediation has not yet started; the anchors below are planned fixes, not completed work.

## Findings

1. Critical — Default Compose exposes the dashboard remotely with loopback protection disabled, while the known keirouter password can establish an administrator session. Evidence: compose.yaml:17-26, backend/internal/auth/auth.go:28-30, backend/internal/gateway/auth_handlers.go:31-55. Confirmed at HEAD 39e9f15.
2. High — Gemini-native requests bypass API-key model restrictions and effective limits. Evidence: backend/internal/gateway/gemini.go:26-100 (`handleGeminiGenerate` resolves and dispatches without `filterAllowedTargets` or `effectiveLimits`; parity reference: handlers.go:223-277, media.go:23-49). Confirmed at HEAD 39e9f15.
3. Medium — Default remote dashboard traffic is HTTP, exposing administrator credentials and sessions to network-path attackers. Evidence: compose.yaml:17-26, backend/internal/gateway/auth_handlers.go:105-155. Confirmed at HEAD 39e9f15 (deployment-level; cookie code correct).
4. Medium — A public portal URL functions as an unrevocable bearer capability for key usage and budget data. Evidence: backend/internal/gateway/server.go:315-317 (route now at server.go:330), backend/internal/gateway/handlers.go:1029-1045 (existence check only — no token, expiry, or revocation). Confirmed at HEAD 39e9f15.
5. Medium — Multipart transcription uploads may spool unbounded data to disk before the body-size limit applies. Evidence: backend/internal/gateway/media.go:178-199 (`ParseMultipartForm(s.requestBodyLimit())` at :180 — the argument is the maxMemory RAM threshold, not a body cap; the `io.LimitReader` at :195 runs after spooling). Confirmed at HEAD 39e9f15.
6. Low — Metrics become publicly reachable under the default non-loopback Compose setting. Evidence: backend/internal/gateway/server.go:262-268 (guard now at :278-283), backend/internal/gateway/middleware.go:72-90 (`loopbackOnly` no-ops when `BindLoopbackOnly=false`). Confirmed at HEAD 39e9f15.
7. Low — Kiro OAuth client secrets are persisted in unsealed account metadata and can enter backups/exports. Evidence: backend/internal/gateway/kiro.go:137-148 (`kiro_client_secret` placed in `Tokens.Extra` at :143), backend/internal/gateway/oauth.go:499-512 (Extra copied into metadata), backend/internal/vault/vault.go:72-80 (metadata JSON-marshaled into plaintext `acc.Metadata`; only access/refresh tokens are sealed). Confirmed at HEAD 39e9f15.
8. Low — The SPA fallback has a local-path existence oracle through raw-path joining and os.Stat. Evidence: backend/internal/gateway/server.go:379-389 (raw `filepath.Join(frontendDir, path)` + `os.Stat`, no traversal confinement). Confirmed at HEAD 39e9f15.

## Recommended remediation order

1. Gemini parity: insert `filterAllowedTargets` and `effectiveLimits` into `handleGeminiGenerate` after `resolveTargets` (gemini.go:68), mirroring handlers.go:223-277; add restricted-key parity tests across OpenAI/Anthropic/Gemini/media dialects.
2. Default password exposure: flip the `compose.yaml` `KEIROUTER_BIND_LOOPBACK_ONLY` default to `true` (document the LAN override), and/or reject default-password logins from non-loopback `RemoteAddr` in `handleLogin`; the Coolify template needs no change (loopback host port already confines the origin).
3. TLS for public dashboards: deployment-level — terminate TLS at the origin or restrict origin firewall to the Coolify/Cloudflare path; document in deploy/README.md (cookie code at auth_handlers.go:130-155 already handles Secure correctly).
4. Portal capability: replace bare key IDs with a scoped, expiring, revocable portal token (store field + admin rotate/revoke endpoint + frontend); largest change — stage after the above.
5. Multipart cap: wrap `r.Body` in `http.MaxBytesReader(w, r.Body, s.requestBodyLimit())` before `ParseMultipartForm`, pass a fixed maxMemory, and return 413 on `*http.MaxBytesError`; add an oversized-upload regression test.
6. Metrics guard: add a dedicated `MetricsLoopbackOnly` security config defaulting to true, independent of `BindLoopbackOnly`, and key the /metrics route middleware on it.
7. Kiro secret sealing: stop writing `kiro_client_secret` into `Tokens.Extra`; seal it through the existing vault secret slot, repopulate it into `creds.Extra` at `Vault.Open`, and redact it from admin listings/exports; rotation for existing plaintext values = reconnect the account.
8. SPA confinement: `filepath.Clean("/"+path)` and reject `..` segments before the `os.Stat` existence probe.

## Scope note

The listed severities are source-review assessments. Confirm deployment-specific exposure, TLS termination, and path normalization before treating the lower-confidence static-serving issue as a live exploit.
