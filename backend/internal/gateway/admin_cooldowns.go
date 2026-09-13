package gateway

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
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
