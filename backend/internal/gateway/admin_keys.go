package gateway

import (
	"errors"
	"net/http"
	"strings"
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

func (s *Server) adminListKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.identity.List(r.Context(), adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		entry := map[string]any{
			"id": k.ID, "name": k.Name, "display": k.Display,
			"disabled": k.Disabled, "plan_id": k.PlanID, "created_at": k.CreatedAt,
		}
		// Resolve plan name.
		if k.PlanID != "" {
			if plan, perr := s.db.Plans().Get(r.Context(), k.PlanID); perr == nil {
				entry["plan_name"] = plan.Name
			}
		}
		// Attach allowed models (empty = all allowed).
		if models, merr := s.identity.Keys().GetAllowedModels(r.Context(), k.ID); merr == nil {
			entry["allowed_models"] = models
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

func (s *Server) adminCreateKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string `json:"name"`
		ProjectID string `json:"project_id"`
		// Optional plan assignment. When set, the key inherits the plan's
		// budget rules (unless per-key overrides are also provided).
		PlanID string `json:"plan_id"`
		// Optional per-key budget overrides — these take precedence over plan
		// defaults when the key has a plan assigned.
		BudgetLimitUSD    *float64 `json:"budget_limit_usd"`
		BudgetLimitTokens *int64   `json:"budget_limit_tokens"`
		BudgetPeriod      string   `json:"budget_period"`
		BudgetAlertPct    *int     `json:"budget_alert_pct"`
		BudgetHardCutoff  *bool    `json:"budget_hard_cutoff"`
		// Optional per-key model access restriction (overrides plan models).
		AllowedModels []string `json:"allowed_models"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.BudgetLimitUSD != nil && *body.BudgetLimitUSD < 0 {
		writeError(w, http.StatusBadRequest, "budget_limit_usd must not be negative")
		return
	}
	if body.BudgetLimitTokens != nil && *body.BudgetLimitTokens < 0 {
		writeError(w, http.StatusBadRequest, "budget_limit_tokens must not be negative")
		return
	}

	// Resolve plan if one was specified.
	var plan *store.Plan
	if body.PlanID != "" {
		p, err := s.db.Plans().Get(r.Context(), body.PlanID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusBadRequest, "plan not found")
				return
			}
			writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
			return
		}
		plan = &p
	}

	// Generate key material (crypto operations, no DB write yet).
	issued, err := s.identity.Generate(adminTenant, body.ProjectID, body.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}

	// Set plan_id on the key record.
	if plan != nil {
		issued.Record.PlanID = plan.ID
	}

	// Determine effective budget: per-key overrides > plan defaults.
	hasPerKeyBudget := (body.BudgetLimitUSD != nil && *body.BudgetLimitUSD > 0) ||
		(body.BudgetLimitTokens != nil && *body.BudgetLimitTokens > 0)
	hasPlanBudget := plan != nil && (plan.LimitMicros > 0 || plan.LimitTokens > 0)
	hasBudget := hasPerKeyBudget || hasPlanBudget
	hasPerKeyModels := len(body.AllowedModels) > 0
	hasPlanModels := plan != nil && plan.AllowedModels != ""
	hasModels := hasPerKeyModels || hasPlanModels

	if !hasBudget && !hasModels && plan == nil {
		// Simple path: no budget, no models, no plan — insert key directly.
		if err := s.identity.CreateFromIssued(r.Context(), issued); err != nil {
			writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id": issued.Record.ID, "name": issued.Record.Name,
			"key": issued.Plaintext, "display": issued.Record.Display, "plan_id": issued.Record.PlanID,
		})
		return
	}

	// Transactional path: key + budget + model access atomically.
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction start failed")
		return
	}
	defer func() { _ = tx.Rollback() }() // no-op after commit

	if err := s.identity.Keys().CreateOnTx(r.Context(), tx, issued.Record); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}

	var budgetRec store.Budget
	if hasBudget {
		// Resolve effective values: per-key overrides win, then plan, then defaults.
		hardCutoff := true
		alertPct := 80
		period := "monthly"
		var limitMicros int64
		var limitTokens int64

		if plan != nil {
			hardCutoff = plan.HardCutoff
			alertPct = plan.AlertPct
			period = plan.Period
			limitMicros = plan.LimitMicros
			limitTokens = plan.LimitTokens
		}

		// Per-key overrides take precedence.
		if body.BudgetHardCutoff != nil {
			hardCutoff = *body.BudgetHardCutoff
		}
		if body.BudgetAlertPct != nil {
			alertPct = *body.BudgetAlertPct
		}
		if body.BudgetPeriod != "" {
			if p, ok := normalizeBudgetPeriod(body.BudgetPeriod); ok {
				period = p
			}
		}
		if body.BudgetLimitUSD != nil && *body.BudgetLimitUSD > 0 {
			limitMicros = int64(*body.BudgetLimitUSD * 1_000_000)
		}
		if body.BudgetLimitTokens != nil && *body.BudgetLimitTokens > 0 {
			limitTokens = *body.BudgetLimitTokens
		}

		if limitMicros <= 0 && limitTokens <= 0 {
			// Plan had no limits and no per-key overrides — skip budget creation.
			hasBudget = false
		} else {
			if alertPct < 1 || alertPct > 100 {
				writeError(w, http.StatusBadRequest, "budget_alert_pct must be between 1 and 100")
				return
			}

			now := time.Now()
			budgetRec = store.Budget{
				ID:          uuid.NewString(),
				TenantID:    adminTenant,
				ScopeKind:   store.ScopeAPIKey,
				ScopeID:     issued.Record.ID,
				LimitMicros: limitMicros,
				LimitTokens: limitTokens,
				Period:      period,
				AlertPct:    alertPct,
				HardCutoff:  hardCutoff,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := s.budgets.CreateOnTx(r.Context(), tx, budgetRec); err != nil {
				writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
				return
			}
		}
	}

	if hasModels {
		// Per-key models take precedence over plan models.
		effectiveModels := body.AllowedModels
		if !hasPerKeyModels && plan != nil {
			effectiveModels = store.GetPlanAllowedModels(*plan)
		}
		if len(effectiveModels) > 0 {
			if err := s.identity.Keys().SetAllowedModelsOnTx(r.Context(), tx, issued.Record.ID, effectiveModels); err != nil {
				writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "transaction commit failed")
		return
	}

	// Invalidate the budget definition cache so the next request picks up
	// the newly-created budget immediately.
	if hasBudget && s.budgetEngine != nil {
		s.budgetEngine.InvalidateBudgetCache()
	}

	resp := map[string]any{
		"id": issued.Record.ID, "name": issued.Record.Name,
		"key": issued.Plaintext, "display": issued.Record.Display, "plan_id": issued.Record.PlanID,
	}
	if hasBudget {
		resp["budget"] = map[string]any{
			"id": budgetRec.ID, "scope_kind": string(budgetRec.ScopeKind),
			"limit_micros": budgetRec.LimitMicros, "limit_tokens": budgetRec.LimitTokens,
			"period": budgetRec.Period, "alert_pct": budgetRec.AlertPct, "hard_cutoff": budgetRec.HardCutoff,
		}
	}
	effectiveModels := body.AllowedModels
	if !hasPerKeyModels && plan != nil {
		effectiveModels = store.GetPlanAllowedModels(*plan)
	}
	if len(effectiveModels) > 0 {
		resp["allowed_models"] = effectiveModels
	}
	if plan != nil {
		resp["plan"] = map[string]any{
			"id": plan.ID, "name": plan.Name,
		}
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) adminDeleteKey(w http.ResponseWriter, r *http.Request) {
	if err := s.identity.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminUpdateKey toggles a key's disabled state and/or updates its model access.
func (s *Server) adminUpdateKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Disabled      *bool    `json:"disabled"`
		AllowedModels []string `json:"allowed_models"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Disabled == nil && body.AllowedModels == nil {
		writeError(w, http.StatusBadRequest, "disabled or allowed_models field is required")
		return
	}
	if body.Disabled != nil {
		if err := s.identity.SetDisabled(r.Context(), id, *body.Disabled); err != nil {
			writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
			return
		}
	}
	if body.AllowedModels != nil {
		if err := s.identity.Keys().SetAllowedModels(r.Context(), id, body.AllowedModels); err != nil {
			writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "disabled": body.Disabled, "allowed_models": body.AllowedModels})
}

// ---- accounts ---------------------------------------------------------------

func (s *Server) adminValidateKey(w http.ResponseWriter, r *http.Request) {
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
	authKind := accountAuthKind(spec, body.APIKey)
	if authKind != store.AuthNone && strings.TrimSpace(body.APIKey) == "" {
		writeError(w, http.StatusBadRequest, "provider and api_key are required")
		return
	}
	if s.vault == nil || s.conns == nil {
		writeError(w, http.StatusInternalServerError, "vault or connectors not configured")
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
		writeJSON(w, http.StatusOK, map[string]any{"status": "error", "message": err.Error()})
		return
	}

	// Build a temporary in-memory account without persisting.
	acc := store.Account{
		ID:       "validate-temp",
		Provider: body.Provider,
		AuthKind: authKind,
	}
	if err := s.vault.Seal(&acc, vault.NewSecret{APIKey: body.APIKey, Metadata: meta}); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "vault seal failed"))
		return
	}

	if verr := s.validateAccountCredentials(r.Context(), acc); verr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "error", "message": sanitizeProviderError(s.log, verr, "credential validation failed")})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
