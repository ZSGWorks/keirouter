package gateway

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/mydisha/keirouter/backend/internal/store"
)

// mountAdmin registers the dashboard admin endpoints on the given router. These
// manage API keys, provider accounts, routing chains, budgets, and usage.

func (s *Server) adminExportDatabase(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	export := map[string]any{}

	// Optional passphrase enables a portable backup: each sealed credential is
	// re-keyed from the local master key to a passphrase-derived key, so the
	// backup can be restored on a machine with a different master key.
	passphrase, ok := decodeDatabaseExportPassphrase(w, r)
	if !ok {
		return
	}
	portable := passphrase != ""
	export["portable"] = portable

	accountsOut, ok := s.exportAccounts(ctx, w, passphrase, portable)
	if !ok {
		return
	}
	export["accounts"] = accountsOut
	export["chains"] = exportChainEntries(s, ctx)
	export["keys"] = exportKeys(s, ctx)
	export["budgets"] = exportBudgets(s, ctx)
	export["proxy_pools"] = exportProxyPools(s, ctx)
	export["endpoint_settings"] = s.loadEndpointSettings(ctx)
	export["access_settings"] = s.loadAccessSettings(ctx)
	export["aliases"] = exportAliases(s, ctx)

	writeJSON(w, http.StatusOK, export)
}

// exportAccounts serializes account credentials, re-keying secrets to the
// passphrase-derived key when exporting a portable backup. It writes the error
// response itself and reports whether the export may continue.
func (s *Server) exportAccounts(ctx context.Context, w http.ResponseWriter, passphrase string, portable bool) ([]map[string]any, bool) {
	accs, _ := s.accounts.ListByTenant(ctx, adminTenant)
	accountsOut := make([]map[string]any, 0, len(accs))
	for _, a := range accs {
		out := map[string]any{
			"id": a.ID, "provider": a.Provider, "label": a.Label,
			"auth_kind": a.AuthKind, "priority": a.Priority,
			"disabled": a.Disabled, "proxy_pool_id": a.ProxyPoolID,
			"metadata": a.Metadata,
		}
		if portable {
			if err := s.exportPortableSecrets(out, a, passphrase); err != nil {
				s.consoleLog.Log("ERROR", fmt.Sprintf("Portable export failed for account %s", a.ID), err.Error())
				writeError(w, http.StatusInternalServerError, "portable export failed: cannot re-key account "+a.ID+" (master key mismatch?)")
				return nil, false
			}
		} else {
			appendWrappedAccountSecrets(out, a)
		}
		if a.TokenExpiresAt != nil {
			out["token_expires_at"] = a.TokenExpiresAt
		}
		accountsOut = append(accountsOut, out)
	}
	return accountsOut, true
}

// appendWrappedAccountSecrets copies locally-sealed credential ciphertexts into
// the export entry when the backup is not portable.
func appendWrappedAccountSecrets(out map[string]any, a store.Account) {
	if a.SecretWrappedDEK != "" {
		out["secret_wrapped_dek"] = a.SecretWrappedDEK
		out["secret_ciphertext"] = a.SecretCiphertext
	}
	if a.TokenWrappedDEK != "" {
		out["token_wrapped_dek"] = a.TokenWrappedDEK
		out["token_ciphertext"] = a.TokenCiphertext
	}
	if a.RefreshWrappedDEK != "" {
		out["refresh_wrapped_dek"] = a.RefreshWrappedDEK
		out["refresh_ciphertext"] = a.RefreshCiphertext
	}
}

// exportTableEntries is the shared shape for flat table exports: list, project
// each row into a map, and return the slice for the export payload.
func exportTableEntries[T any](rows []T, project func(T) map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, project(row))
	}
	return out
}

func exportChainEntries(s *Server, ctx context.Context) []map[string]any {
	chains, _ := s.chains.ListByTenant(ctx, adminTenant)
	return exportTableEntries(chains, chainExportEntry)
}

func exportKeys(s *Server, ctx context.Context) []map[string]any {
	keys, _ := s.identity.List(ctx, adminTenant)
	return exportTableEntries(keys, func(k store.APIKey) map[string]any {
		return map[string]any{"name": k.Name, "disabled": k.Disabled}
	})
}

func exportBudgets(s *Server, ctx context.Context) []map[string]any {
	budgets, _ := s.budgets.ListByTenant(ctx, adminTenant)
	return exportTableEntries(budgets, func(b store.Budget) map[string]any {
		return map[string]any{
			"scope_kind": b.ScopeKind, "scope_id": b.ScopeID,
			"limit_micros": b.LimitMicros, "period": b.Period,
			"alert_pct": b.AlertPct, "hard_cutoff": b.HardCutoff,
		}
	})
}

func exportProxyPools(s *Server, ctx context.Context) []map[string]any {
	pools, _ := s.pools.List(ctx)
	return exportTableEntries(pools, func(p store.ProxyPool) map[string]any {
		return map[string]any{
			"id": p.ID, "name": p.Name, "type": p.Type,
			"proxy_url": p.ProxyURL, "no_proxy": p.NoProxy,
			"strict": p.Strict, "is_active": p.IsActive,
		}
	})
}

func exportAliases(s *Server, ctx context.Context) map[string]string {
	aliases, _ := s.aliases.List(ctx)
	aliasMap := map[string]string{}
	for _, a := range aliases {
		aliasMap[a.Alias] = a.Target
	}
	return aliasMap
}

func decodeDatabaseExportPassphrase(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if !decodeJSON(w, r, &req) {
		return "", false
	}
	return strings.TrimSpace(req.Passphrase), true
}
