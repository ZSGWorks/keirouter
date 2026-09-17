package gateway

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/store"
)

// adminResetAccountCooldown clears dispatcher cooldown state for one account:
// the account-level cooldown, backoff level, and credits-exhausted flag plus
// all its active model-level cooldowns. Both writes run in one transaction so
// a mid-reset failure leaves nothing applied. Health history, probe results,
// reconnect flags, and disabled state are untouched. Idempotent: resetting an
// unparked account succeeds with zero counts.
func (s *Server) adminResetAccountCooldown(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc, err := s.accounts.Get(ctx, chi.URLParam(r, "id"))
	if err != nil || acc.TenantID != adminTenant {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	var clearedAccounts, clearedModels int64
	txErr := s.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		clearedAccounts, err = s.accounts.ClearAccountCooldownsOnTx(ctx, tx, acc.ID)
		if err != nil {
			return err
		}
		clearedModels, err = s.db.Routing().ClearAccountModelCooldownsOnTx(ctx, tx, acc.ID)
		return err
	})
	if txErr != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, txErr, "internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cleared_accounts": clearedAccounts,
		"cleared_models":   clearedModels,
	})
}

// adminResetTenantCooldowns clears dispatcher cooldown state for every parked
// account of the tenant plus all active model-level cooldowns on tenant
// accounts, in one transaction. Same untouched guarantees and idempotency as
// the per-account reset.
func (s *Server) adminResetTenantCooldowns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var clearedAccounts, clearedModels int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var err error
		clearedAccounts, err = s.accounts.ClearTenantCooldownsOnTx(ctx, tx, adminTenant)
		if err != nil {
			return err
		}
		clearedModels, err = s.db.Routing().ClearTenantModelCooldownsOnTx(ctx, tx, adminTenant)
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cleared_accounts": clearedAccounts,
		"cleared_models":   clearedModels,
	})
}

// Cooldown scopes reported to the Overview card.
const (
	cooldownScopeAccount = "account"
	cooldownScopeModel   = "model"
	cooldownScopeBoth    = "both"
)

// allModelsSentinel is the model_cooldowns value used for an account-wide lock
// rather than one specific model.
const allModelsSentinel = "__all__"

// cooldownAccountRow is one parked account after merging its account-level
// cooldown with its model-level cooldowns.
type cooldownAccountRow struct {
	AccountID         string
	Label             string
	Scope             string
	Models            []string
	Reason            string
	ReasonLabel       string
	BackoffLevel      int
	ExpiresAt         time.Time
	RetryAfterSeconds int64
}

// adminListCooldowns reports the providers parked on a dispatcher cooldown so
// the dashboard Overview card can surface and clear them. It merges the
// account-level cooldown with the tenant's active model-level cooldowns into a
// single row per account, groups rows by provider, and omits disabled or
// expired entries. Causes are inferred: model_cooldowns persists no reason and
// the account row only records a backoff level and the credits-exhausted flag.
func (s *Server) adminListCooldowns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()

	accounts, err := s.accounts.ListByTenant(ctx, adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	modelCooldowns, err := s.db.Routing().ListActiveModelCooldowns(ctx, adminTenant, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	modelsByAccount, expiryByAccount := indexModelCooldowns(modelCooldowns)

	grouped := make(map[string][]cooldownAccountRow)
	for _, acc := range accounts {
		row, parked := buildCooldownRow(acc, modelsByAccount[acc.ID], expiryByAccount[acc.ID], now)
		if !parked {
			continue
		}
		grouped[acc.Provider] = append(grouped[acc.Provider], row)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at": now.UTC(),
		"providers":    cooldownProviderGroups(grouped),
	})
}

// indexModelCooldowns splits active model cooldowns into per-account model
// lists (excluding the account-wide sentinel) and the latest expiry per account.
func indexModelCooldowns(rows []store.ModelCooldown) (modelsByAccount map[string][]string, expiryByAccount map[string]time.Time) {
	modelsByAccount = make(map[string][]string, len(rows))
	expiryByAccount = make(map[string]time.Time, len(rows))
	for _, mc := range rows {
		if mc.Model != allModelsSentinel {
			modelsByAccount[mc.AccountID] = append(modelsByAccount[mc.AccountID], mc.Model)
		}
		if mc.CooldownUntil.After(expiryByAccount[mc.AccountID]) {
			expiryByAccount[mc.AccountID] = mc.CooldownUntil
		}
	}
	return modelsByAccount, expiryByAccount
}

// buildCooldownRow merges an account's account-level cooldown with its active
// model cooldowns into one report row. The boolean is false when nothing is
// parked. Causes are inferred because model_cooldowns persists no reason: the
// credits flag outranks a per-model lock, which outranks a plain backoff.
func buildCooldownRow(acc store.Account, models []string, modelExpiry time.Time, now time.Time) (cooldownAccountRow, bool) {
	state := newAccountCooldownState(acc, models, modelExpiry, now)
	if acc.Disabled || !state.parked() {
		return cooldownAccountRow{}, false
	}

	reason, reasonLabel := inferCooldownReason(acc, state.modelParked)
	row := cooldownAccountRow{
		AccountID:    acc.ID,
		Label:        acc.Label,
		Models:       append([]string{}, models...),
		BackoffLevel: acc.BackoffLevel,
		ExpiresAt:    state.expiresAt(acc),
		Scope:        cooldownScope(state.accountParked || state.accountWideLock, state.modelParked),
		Reason:       reason,
		ReasonLabel:  reasonLabel,
	}
	if remaining := row.ExpiresAt.Sub(now); remaining > 0 {
		row.RetryAfterSeconds = int64(remaining.Seconds())
	}
	return row, true
}

// accountCooldownState captures which cooldown layers are active for an account.
type accountCooldownState struct {
	accountParked   bool
	modelParked     bool
	accountWideLock bool
	modelExpiry     time.Time
}

// parked reports whether any cooldown layer is active.
func (s accountCooldownState) parked() bool {
	return s.accountParked || s.modelParked || s.accountWideLock
}

// expiresAt returns the latest active expiry across the account and model layers.
func (s accountCooldownState) expiresAt(acc store.Account) time.Time {
	expiresAt := s.modelExpiry
	if s.accountParked && acc.CooldownUntil.After(expiresAt) {
		expiresAt = *acc.CooldownUntil
	}
	return expiresAt
}

// accountCooldownState reports which cooldown layers are parked for an account.
func newAccountCooldownState(acc store.Account, models []string, modelExpiry time.Time, now time.Time) accountCooldownState {
	modelParked := len(models) > 0
	return accountCooldownState{
		accountParked:   acc.CooldownUntil != nil && acc.CooldownUntil.After(now),
		modelParked:     modelParked,
		accountWideLock: modelExpiry.After(now) && !modelParked,
		modelExpiry:     modelExpiry,
	}
}

// inferCooldownReason names the most likely cause of a parked account. Causes
// are inferred: model_cooldowns stores no reason, so the credits flag outranks
// a per-model lock, which outranks a plain backoff.
func inferCooldownReason(acc store.Account, modelParked bool) (string, string) {
	switch {
	case acc.CreditsExhausted:
		return "credits_exhausted", "Credits/quota exhausted"
	case modelParked:
		return "model_rate_limit", "Model rate limit"
	default:
		return "rate_limit", "Rate limit / upstream error"
	}
}

// cooldownProviderGroups sorts each provider's parked accounts and renders the
// grouped map into the JSON response shape, ordered by provider id.
// cooldownProviderGroup is one provider's parked accounts in the JSON response.
type cooldownProviderGroup struct {
	Provider     string           `json:"provider"`
	ProviderName string           `json:"provider_name"`
	Accounts     []map[string]any `json:"accounts"`
}

func cooldownProviderGroups(grouped map[string][]cooldownAccountRow) []cooldownProviderGroup {
	providers := make([]cooldownProviderGroup, 0, len(grouped))
	for provider, rows := range grouped {
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].ExpiresAt.Equal(rows[j].ExpiresAt) {
				return rows[i].AccountID < rows[j].AccountID
			}
			return rows[i].ExpiresAt.Before(rows[j].ExpiresAt)
		})
		entries := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			entries = append(entries, map[string]any{
				"account_id":          row.AccountID,
				"label":               row.Label,
				"scope":               row.Scope,
				"models":              row.Models,
				"reason":              row.Reason,
				"reason_label":        row.ReasonLabel,
				"backoff_level":       row.BackoffLevel,
				"expires_at":          row.ExpiresAt.UTC(),
				"retry_after_seconds": row.RetryAfterSeconds,
			})
		}
		display := provider
		if spec, ok := connectors.SpecByID(provider); ok {
			display = spec.DisplayName
		}
		providers = append(providers, cooldownProviderGroup{
			Provider:     provider,
			ProviderName: display,
			Accounts:     entries,
		})
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Provider < providers[j].Provider
	})
	return providers
}

// cooldownScope classifies whether an account is parked at the account level,
// the model level, or both.
func cooldownScope(hasAccount, hasModel bool) string {
	switch {
	case hasAccount && hasModel:
		return cooldownScopeBoth
	case hasModel:
		return cooldownScopeModel
	default:
		return cooldownScopeAccount
	}
}

// withTx runs fn inside a transaction, rolling back on error or panic-free
// failure and committing only when fn succeeds.
func (s *Server) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
