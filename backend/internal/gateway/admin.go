package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/consolelog"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/store"
)

// mountAdmin registers the dashboard admin endpoints on the given router. These
// manage API keys, provider accounts, routing chains, budgets, and usage.

func (s *Server) mountAdmin(r chi.Router) {
	r.Get("/providers", s.adminListProviders)
	r.Get("/providers/{id}/models", s.adminProviderModels)
	r.Get("/providers/{id}/routing", s.adminGetProviderRouting)
	r.Post("/providers/{id}/routing", s.adminUpdateProviderRouting)
	r.Patch("/providers/{id}/routing", s.adminUpdateProviderRouting)

	r.Get("/keys", s.adminListKeys)
	r.Post("/keys", s.adminCreateKey)
	r.Patch("/keys/{id}", s.adminUpdateKey)
	r.Delete("/keys/{id}", s.adminDeleteKey)

	r.Get("/accounts", s.adminListAccounts)
	r.Post("/accounts", s.adminCreateAccount)
	r.Post("/accounts/bulk", s.adminBulkCreateAccounts)
	r.Post("/validate-key", s.adminValidateKey)
	r.Patch("/accounts/{id}", s.adminUpdateAccount)
	r.Delete("/accounts/{id}", s.adminDeleteAccount)
	r.Post("/accounts/{id}/test", s.adminTestAccount)
	r.Get("/accounts/{id}/quota", s.adminAccountQuota)
	r.Get("/accounts/{id}/codex-reset-credits", s.adminCodexResetCredits)
	r.Post("/accounts/{id}/codex-consume-credit", s.adminCodexConsumeCredit)
	r.Get("/accounts/{id}/codex-usage-details", s.adminCodexUsageDetails)

	r.Get("/chains", s.adminListChains)
	r.Post("/chains", s.adminCreateChain)
	r.Patch("/chains/{id}", s.adminUpdateChain)
	r.Delete("/chains/{id}", s.adminDeleteChain)

	r.Get("/plans", s.adminListPlans)
	r.Post("/plans", s.adminCreatePlan)
	r.Patch("/plans/{id}", s.adminUpdatePlan)
	r.Delete("/plans/{id}", s.adminDeletePlan)
	r.Get("/plans/{id}/keys", s.adminListPlanKeys)

	r.Get("/budgets", s.adminListBudgets)
	r.Get("/budgets/status", s.adminBudgetStatus)
	r.Post("/budgets", s.adminCreateBudget)
	r.Patch("/budgets/{id}", s.adminUpdateBudget)
	r.Delete("/budgets/{id}", s.adminDeleteBudget)

	r.Get("/usage", s.adminUsageSummary)
	r.Get("/usage/insights", s.adminUsageInsights)
	r.Get("/usage/models", s.adminModelUsageAccurate)
	r.Get("/usage/stream", s.adminUsageStream)
	r.Get("/quota", s.adminQuotaUsage)
	r.Get("/health/accounts", s.adminListAccountHealth)
	r.Post("/health/check-now", s.adminRunHealthCheck)
	r.Post("/pricing/refresh", s.adminRefreshPricing)
	s.mountProviderHealth(r)
	r.Get("/console", s.adminConsoleLog)
	r.Delete("/console", s.adminConsoleClear)
	r.Get("/console/stream", s.adminConsoleStream)

	r.Get("/proxy-pools", s.adminListProxyPools)
	r.Post("/proxy-pools", s.adminCreateProxyPool)
	r.Post("/proxy-pools/cloudflare-deploy", s.adminDeployCloudflareRelay)
	r.Patch("/proxy-pools/{id}", s.adminUpdateProxyPool)
	r.Delete("/proxy-pools/{id}", s.adminDeleteProxyPool)
	r.Post("/proxy-pools/{id}/test", s.adminTestProxyPool)

	r.Get("/skills", s.adminListSkills)
	r.Post("/skills", s.adminCreateSkill)
	r.Post("/skills/{id}", s.adminUpdateSkill)
	r.Delete("/skills/{id}", s.adminDeleteSkill)

	r.Get("/models/alias", s.adminListAliases)
	r.Put("/models/alias", s.adminSetAlias)
	r.Delete("/models/alias", s.adminDeleteAlias)

	r.Get("/models/disabled", s.adminListDisabledModels)
	r.Post("/models/disabled", s.adminDisableModels)
	r.Delete("/models/disabled", s.adminEnableModels)

	r.Get("/settings/endpoint", s.adminGetEndpointSettings)
	r.Post("/settings/endpoint", s.adminUpdateEndpointSettings)
	r.Post("/settings/headroom-test", s.adminTestHeadroom)
	r.Get("/settings/access", s.adminGetAccessSettings)
	r.Post("/settings/access", s.adminUpdateAccessSettings)
	r.Post("/settings/database/export", s.adminExportDatabase)
	r.Post("/settings/database", s.adminImportDatabase)
	r.Post("/settings/database/import-foreign", s.adminImportForeignConfig)
	r.Get("/settings/sqlite", s.adminSQLiteStatus)
	r.Get("/settings/sqlite/backup", s.adminSQLiteBackup)
	r.Post("/settings/sqlite/restore", s.adminSQLiteRestore)
	r.Post("/settings/proxy-test", s.adminTestProxy)

	// Update check (queries GitHub for the latest release + changelog).
	r.Get("/update/check", s.adminUpdateCheck)

	// Tunnel management endpoints.
	r.Get("/tunnel/status", s.adminTunnelStatus)
	r.Post("/tunnel/enable", s.adminTunnelEnable)
	r.Post("/tunnel/disable", s.adminTunnelDisable)
	r.Get("/tunnel/tailscale-check", s.adminTailscaleCheck)
	r.Post("/tunnel/tailscale-enable", s.adminTailscaleEnable)
	r.Post("/tunnel/tailscale-disable", s.adminTailscaleDisable)
	r.Post("/tunnel/tailscale-install", s.adminTailscaleInstall)

	s.mountOAuth(r)
	s.mountKiro(r)
	s.mountCustomFlows(r)
	s.mountCustomProviders(r)

	s.mountCLITools(r)

	// Branding / white-label settings.
	r.Get("/settings/branding", s.adminGetBranding)
	r.Post("/settings/branding", s.adminUpdateBranding)

	// System monitoring (CPU, memory, disk, Go runtime).
	r.Get("/system", s.adminSystem)
	r.Get("/system/history", s.adminSystemHistory)
	r.Get("/system/resources", s.adminSystemResourceHistory)

	// Guardrails (content safety policies + audit log).
	s.mountGuardrails(r)
}

const adminTenant = store.DefaultTenantID

// ---- providers --------------------------------------------------------------

// adminRefreshPricing force-refreshes the models.dev catalog and reloads its
// projected prices.
func (s *Server) adminRefreshPricing(w http.ResponseWriter, r *http.Request) {
	if s.refreshPricingCatalog == nil {
		writeError(w, http.StatusInternalServerError, "pricing reload not configured")
		return
	}
	ctx, cancel := contextWithTimeout(r, 90*time.Second)
	defer cancel()
	if err := s.refreshPricingCatalog(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "pricing reload failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// ---- providers --------------------------------------------------------------

func (s *Server) adminListAliases(w http.ResponseWriter, r *http.Request) {
	aliases, err := s.aliases.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make(map[string]string, len(aliases))
	for _, a := range aliases {
		out[a.Alias] = a.Target
	}
	writeJSON(w, http.StatusOK, map[string]any{"aliases": out})
}

func (s *Server) adminSetAlias(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Alias  string `json:"alias"`
		Target string `json:"target"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Alias == "" || body.Target == "" {
		writeError(w, http.StatusBadRequest, "alias and target are required")
		return
	}
	if !strings.Contains(body.Target, "/") {
		writeError(w, http.StatusBadRequest, "target must be in 'provider/model' format")
		return
	}
	if err := s.aliases.Set(r.Context(), body.Alias, body.Target); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) adminDeleteAlias(w http.ResponseWriter, r *http.Request) {
	alias := r.URL.Query().Get("alias")
	if alias == "" {
		writeError(w, http.StatusBadRequest, "alias query param is required")
		return
	}
	if err := s.aliases.Delete(r.Context(), alias); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- disabled models --------------------------------------------------------

func (s *Server) adminConsoleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial history.
	entries := s.consoleLog.Entries()
	initData, _ := json.Marshal(map[string]any{"type": "init", "logs": entries})
	fmt.Fprintf(w, "data: %s\n\n", initData)
	flusher.Flush()

	// Subscribe to new log lines via buffered channel.
	listener := consolelog.NewListener(256)
	s.consoleLog.Subscribe(listener)
	defer s.consoleLog.Unsubscribe(listener)

	// Keepalive ping every 25s.
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-listener.C:
			var data []byte
			if ev.Clear {
				data, _ = json.Marshal(map[string]any{"type": "clear"})
			} else {
				data, _ = json.Marshal(map[string]any{"type": "line", "log": ev.Entry})
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// adminConsoleClear clears the log buffer.
func (s *Server) adminConsoleClear(w http.ResponseWriter, r *http.Request) {
	s.consoleLog.Clear()
	w.WriteHeader(http.StatusNoContent)
}

// ---- database export/import -------------------------------------------------

func (s *Server) adminImportDatabase(w http.ResponseWriter, r *http.Request) {
	var payload map[string]json.RawMessage
	if !decodeJSON(w, r, &payload) {
		return
	}
	ctx := r.Context()
	imported := 0

	// A portable backup carries passphrase-encrypted secrets; the passphrase is
	// supplied alongside the payload so we can re-key into the local master key.
	portable := false
	if raw, ok := payload["portable"]; ok {
		_ = json.Unmarshal(raw, &portable)
	}
	passphrase := ""
	if raw, ok := payload["passphrase"]; ok {
		_ = json.Unmarshal(raw, &passphrase)
	}
	passphrase = strings.TrimSpace(passphrase)
	if portable && passphrase == "" {
		writeError(w, http.StatusBadRequest, "this backup is portable: a passphrase is required to import it")
		return
	}

	// Import providers (accounts) — preserves encrypted credentials.
	if raw, ok := payload["accounts"]; ok {
		var accounts []struct {
			ID                string                `json:"id"`
			Provider          string                `json:"provider"`
			Label             string                `json:"label"`
			AuthKind          string                `json:"auth_kind"`
			Priority          int                   `json:"priority"`
			Disabled          bool                  `json:"disabled"`
			ProxyPoolID       string                `json:"proxy_pool_id"`
			Metadata          string                `json:"metadata"`
			SecretWrappedDEK  string                `json:"secret_wrapped_dek"`
			SecretCiphertext  string                `json:"secret_ciphertext"`
			TokenWrappedDEK   string                `json:"token_wrapped_dek"`
			TokenCiphertext   string                `json:"token_ciphertext"`
			RefreshWrappedDEK string                `json:"refresh_wrapped_dek"`
			RefreshCiphertext string                `json:"refresh_ciphertext"`
			PortableSecret    portableAccountSecret `json:"portable_secret"`
			TokenExpiresAt    *string               `json:"token_expires_at"`
		}
		if err := json.Unmarshal(raw, &accounts); err == nil {
			for _, a := range accounts {
				now := time.Now()
				var expiresAt *time.Time
				if a.TokenExpiresAt != nil {
					if t, err := time.Parse(time.RFC3339, *a.TokenExpiresAt); err == nil {
						expiresAt = &t
					}
				}
				acc := store.Account{
					ID:                defaultStr(a.ID, uuid.NewString()),
					TenantID:          adminTenant,
					Provider:          a.Provider,
					Label:             a.Label,
					AuthKind:          store.AuthKind(defaultStr(a.AuthKind, "api_key")),
					SecretWrappedDEK:  a.SecretWrappedDEK,
					SecretCiphertext:  a.SecretCiphertext,
					TokenWrappedDEK:   a.TokenWrappedDEK,
					TokenCiphertext:   a.TokenCiphertext,
					RefreshWrappedDEK: a.RefreshWrappedDEK,
					RefreshCiphertext: a.RefreshCiphertext,
					TokenExpiresAt:    expiresAt,
					Metadata:          a.Metadata,
					Priority:          defaultInt(a.Priority, 100),
					Disabled:          a.Disabled,
					ProxyPoolID:       a.ProxyPoolID,
					CreatedAt:         now,
					UpdatedAt:         now,
				}
				if portable {
					if err := s.importPortableSecrets(&acc, a.PortableSecret, passphrase); err != nil {
						s.consoleLog.Log("ERROR", fmt.Sprintf("Portable import failed for account %s", acc.ID), err.Error())
						writeError(w, http.StatusBadRequest, "portable import failed: wrong passphrase or corrupt backup")
						return
					}
				}
				if err := s.accounts.Create(ctx, acc); err == nil {
					imported++
				}
			}
		}
	}

	// Import chains.
	if raw, ok := payload["chains"]; ok {
		var chains []importedChain
		if err := json.Unmarshal(raw, &chains); err == nil {
			for _, c := range chains {
				chain, err := chainFromImport(c)
				if err != nil {
					continue
				}
				if err := s.chains.Create(ctx, chain); err == nil {
					imported++
				}
			}
		}
	}

	// Import budgets.
	if raw, ok := payload["budgets"]; ok {
		var budgets []struct {
			ScopeKind   string `json:"scope_kind"`
			ScopeID     string `json:"scope_id"`
			LimitMicros int64  `json:"limit_micros"`
			Period      string `json:"period"`
			AlertPct    int    `json:"alert_pct"`
			HardCutoff  bool   `json:"hard_cutoff"`
		}
		if err := json.Unmarshal(raw, &budgets); err == nil {
			for _, b := range budgets {
				now := time.Now()
				budget := store.Budget{
					ID:          uuid.NewString(),
					TenantID:    adminTenant,
					ScopeKind:   store.BudgetScope(defaultStr(b.ScopeKind, string(store.ScopeTenant))),
					ScopeID:     defaultStr(b.ScopeID, adminTenant),
					LimitMicros: b.LimitMicros,
					Period:      defaultStr(b.Period, "monthly"),
					AlertPct:    defaultInt(b.AlertPct, 80),
					HardCutoff:  b.HardCutoff,
					CreatedAt:   now,
					UpdatedAt:   now,
				}
				if err := s.budgets.Create(ctx, budget); err == nil {
					imported++
				}
			}
		}
		if s.budgetEngine != nil {
			s.budgetEngine.InvalidateBudgetCache()
		}
	}

	// Import proxy pools.
	if raw, ok := payload["proxy_pools"]; ok {
		var pools []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Type     string `json:"type"`
			ProxyURL string `json:"proxy_url"`
			NoProxy  string `json:"no_proxy"`
			Strict   bool   `json:"strict"`
			IsActive *bool  `json:"is_active"`
		}
		if err := json.Unmarshal(raw, &pools); err == nil {
			for _, p := range pools {
				now := time.Now()
				pool := store.ProxyPool{
					ID:         defaultStr(p.ID, uuid.NewString()),
					Name:       p.Name,
					Type:       defaultStr(p.Type, "http"),
					ProxyURL:   p.ProxyURL,
					NoProxy:    p.NoProxy,
					Strict:     p.Strict,
					IsActive:   p.IsActive == nil || *p.IsActive,
					TestStatus: "unknown",
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				if err := s.pools.Create(ctx, pool); err == nil {
					imported++
				}
			}
		}
	}

	// Import endpoint settings.
	if raw, ok := payload["endpoint_settings"]; ok {
		if err := s.settings.Set(ctx, endpointSettingsKey, string(raw)); err == nil {
			imported++
		}
	}

	// Import aliases.
	if raw, ok := payload["aliases"]; ok {
		var aliases map[string]string
		if err := json.Unmarshal(raw, &aliases); err == nil {
			for alias, target := range aliases {
				_ = s.aliases.Set(ctx, alias, target)
				imported++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"imported": imported})
}

// ---- proxy test -------------------------------------------------------------

// validateAccountCredentials unseals an account's credentials and, if the
// connector implements core.Validator, probes the upstream to confirm they are
// accepted. Returns nil when validation passes or the connector does not support
// it. No-auth accounts still run connector probes when available so local
// endpoints such as Ollama/SearXNG can verify reachability.
//
// When the initial probe fails with an auth error and the account is OAuth,
// it retries once after forcing a token refresh (even if the token hasn't
// reached its local expiry — tokens can be invalidated server-side before
// expiry). A permanent refresh failure marks the account as needing
// reconnection.
func (s *Server) validateAccountCredentials(ctx context.Context, acc store.Account) error {
	if s.conns == nil || s.vault == nil {
		return nil // can't validate without registry + vault
	}
	// Skip validation for providers behind WAF/CDN that block probes.
	if spec, ok := connectors.SpecByID(acc.Provider); ok && spec.SkipValidation {
		return nil
	}
	conn, err := s.conns.Get(acc.Provider)
	if err != nil {
		return nil // provider has no connector; skip validation
	}
	v, ok := conn.(core.Validator)
	if !ok {
		return nil // connector doesn't support validation
	}
	// Refresh OAuth tokens if they are about to expire so the upstream probe
	// does not fail with a stale access token.
	if s.refresher != nil {
		if refreshed, err := s.refresher.EnsureFresh(ctx, acc); err == nil {
			acc = refreshed
		}
		// If refresh fails, fall through with the original account — Validate
		// will report the upstream error, which is more actionable.
	}
	creds, err := s.vault.Open(acc)
	if err != nil {
		return errors.New("could not decrypt credentials")
	}
	// Apply a reasonable timeout for the probe.
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	probeErr := v.Validate(probeCtx, creds)
	if probeErr == nil {
		return nil
	}

	// If validation failed with an auth error on an OAuth account, the token
	// may have been invalidated server-side before its local expiry. Force a
	// refresh retry and mark the account if the refresh is permanently dead.
	if acc.AuthKind == store.AuthOAuth && s.refresher != nil && s.accounts != nil {
		pe := core.AsProviderError(probeErr)
		if pe != nil && pe.Kind == core.ErrAuth {
			if refreshed, rerr := s.refresher.ForceRefresh(ctx, acc); rerr != nil {
				return probeErr // ForceRefresh already marks needs_reconnect if permanent
			} else {
				// Refresh succeeded — retry validation with the new token.
				newCreds, cerr := s.vault.Open(refreshed)
				if cerr == nil {
					probeCtx2, cancel2 := context.WithTimeout(ctx, 15*time.Second)
					defer cancel2()
					retryErr := v.Validate(probeCtx2, newCreds)
					if retryErr == nil {
						// Clear needs_reconnect if it was set.
						if acc.NeedsReconnect {
							_ = s.accounts.SetNeedsReconnect(ctx, acc.ID, false)
							s.warmProviderModelCacheAsync(acc.Provider)
						}
						return nil
					}
					// Retry still failed — mark for reconnect if it's an auth error.
					retryPE := core.AsProviderError(retryErr)
					if retryPE != nil && retryPE.Kind == core.ErrAuth {
						_ = s.accounts.SetNeedsReconnect(ctx, acc.ID, true)
						s.invalidateProviderModelCache(acc.Provider)
					}
					return retryErr
				}
			}
		}
	}

	return probeErr
}

// decodeJSON decodes a request body into v, writing a 400 on failure. It
// returns false when the caller should stop.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, http.ErrBodyReadAfterClose) {
			writeError(w, http.StatusBadRequest, "empty body")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func defaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func defaultBool(v, def bool) bool {
	return v || (!v && def)
}
