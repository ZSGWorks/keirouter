---
id: doc-003
title: Security Scan Findings 2026-09-09
type: other
created_date: '2026-09-09 08:14'
updated_date: '2026-09-09 08:14'
tags:
  - security
  - audit
  - findings
---
# Security Scan Findings

Standard source-based security scan completed 2026-09-09 against revision 4bd79c28f34c201ff1382dba1127bde676a67d2e. Coverage was partial: five risk-focused surfaces were reviewed from a 698-file inventory. No live deployment or dynamic exploit testing was performed.

## Findings

1. Critical — Default Compose exposes the dashboard remotely with loopback protection disabled, while the known keirouter password can establish an administrator session. Evidence: compose.yaml:17-26, backend/internal/auth/auth.go:28-30, backend/internal/gateway/auth_handlers.go:31-55.
2. High — Gemini-native requests bypass API-key model restrictions and effective limits. Evidence: backend/internal/gateway/gemini.go:26-100.
3. Medium — Default remote dashboard traffic is HTTP, exposing administrator credentials and sessions to network-path attackers. Evidence: compose.yaml:17-26, backend/internal/gateway/auth_handlers.go:105-155.
4. Medium — A public portal URL functions as an unrevocable bearer capability for key usage and budget data. Evidence: backend/internal/gateway/server.go:315-317, backend/internal/gateway/handlers.go:1045-1241.
5. Medium — Multipart transcription uploads may spool unbounded data to disk before the body-size limit applies. Evidence: backend/internal/gateway/media.go:178-199.
6. Low — Metrics become publicly reachable under the default non-loopback Compose setting. Evidence: backend/internal/gateway/server.go:262-268, backend/internal/gateway/middleware.go:72-90.
7. Low — Kiro OAuth client secrets are persisted in unsealed account metadata and can enter backups/exports. Evidence: backend/internal/gateway/kiro.go:137-148, backend/internal/gateway/oauth.go:499-512, backend/internal/vault/vault.go:72-80.
8. Low — The SPA fallback has a local-path existence oracle through raw-path joining and os.Stat. Evidence: backend/internal/gateway/server.go:361-375.

## Recommended remediation order

1. Remove the remotely usable default dashboard password and require a supplied high-entropy admin secret for non-loopback deployments.
2. Apply allowed-target filtering and effective limits before Gemini-native dispatch; add restricted-key parity tests for every protocol.
3. Put public dashboards behind TLS, set Secure session cookies, and protect metrics independently.
4. Replace portal key IDs with scoped, expiring, revocable portal tokens.
5. Apply MaxBytesReader before multipart parsing and clean up temporary upload files.
6. Encrypt or eliminate persisted Kiro client secrets and redact them from exports.
7. Confine and sanitize SPA fallback paths before filesystem checks.

## Scope note

The listed severities are source-review assessments. Confirm deployment-specific exposure, TLS termination, and path normalization before treating the lower-confidence static-serving issue as a live exploit.
