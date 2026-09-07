package gateway

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mydisha/keirouter/backend/internal/budget"
	"github.com/mydisha/keirouter/backend/internal/store"
)

// mountAdmin registers the dashboard admin endpoints on the given router. These
// manage API keys, provider accounts, routing chains, budgets, and usage.

func (s *Server) adminListBudgets(w http.ResponseWriter, r *http.Request) {
	budgets, err := s.budgets.ListByTenant(r.Context(), adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make([]map[string]any, 0, len(budgets))
	for _, b := range budgets {
		out = append(out, map[string]any{
			"id": b.ID, "scope_kind": b.ScopeKind, "scope_id": b.ScopeID,
			"limit_micros": b.LimitMicros, "limit_tokens": b.LimitTokens,
			"period": b.Period, "alert_pct": b.AlertPct, "hard_cutoff": b.HardCutoff,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"budgets": out})
}

func (s *Server) adminCreateBudget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ScopeKind   string  `json:"scope_kind"`
		ScopeID     string  `json:"scope_id"`
		LimitUSD    float64 `json:"limit_usd"`
		LimitTokens int64   `json:"limit_tokens"`
		Period      string  `json:"period"`
		AlertPct    int     `json:"alert_pct"`
		HardCutoff  *bool   `json:"hard_cutoff"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.LimitUSD <= 0 && body.LimitTokens <= 0 {
		writeError(w, http.StatusBadRequest, "limit_usd or limit_tokens must be positive")
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
	period, ok := normalizeBudgetPeriod(body.Period)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid budget period")
		return
	}
	alertPct := defaultInt(body.AlertPct, 80)
	if alertPct < 1 || alertPct > 100 {
		writeError(w, http.StatusBadRequest, "alert_pct must be between 1 and 100")
		return
	}
	scopeKind := store.BudgetScope(defaultStr(body.ScopeKind, string(store.ScopeTenant)))
	scopeID := strings.TrimSpace(body.ScopeID)
	switch scopeKind {
	case store.ScopeTenant:
		scopeID = defaultStr(scopeID, adminTenant)
	case store.ScopeAPIKey:
		if scopeID == "" {
			writeError(w, http.StatusBadRequest, "scope_id is required for api_key budgets")
			return
		}
		if _, err := s.identity.Get(r.Context(), scopeID); err != nil {
			writeError(w, http.StatusBadRequest, "api key not found")
			return
		}
	case store.ScopeProject:
		if scopeID == "" {
			writeError(w, http.StatusBadRequest, "scope_id is required for project budgets")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid budget scope")
		return
	}
	hardCutoff := true
	if body.HardCutoff != nil {
		hardCutoff = *body.HardCutoff
	}

	now := time.Now()
	b := store.Budget{
		ID:          uuid.NewString(),
		TenantID:    adminTenant,
		ScopeKind:   scopeKind,
		ScopeID:     scopeID,
		LimitMicros: int64(body.LimitUSD * 1_000_000),
		LimitTokens: body.LimitTokens,
		Period:      period,
		AlertPct:    alertPct,
		HardCutoff:  hardCutoff,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.budgets.Create(r.Context(), b); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": b.ID})
	if s.budgetEngine != nil {
		s.budgetEngine.InvalidateBudgetCache()
	}
}

func (s *Server) adminDeleteBudget(w http.ResponseWriter, r *http.Request) {
	if err := s.budgets.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
	if s.budgetEngine != nil {
		s.budgetEngine.InvalidateBudgetCache()
	}
}

func (s *Server) adminUpdateBudget(w http.ResponseWriter, r *http.Request) {
	existing, err := s.budgets.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "budget not found")
			return
		}
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}

	var body struct {
		LimitUSD    *float64 `json:"limit_usd"`
		LimitTokens *int64   `json:"limit_tokens"`
		Period      *string  `json:"period"`
		AlertPct    *int     `json:"alert_pct"`
		HardCutoff  *bool    `json:"hard_cutoff"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if body.LimitUSD != nil {
		if *body.LimitUSD < 0 {
			writeError(w, http.StatusBadRequest, "limit_usd must not be negative")
			return
		}
		existing.LimitMicros = int64(*body.LimitUSD * 1_000_000)
	}
	if body.LimitTokens != nil {
		if *body.LimitTokens < 0 {
			writeError(w, http.StatusBadRequest, "limit_tokens must not be negative")
			return
		}
		existing.LimitTokens = *body.LimitTokens
	}
	if body.Period != nil {
		period, ok := normalizeBudgetPeriod(*body.Period)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid budget period")
			return
		}
		existing.Period = period
	}
	if body.AlertPct != nil {
		if *body.AlertPct < 1 || *body.AlertPct > 100 {
			writeError(w, http.StatusBadRequest, "alert_pct must be between 1 and 100")
			return
		}
		existing.AlertPct = *body.AlertPct
	}
	if body.HardCutoff != nil {
		existing.HardCutoff = *body.HardCutoff
	}
	if existing.LimitMicros <= 0 && existing.LimitTokens <= 0 {
		writeError(w, http.StatusBadRequest, "limit_usd or limit_tokens must be positive")
		return
	}
	existing.UpdatedAt = time.Now()

	if err := s.budgets.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
	if s.budgetEngine != nil {
		s.budgetEngine.InvalidateBudgetCache()
	}
}

func normalizeBudgetPeriod(period string) (string, bool) {
	period = defaultStr(period, "monthly")
	switch period {
	case "daily", "weekly", "monthly", "total":
		return period, true
	default:
		return "", false
	}
}

// adminBudgetStatus returns all budgets enriched with current-period spend data.
func (s *Server) adminBudgetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	budgets, err := s.budgets.ListByTenant(ctx, adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}

	now := time.Now()
	scopes := make([]store.SpendScope, 0, len(budgets))
	sinceByBudget := make([]time.Time, len(budgets))
	for i, b := range budgets {
		since := budget.PeriodStart(b.Period, now)
		sinceByBudget[i] = since
		scopes = append(scopes, store.SpendScope{Kind: b.ScopeKind, ScopeID: b.ScopeID, Since: since})
	}
	spendResults, err := s.usage.SpendAndTokensBatch(ctx, scopes)
	if err != nil {
		s.log.Error("budget status: batch spend lookup failed", "err", err)
		spendResults = make([]store.SpendResult, len(budgets))
	}

	out := make([]map[string]any, 0, len(budgets))
	for i, b := range budgets {
		since := sinceByBudget[i]
		var spent, tokens int64
		if i < len(spendResults) {
			spent = spendResults[i].CostMicros
			tokens = spendResults[i].Tokens
		}

		pctUsed := 0.0
		if b.LimitMicros > 0 {
			pctUsed = float64(spent) / float64(b.LimitMicros) * 100
		}
		tokPctUsed := 0.0
		if b.LimitTokens > 0 {
			tokPctUsed = float64(tokens) / float64(b.LimitTokens) * 100
		}

		// Resolve scope display name.
		scopeName := string(b.ScopeKind)
		if b.ScopeKind == store.ScopeAPIKey {
			if key, kerr := s.identity.Get(ctx, b.ScopeID); kerr == nil && key.Name != "" {
				scopeName = key.Name
			}
		}

		out = append(out, map[string]any{
			"id":              b.ID,
			"scope_kind":      b.ScopeKind,
			"scope_id":        b.ScopeID,
			"scope_name":      scopeName,
			"limit_micros":    b.LimitMicros,
			"limit_tokens":    b.LimitTokens,
			"period":          b.Period,
			"alert_pct":       b.AlertPct,
			"hard_cutoff":     b.HardCutoff,
			"spent_micros":    spent,
			"spent_tokens":    tokens,
			"pct_used":        pctUsed,
			"tokens_pct_used": tokPctUsed,
			"period_start":    since,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"budgets": out})
}

// ---- usage ------------------------------------------------------------------

func (s *Server) adminUsageSummary(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	since := time.Now().AddDate(0, 0, -30)
	switch period {
	case "today":
		now := time.Now()
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "week":
		since = time.Now().AddDate(0, 0, -7)
	case "month", "":
		since = time.Now().AddDate(0, -1, 0)
	}

	sum, err := s.usage.Summarize(r.Context(), adminTenant, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_requests":     sum.TotalRequests,
		"prompt_tokens":      sum.PromptTokens,
		"completion_tokens":  sum.CompletionTokens,
		"cached_tokens":      sum.CachedTokens,
		"cache_write_tokens": sum.CacheWriteTokens,
		"cost_usd":           float64(sum.CostMicros) / 1_000_000,
		"cache_hits":         sum.CacheHits,
		"since":              since,
	})
}

// ---- model aliases ----------------------------------------------------------
