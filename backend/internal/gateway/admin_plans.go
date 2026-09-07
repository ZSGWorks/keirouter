package gateway

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mydisha/keirouter/backend/internal/store"
)

// mountAdmin registers the dashboard admin endpoints on the given router. These
// manage API keys, provider accounts, routing chains, budgets, and usage.

func (s *Server) adminListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.db.Plans().List(r.Context(), adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		keyCount, _ := s.db.Plans().CountKeys(r.Context(), p.ID)
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "description": p.Description,
			"limit_micros": p.LimitMicros, "limit_tokens": p.LimitTokens,
			"rpm_limit": p.RPMLimit, "tpm_limit": p.TPMLimit, "concurrency_limit": p.ConcurrencyLimit,
			"period": p.Period, "alert_pct": p.AlertPct, "hard_cutoff": p.HardCutoff,
			"allowed_models": store.GetPlanAllowedModels(p),
			"key_count":      keyCount,
			"created_at":     p.CreatedAt, "updated_at": p.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": out})
}

func (s *Server) adminCreatePlan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name             string   `json:"name"`
		Description      string   `json:"description"`
		LimitUSD         float64  `json:"limit_usd"`
		LimitTokens      int64    `json:"limit_tokens"`
		RPMLimit         int64    `json:"rpm_limit"`
		TPMLimit         int64    `json:"tpm_limit"`
		ConcurrencyLimit int64    `json:"concurrency_limit"`
		Period           string   `json:"period"`
		AlertPct         int      `json:"alert_pct"`
		HardCutoff       *bool    `json:"hard_cutoff"`
		AllowedModels    []string `json:"allowed_models"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.LimitUSD < 0 {
		writeError(w, http.StatusBadRequest, "limit_usd must not be negative")
		return
	}
	if body.LimitTokens < 0 {
		writeError(w, http.StatusBadRequest, "limit_tokens must not be negative")
		return
	}
	if body.RPMLimit < 0 || body.TPMLimit < 0 || body.ConcurrencyLimit < 0 {
		writeError(w, http.StatusBadRequest, "rate limits must not be negative")
		return
	}
	period, ok := normalizeBudgetPeriod(body.Period)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid period")
		return
	}
	alertPct := defaultInt(body.AlertPct, 80)
	if alertPct < 1 || alertPct > 100 {
		writeError(w, http.StatusBadRequest, "alert_pct must be between 1 and 100")
		return
	}
	hardCutoff := true
	if body.HardCutoff != nil {
		hardCutoff = *body.HardCutoff
	}

	now := time.Now()
	p := store.Plan{
		ID:               uuid.NewString(),
		TenantID:         adminTenant,
		Name:             body.Name,
		Description:      body.Description,
		LimitMicros:      int64(body.LimitUSD * 1_000_000),
		LimitTokens:      body.LimitTokens,
		RPMLimit:         body.RPMLimit,
		TPMLimit:         body.TPMLimit,
		ConcurrencyLimit: body.ConcurrencyLimit,
		Period:           period,
		AlertPct:         alertPct,
		HardCutoff:       hardCutoff,
		AllowedModels:    store.SetPlanAllowedModels(body.AllowedModels),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.db.Plans().Create(r.Context(), p); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": p.ID, "name": p.Name, "description": p.Description,
		"limit_micros": p.LimitMicros, "limit_tokens": p.LimitTokens,
		"rpm_limit": p.RPMLimit, "tpm_limit": p.TPMLimit, "concurrency_limit": p.ConcurrencyLimit,
		"period": p.Period, "alert_pct": p.AlertPct, "hard_cutoff": p.HardCutoff,
		"allowed_models": store.GetPlanAllowedModels(p),
	})
}

type planUpdateRequest struct {
	Name             *string  `json:"name"`
	Description      *string  `json:"description"`
	LimitUSD         *float64 `json:"limit_usd"`
	LimitTokens      *int64   `json:"limit_tokens"`
	RPMLimit         *int64   `json:"rpm_limit"`
	TPMLimit         *int64   `json:"tpm_limit"`
	ConcurrencyLimit *int64   `json:"concurrency_limit"`
	Period           *string  `json:"period"`
	AlertPct         *int     `json:"alert_pct"`
	HardCutoff       *bool    `json:"hard_cutoff"`
	AllowedModels    []string `json:"allowed_models"`
}

func (s *Server) adminUpdatePlan(w http.ResponseWriter, r *http.Request) {
	existing, err := s.db.Plans().Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}

	var body planUpdateRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	if message := applyPlanUpdate(&existing, body); message != "" {
		writeError(w, http.StatusBadRequest, message)
		return
	}
	existing.UpdatedAt = time.Now()

	if err := s.db.Plans().Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": existing.ID, "name": existing.Name, "description": existing.Description,
		"limit_micros": existing.LimitMicros, "limit_tokens": existing.LimitTokens,
		"rpm_limit": existing.RPMLimit, "tpm_limit": existing.TPMLimit, "concurrency_limit": existing.ConcurrencyLimit,
		"period": existing.Period, "alert_pct": existing.AlertPct, "hard_cutoff": existing.HardCutoff,
		"allowed_models": store.GetPlanAllowedModels(existing),
	})
}

func applyPlanUpdate(existing *store.Plan, body planUpdateRequest) string {
	if message := validatePlanUpdate(body); message != "" {
		return message
	}
	applyPlanUpdateFields(existing, body)
	return ""
}

// validatePlanUpdate returns the first invalid-field error message, or "" when
// every present field passes. Validation is separate from mutation so each pass
// stays small.
func validatePlanUpdate(body planUpdateRequest) string {
	if body.Name != nil && *body.Name == "" {
		return "name cannot be empty"
	}
	for _, invalid := range []struct {
		value *int64
		field string
	}{
		{body.LimitTokens, "limit_tokens"},
		{body.RPMLimit, "rpm_limit"},
		{body.TPMLimit, "tpm_limit"},
		{body.ConcurrencyLimit, "concurrency_limit"},
	} {
		if invalid.value != nil && *invalid.value < 0 {
			return fmt.Sprintf("%s must not be negative", invalid.field)
		}
	}
	if body.LimitUSD != nil && *body.LimitUSD < 0 {
		return "limit_usd must not be negative"
	}
	if body.Period != nil {
		if _, ok := normalizeBudgetPeriod(*body.Period); !ok {
			return "invalid period"
		}
	}
	if body.AlertPct != nil && (*body.AlertPct < 1 || *body.AlertPct > 100) {
		return "alert_pct must be between 1 and 100"
	}
	return ""
}

func applyPlanUpdateFields(existing *store.Plan, body planUpdateRequest) {
	if body.Name != nil {
		existing.Name = *body.Name
	}
	if body.Description != nil {
		existing.Description = *body.Description
	}
	if body.LimitUSD != nil {
		existing.LimitMicros = int64(*body.LimitUSD * 1_000_000)
	}
	if body.LimitTokens != nil {
		existing.LimitTokens = *body.LimitTokens
	}
	if body.RPMLimit != nil {
		existing.RPMLimit = *body.RPMLimit
	}
	if body.TPMLimit != nil {
		existing.TPMLimit = *body.TPMLimit
	}
	if body.ConcurrencyLimit != nil {
		existing.ConcurrencyLimit = *body.ConcurrencyLimit
	}
	if body.Period != nil {
		existing.Period, _ = normalizeBudgetPeriod(*body.Period)
	}
	if body.AlertPct != nil {
		existing.AlertPct = *body.AlertPct
	}
	if body.HardCutoff != nil {
		existing.HardCutoff = *body.HardCutoff
	}
	if body.AllowedModels != nil {
		existing.AllowedModels = store.SetPlanAllowedModels(body.AllowedModels)
	}
}

func (s *Server) adminDeletePlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	keyCount, err := s.db.Plans().CountKeys(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	if keyCount > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("plan has %d API key(s) assigned — reassign or delete them first", keyCount))
		return
	}
	if err := s.db.Plans().Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminListPlanKeys(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "id")
	if _, err := s.db.Plans().Get(r.Context(), planID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "plan not found")
			return
		}
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	keys, err := s.identity.List(r.Context(), adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	var out []map[string]any
	for _, k := range keys {
		if k.PlanID == planID {
			entry := map[string]any{
				"id": k.ID, "name": k.Name, "display": k.Display,
				"disabled": k.Disabled, "created_at": k.CreatedAt,
			}
			if models, merr := s.identity.Keys().GetAllowedModels(r.Context(), k.ID); merr == nil {
				entry["allowed_models"] = models
			}
			out = append(out, entry)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

// ---- budgets ----------------------------------------------------------------
