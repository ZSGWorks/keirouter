package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/httputil"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/vault"
)

// mountAdmin registers the dashboard admin endpoints on the given router. These
// manage API keys, provider accounts, routing chains, budgets, and usage.

func (s *Server) adminListAccounts(w http.ResponseWriter, r *http.Request) {
	accs, err := s.accounts.ListByTenant(r.Context(), adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make([]map[string]any, 0, len(accs))
	for _, a := range accs {
		// Never expose secret material.
		out = append(out, map[string]any{
			"id": a.ID, "provider": a.Provider, "label": a.Label,
			"auth_kind": a.AuthKind, "priority": a.Priority,
			"disabled": a.Disabled, "proxy_pool_id": a.ProxyPoolID,
			"needs_reconnect":   a.NeedsReconnect,
			"credits_exhausted": a.CreditsExhausted,
			"created_at":        a.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out})
}

func (s *Server) adminCreateAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider          string `json:"provider"`
		Label             string `json:"label"`
		APIKey            string `json:"api_key"`
		BaseURL           string `json:"base_url"`
		Region            string `json:"region"`
		AccountID         string `json:"account_id"`
		AzureEndpoint     string `json:"azure_endpoint"`
		AzureDeployment   string `json:"azure_deployment"`
		AzureAPIVersion   string `json:"azure_api_version"`
		AzureOrganization string `json:"azure_organization"`
		ProxyPoolID       string `json:"proxy_pool_id"`
		Priority          int    `json:"priority"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	spec, ok := connectors.SpecByID(body.Provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown provider: "+body.Provider)
		return
	}
	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not configured")
		return
	}
	if err := s.validateProxyPoolID(r.Context(), body.ProxyPoolID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	authKind := accountAuthKind(spec, body.APIKey)
	if authKind != store.AuthNone && strings.TrimSpace(body.APIKey) == "" {
		writeError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	// SSRF Protection: Validate base_url before use
	if body.BaseURL != "" {
		if err := httputil.ValidateBaseURL(body.BaseURL); err != nil {
			s.log.Warn("blocked suspicious base_url", "url", body.BaseURL, "error", err)
			writeError(w, http.StatusBadRequest, "invalid base_url: URL blocked by security policy")
			return
		}
	}
	if body.AzureEndpoint != "" {
		if err := httputil.ValidateBaseURL(body.AzureEndpoint); err != nil {
			s.log.Warn("blocked suspicious azure_endpoint", "url", body.AzureEndpoint, "error", err)
			writeError(w, http.StatusBadRequest, "invalid azure_endpoint: URL blocked by security policy")
			return
		}
	}
	meta, err := providerAccountMetadata(spec, providerMetadataInput{
		BaseURL:           body.BaseURL,
		Region:            body.Region,
		AccountID:         body.AccountID,
		AzureEndpoint:     body.AzureEndpoint,
		AzureDeployment:   body.AzureDeployment,
		AzureAPIVersion:   body.AzureAPIVersion,
		AzureOrganization: body.AzureOrganization,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now()
	label := strings.TrimSpace(body.Label)
	if label == "" {
		label = spec.DisplayName
	}
	acc := store.Account{
		ID:        uuid.NewString(),
		TenantID:  adminTenant,
		Provider:  body.Provider,
		Label:     label,
		AuthKind:  authKind,
		Priority:  defaultInt(body.Priority, 100),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if body.ProxyPoolID != "" {
		acc.ProxyPoolID = strings.TrimSpace(body.ProxyPoolID)
	}
	if err := s.vault.Seal(&acc, vault.NewSecret{APIKey: body.APIKey, Metadata: meta}); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "vault seal failed"))
		return
	}

	// Validate credentials against the upstream before persisting.
	if verr := s.validateAccountCredentials(r.Context(), acc); verr != nil {
		writeError(w, http.StatusBadRequest, sanitizeError(s.log, verr, "credential validation failed"))
		return
	}

	if err := s.accounts.Create(r.Context(), acc); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "account creation failed"))
		return
	}
	s.warmProviderModelCacheAsync(acc.Provider)
	writeJSON(w, http.StatusCreated, map[string]any{"id": acc.ID, "provider": acc.Provider, "label": acc.Label})
}

// bulkMaxItems caps the number of credentials accepted in a single bulk import
// to bound memory, upstream validation fan-out, and DB write time.
const bulkMaxItems = 1000

// bulkValidateConcurrency bounds how many upstream credential probes run at
// once during a bulk import with validation enabled.
const bulkValidateConcurrency = 6

type bulkAccountItem struct {
	Label   string `json:"label"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

type bulkAccountsRequest struct {
	Provider string `json:"provider"`
	// Shared settings applied to every item unless an item overrides them.
	BaseURL           string `json:"base_url"`
	Region            string `json:"region"`
	AccountID         string `json:"account_id"`
	AzureEndpoint     string `json:"azure_endpoint"`
	AzureDeployment   string `json:"azure_deployment"`
	AzureAPIVersion   string `json:"azure_api_version"`
	AzureOrganization string `json:"azure_organization"`
	Priority          int    `json:"priority"`
	ProxyPoolID       string `json:"proxy_pool_id"`
	// Validate probes each credential against the upstream before persisting.
	// Off by default for bulk to avoid slow imports and upstream rate limits.
	Validate bool              `json:"validate"`
	Items    []bulkAccountItem `json:"items"`
}

type bulkAccountResult struct {
	Index  int    `json:"index"`
	Label  string `json:"label"`
	Status string `json:"status"` // created | error | skipped
	ID     string `json:"id,omitempty"`
	Error  string `json:"error,omitempty"`
}

// adminBulkCreateAccounts imports many provider credentials in one request. It
// reuses the same sealing, metadata, validation, and persistence path as the
// single-create handler, but reports a per-item outcome so partial failures
// don't abort the whole batch. Upstream validation (when enabled) runs with a
// bounded worker pool; DB writes are serialized to stay friendly to SQLite.
func (s *Server) adminBulkCreateAccounts(w http.ResponseWriter, r *http.Request) {
	var body bulkAccountsRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	spec, ok := connectors.SpecByID(body.Provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown provider: "+body.Provider)
		return
	}
	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not configured")
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items is required")
		return
	}
	if len(body.Items) > bulkMaxItems {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("too many items: %d (max %d)", len(body.Items), bulkMaxItems))
		return
	}
	if err := s.validateProxyPoolID(r.Context(), body.ProxyPoolID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// SSRF Protection: validate the shared base/endpoint URLs once up front.
	if body.BaseURL != "" {
		if err := httputil.ValidateBaseURL(body.BaseURL); err != nil {
			s.log.Warn("blocked suspicious base_url", "url", body.BaseURL, "error", err)
			writeError(w, http.StatusBadRequest, "invalid base_url: URL blocked by security policy")
			return
		}
	}
	if body.AzureEndpoint != "" {
		if err := httputil.ValidateBaseURL(body.AzureEndpoint); err != nil {
			s.log.Warn("blocked suspicious azure_endpoint", "url", body.AzureEndpoint, "error", err)
			writeError(w, http.StatusBadRequest, "invalid azure_endpoint: URL blocked by security policy")
			return
		}
	}

	results := make([]bulkAccountResult, len(body.Items))
	var (
		seen    = map[string]struct{}{} // de-dup api keys within the batch
		seenMu  sync.Mutex
		writeMu sync.Mutex // serialize DB writes (SQLite-friendly)
		sem     = make(chan struct{}, bulkValidateConcurrency)
		wg      sync.WaitGroup
	)

	for i, item := range body.Items {
		label := strings.TrimSpace(item.Label)
		key := strings.TrimSpace(item.APIKey)
		results[i] = bulkAccountResult{Index: i, Label: label}

		authKind := accountAuthKind(spec, key)
		if authKind != store.AuthNone && key == "" {
			results[i].Status = "error"
			results[i].Error = "api_key is required"
			continue
		}

		// De-duplicate identical keys within the same batch.
		if key != "" {
			seenMu.Lock()
			if _, dup := seen[key]; dup {
				seenMu.Unlock()
				results[i].Status = "skipped"
				results[i].Error = "duplicate api key in batch"
				continue
			}
			seen[key] = struct{}{}
			seenMu.Unlock()
		}

		// Per-item base URL overrides the shared one when present.
		baseURL := strings.TrimSpace(item.BaseURL)
		if baseURL == "" {
			baseURL = body.BaseURL
		}
		if baseURL != "" {
			if err := httputil.ValidateBaseURL(baseURL); err != nil {
				results[i].Status = "error"
				results[i].Error = "invalid base_url: URL blocked by security policy"
				continue
			}
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(i int, label, key, baseURL string, authKind store.AuthKind) {
			defer wg.Done()
			defer func() { <-sem }()

			meta, err := providerAccountMetadata(spec, providerMetadataInput{
				BaseURL:           baseURL,
				Region:            body.Region,
				AccountID:         body.AccountID,
				AzureEndpoint:     body.AzureEndpoint,
				AzureDeployment:   body.AzureDeployment,
				AzureAPIVersion:   body.AzureAPIVersion,
				AzureOrganization: body.AzureOrganization,
			})
			if err != nil {
				results[i].Status = "error"
				results[i].Error = err.Error()
				return
			}

			now := time.Now()
			displayLabel := label
			if displayLabel == "" {
				displayLabel = fmt.Sprintf("%s-%d", spec.DisplayName, i+1)
			}
			acc := store.Account{
				ID:        uuid.NewString(),
				TenantID:  adminTenant,
				Provider:  body.Provider,
				Label:     displayLabel,
				AuthKind:  authKind,
				Priority:  defaultInt(body.Priority, 100),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if body.ProxyPoolID != "" {
				acc.ProxyPoolID = strings.TrimSpace(body.ProxyPoolID)
			}
			if err := s.vault.Seal(&acc, vault.NewSecret{APIKey: key, Metadata: meta}); err != nil {
				results[i].Status = "error"
				results[i].Error = "vault seal failed"
				return
			}

			if body.Validate {
				if verr := s.validateAccountCredentials(r.Context(), acc); verr != nil {
					results[i].Status = "error"
					results[i].Error = sanitizeError(s.log, verr, "credential validation failed")
					return
				}
			}

			writeMu.Lock()
			err = s.accounts.Create(r.Context(), acc)
			writeMu.Unlock()
			if err != nil {
				results[i].Status = "error"
				results[i].Error = sanitizeError(s.log, err, "account creation failed")
				return
			}
			results[i].Status = "created"
			results[i].ID = acc.ID
			results[i].Label = displayLabel
		}(i, label, key, baseURL, authKind)
	}

	wg.Wait()

	sort.Slice(results, func(a, b int) bool { return results[a].Index < results[b].Index })
	var created, failed, skipped int
	for _, res := range results {
		switch res.Status {
		case "created":
			created++
		case "skipped":
			skipped++
		default:
			failed++
		}
	}

	s.warmProviderModelCacheAsync(body.Provider)

	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(results),
		"created": created,
		"failed":  failed,
		"skipped": skipped,
		"results": results,
	})
}

func (s *Server) adminDeleteAccount(w http.ResponseWriter, r *http.Request) {
	acc, getErr := s.accounts.Get(r.Context(), chi.URLParam(r, "id"))
	if getErr != nil {
		s.log.Warn("failed to fetch account before delete; model cache invalidation may be skipped", "id", chi.URLParam(r, "id"), "err", getErr)
	}
	if err := s.accounts.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	if acc.Provider != "" {
		s.invalidateProviderModelCache(acc.Provider)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminUpdateAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := s.accounts.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	var body struct {
		Label       *string `json:"label"`
		Priority    *int    `json:"priority"`
		Disabled    *bool   `json:"disabled"`
		ProxyPoolID *string `json:"proxy_pool_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Label != nil {
		acc.Label = *body.Label
	}
	if body.Priority != nil {
		acc.Priority = *body.Priority
	}
	if body.Disabled != nil {
		acc.Disabled = *body.Disabled
	}
	if body.ProxyPoolID != nil {
		poolID := strings.TrimSpace(*body.ProxyPoolID)
		if err := s.validateProxyPoolID(r.Context(), poolID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		acc.ProxyPoolID = poolID
	}
	if err := s.accounts.Update(r.Context(), acc); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	s.invalidateModelCacheIfDisabled(body.Disabled, acc.Provider)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": acc.ID, "provider": acc.Provider, "label": acc.Label,
		"priority": acc.Priority, "disabled": acc.Disabled,
		"proxy_pool_id": acc.ProxyPoolID,
	})
}

func (s *Server) validateProxyPoolID(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if s.pools == nil {
		return fmt.Errorf("proxy pools not configured")
	}
	if _, err := s.pools.Get(ctx, id); err != nil {
		return fmt.Errorf("proxy pool not found")
	}
	return nil
}

func (s *Server) adminTestAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := s.accounts.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	verr := s.validateAccountCredentials(r.Context(), acc)
	if verr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       acc.ID,
			"provider": acc.Provider,
			"label":    acc.Label,
			"status":   "error",
			"message":  sanitizeProviderError(s.log, verr, "account validation failed"),
		})
		return
	}
	// Validation passed: clear needs_reconnect if it was flagged, since a
	// successful probe means the current credentials are accepted.
	if acc.NeedsReconnect {
		if err := s.accounts.SetNeedsReconnect(r.Context(), acc.ID, false); err != nil {
			s.log.Warn("failed to clear needs_reconnect after successful test", "account", acc.ID, "err", err)
		} else {
			s.warmProviderModelCacheAsync(acc.Provider)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       acc.ID,
		"provider": acc.Provider,
		"label":    acc.Label,
		"status":   "ok",
	})
}

// adminAccountQuota fetches upstream quota/credit info for a specific account.
func (s *Server) adminAccountQuota(w http.ResponseWriter, r *http.Request) {
	acc, err := s.accounts.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	qs := connectors.GetQuotaSource(acc.Provider)
	if qs == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"provider":  acc.Provider,
			"supported": false,
			"message":   "Upstream quota not available for this provider.",
		})
		return
	}

	if s.vault == nil {
		writeError(w, http.StatusInternalServerError, "vault not configured")
		return
	}

	// Quota endpoints are often called after the page has been idle for a while.
	// Refresh OAuth credentials first so one stale account does not lose its
	// usage panel while the rest of the provider page continues to work.
	quotaAcc := acc
	if s.refresher != nil {
		if refreshed, refreshErr := s.refresher.EnsureFresh(r.Context(), acc); refreshErr == nil {
			quotaAcc = refreshed
		}
	}

	creds, err := s.vault.Open(quotaAcc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not decrypt credentials")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	quota, qerr := qs.FetchQuota(ctx, creds)
	if qerr != nil {
		writeError(w, http.StatusBadGateway, sanitizeProviderError(s.log, qerr, "quota request failed"))
		return
	}

	var quotas []map[string]any
	for _, q := range quota.Quotas {
		quotas = append(quotas, map[string]any{
			"resource_type": q.ResourceType,
			"used":          q.Used,
			"limit":         q.Limit,
			"remaining":     q.Remaining,
			"reset_at":      q.ResetAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider":  acc.Provider,
		"supported": true,
		"plan_name": quota.PlanName,
		"message":   "Quota information retrieved from upstream provider.",
		"quotas":    quotas,
	})
}

func (s *Server) adminTestProxy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProxyURL string `json:"proxyUrl"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ProxyURL == "" {
		writeError(w, http.StatusBadRequest, "proxyUrl is required")
		return
	}

	// Validate proxy URL syntax only — proxy URLs are admin-configured trusted
	// infrastructure, so SSRF restrictions (which guard outbound target URLs)
	// do not apply here. Localhost proxies (Clash, V2Ray, etc.) are expected.
	parsed, err := url.Parse(body.ProxyURL)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "invalid proxy URL: " + err.Error()})
		return
	}
	if parsed.Host == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "invalid proxy URL: host is required"})
		return
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "socks5" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "unsupported proxy scheme: " + parsed.Scheme})
		return
	}

	start := time.Now()
	transport := &http.Transport{Proxy: http.ProxyURL(parsed)}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://httpbin.org/ip", nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	result := map[string]any{
		"ok":        resp.StatusCode < 400,
		"status":    resp.StatusCode,
		"elapsedMs": elapsed.Milliseconds(),
	}

	// Parse exit IP from httpbin.org/ip response body.
	if resp.StatusCode < 400 {
		var ipInfo struct {
			Origin string `json:"origin"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ipInfo); err == nil && ipInfo.Origin != "" {
			result["exitIP"] = ipInfo.Origin
		}
	} else {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if len(errBody) > 0 {
			result["error"] = string(errBody)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// ---- helpers ----------------------------------------------------------------

type providerMetadataInput struct {
	BaseURL           string
	Region            string
	AccountID         string
	AzureEndpoint     string
	AzureDeployment   string
	AzureAPIVersion   string
	AzureOrganization string
}

func accountAuthKind(spec connectors.ProviderSpec, apiKey string) store.AuthKind {
	if strings.TrimSpace(apiKey) == "" && spec.AuthKind == "none" {
		return store.AuthNone
	}
	return store.AuthAPIKey
}

func providerAccountMetadata(spec connectors.ProviderSpec, in providerMetadataInput) (map[string]string, error) {
	meta := map[string]string{}

	baseURL := strings.TrimSpace(in.BaseURL)
	if in.Region != "" {
		meta["region"] = strings.TrimSpace(in.Region)
		if resolved := connectors.ResolveRegionBaseURL(spec.ID, in.Region); resolved != "" {
			baseURL = resolved
		}
	}
	if spec.BaseURL == "" && spec.ID != "azure" && baseURL == "" {
		return nil, fmt.Errorf("base_url is required for %s", spec.DisplayName)
	}
	if baseURL != "" {
		meta["base_url"] = baseURL
	}

	switch spec.ID {
	case "cloudflare-ai":
		accountID := strings.TrimSpace(in.AccountID)
		if accountID == "" {
			return nil, errors.New("account_id is required for Cloudflare Workers AI")
		}
		// OpenAICompatible resolves {accountId} placeholders from Extra.
		meta["accountId"] = accountID
	case "qoder":
		// API-key connections to Qoder use a Personal Access Token (pt-*),
		// which the connector exchanges for a short-lived COSY job token. Mark
		// the auth method and mint a stable machine id for the COSY envelope.
		meta["qoder_auth_method"] = "pat"
		meta["machine_id"] = uuid.NewString()
	case "azure":
		endpoint := strings.TrimRight(strings.TrimSpace(in.AzureEndpoint), "/")
		deployment := strings.TrimSpace(in.AzureDeployment)
		if endpoint == "" {
			return nil, errors.New("azure_endpoint is required for Azure OpenAI")
		}
		if deployment == "" {
			return nil, errors.New("azure_deployment is required for Azure OpenAI")
		}
		meta["azure_endpoint"] = endpoint
		meta["deployment"] = deployment
		if v := strings.TrimSpace(in.AzureAPIVersion); v != "" {
			meta["api_version"] = v
		}
		if v := strings.TrimSpace(in.AzureOrganization); v != "" {
			meta["organization"] = v
		}
	}

	return meta, nil
}
