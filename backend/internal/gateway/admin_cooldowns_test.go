package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/store"
)

type cooldownResetCounts struct {
	ClearedAccounts int64 `json:"cleared_accounts"`
	ClearedModels   int64 `json:"cleared_models"`
}

func openCooldownTestDB(t *testing.T) *store.DB {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Tenants().EnsureDefault(ctx))
	t.Cleanup(func() { _ = db.Close() })
	return db
}

type cooldownSeed struct {
	id     string
	tenant string
	parked bool
}

func seedCooldownAccount(t *testing.T, db *store.DB, s cooldownSeed) {
	t.Helper()
	ctx := context.Background()
	acc := store.Account{
		ID: s.id, TenantID: s.tenant, Provider: "openai", Label: s.id,
		AuthKind: store.AuthAPIKey, NeedsReconnect: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if s.parked {
		until := time.Now().Add(time.Hour)
		acc.CooldownUntil = &until
		acc.BackoffLevel = 2
		acc.CreditsExhausted = true
	}
	require.NoError(t, db.Accounts().Create(ctx, acc))
	if s.parked {
		require.NoError(t, db.Routing().SetModelCooldown(ctx, s.id, "gpt-4", time.Now().Add(time.Hour)))
		require.NoError(t, db.Routing().SetModelCooldown(ctx, s.id, "gpt-5", time.Now().Add(time.Hour)))
		require.NoError(t, db.Health().Upsert(ctx, store.AccountHealth{
			ID: "h-" + s.id, TenantID: s.tenant, AccountID: s.id, Provider: "openai",
			Model: "__all__", Status: "unhealthy", ConsecutiveFailures: 3,
			LastCheckedAt: time.Now(), LastError: "boom", UpdatedAt: time.Now(),
		}))
	}
}

func serveCooldownReset(t *testing.T, db *store.DB, method, path string) (int, cooldownResetCounts) {
	t.Helper()
	s := &Server{db: db, accounts: db.Accounts()}
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) { s.mountAdmin(r) })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	var counts cooldownResetCounts
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &counts), "body=%s", rec.Body.String())
	}
	return rec.Code, counts
}

// TestAdminResetAccountCooldown asserts a parked account is unparked at both
// levels while health history, reconnect flags, and disabled state survive.
func TestAdminResetAccountCooldown(t *testing.T) {
	ctx := context.Background()
	db := openCooldownTestDB(t)
	seedCooldownAccount(t, db, cooldownSeed{id: "acc-parked", tenant: adminTenant, parked: true})
	require.NoError(t, db.ProviderHealth().InsertProbeResult(ctx, store.ProviderProbeResult{
		ID: "probe-1", Provider: "openai", ProviderAccountID: "acc-parked",
		Model: "gpt-4", Status: "failed", TriggeredBy: "scheduled", CreatedAt: time.Now(),
	}))

	code, counts := serveCooldownReset(t, db, http.MethodPost, "/api/accounts/acc-parked/cooldown/reset")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, int64(1), counts.ClearedAccounts)
	require.Equal(t, int64(2), counts.ClearedModels)

	acc, err := db.Accounts().Get(ctx, "acc-parked")
	require.NoError(t, err)
	require.Nil(t, acc.CooldownUntil)
	require.Zero(t, acc.BackoffLevel)
	require.False(t, acc.CreditsExhausted)
	// Untouched guarantees.
	require.True(t, acc.NeedsReconnect, "reset must not clear needs_reconnect")
	require.False(t, acc.Disabled)

	active, err := db.Routing().IsModelCooldownActive(ctx, "acc-parked", "gpt-4")
	require.NoError(t, err)
	require.False(t, active)

	h, err := db.Health().Get(ctx, "acc-parked", "__all__")
	require.NoError(t, err)
	require.Equal(t, "unhealthy", h.Status, "health history must survive a cooldown reset")
	require.Equal(t, 3, h.ConsecutiveFailures)

	probes, total, err := db.ProviderHealth().ListProbeResults(ctx, "openai", time.Now().Add(-time.Hour), 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total, "probe history must survive a cooldown reset")
	require.Len(t, probes, 1)
	require.Equal(t, "probe-1", probes[0].ID)
}

// TestAdminResetAccountCooldown_Empty asserts resetting an unparked account
// succeeds with zero counts and changes nothing. An account whose cooldown
// already expired counts as unparked too.
func TestAdminResetAccountCooldown_Empty(t *testing.T) {
	ctx := context.Background()
	db := openCooldownTestDB(t)
	seedCooldownAccount(t, db, cooldownSeed{id: "acc-clean", tenant: adminTenant})

	expired := time.Now().Add(-time.Hour)
	require.NoError(t, db.Accounts().Create(ctx, store.Account{
		ID: "acc-expired", TenantID: adminTenant, Provider: "openai",
		AuthKind: store.AuthAPIKey, NeedsReconnect: true, CooldownUntil: &expired,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-expired", "gpt-4", time.Now().Add(-time.Minute)))

	code, counts := serveCooldownReset(t, db, http.MethodPost, "/api/accounts/acc-clean/cooldown/reset")
	require.Equal(t, http.StatusOK, code)
	require.Zero(t, counts.ClearedAccounts)
	require.Zero(t, counts.ClearedModels)

	acc, err := db.Accounts().Get(ctx, "acc-clean")
	require.NoError(t, err, "unparked account must survive an empty reset")
	require.True(t, acc.NeedsReconnect)

	code, counts = serveCooldownReset(t, db, http.MethodPost, "/api/accounts/acc-expired/cooldown/reset")
	require.Equal(t, http.StatusOK, code)
	require.Zero(t, counts.ClearedAccounts, "expired cooldown must not count as cleared")
	require.Zero(t, counts.ClearedModels, "expired model cooldown must not count as cleared")
}

// TestAdminResetAccountCooldown_Scope asserts unknown ids 404 and
// foreign-tenant accounts are invisible (404) with their cooldowns intact.
func TestAdminResetAccountCooldown_Scope(t *testing.T) {
	ctx := context.Background()
	db := openCooldownTestDB(t)
	require.NoError(t, db.Tenants().Upsert(ctx, store.Tenant{ID: "foreign", Name: "Foreign", CreatedAt: time.Now()}))
	seedCooldownAccount(t, db, cooldownSeed{id: "acc-foreign", tenant: "foreign", parked: true})

	code, _ := serveCooldownReset(t, db, http.MethodPost, "/api/accounts/nope/cooldown/reset")
	require.Equal(t, http.StatusNotFound, code)

	code, _ = serveCooldownReset(t, db, http.MethodPost, "/api/accounts/acc-foreign/cooldown/reset")
	require.Equal(t, http.StatusNotFound, code, "foreign-tenant account must not be resettable")

	acc, err := db.Accounts().Get(ctx, "acc-foreign")
	require.NoError(t, err)
	require.NotNil(t, acc.CooldownUntil, "foreign cooldown must survive a rejected reset")
	active, err := db.Routing().IsModelCooldownActive(ctx, "acc-foreign", "gpt-4")
	require.NoError(t, err)
	require.True(t, active)
}

// TestAdminResetTenantCooldowns asserts the tenant-wide reset clears every
// parked account of the tenant (plus their model cooldowns), reports honest
// counts, and leaves foreign-tenant state alone.
func TestAdminResetTenantCooldowns(t *testing.T) {
	ctx := context.Background()
	db := openCooldownTestDB(t)
	require.NoError(t, db.Tenants().Upsert(ctx, store.Tenant{ID: "foreign", Name: "Foreign", CreatedAt: time.Now()}))
	seedCooldownAccount(t, db, cooldownSeed{id: "acc-a", tenant: adminTenant, parked: true})
	seedCooldownAccount(t, db, cooldownSeed{id: "acc-b", tenant: adminTenant, parked: true})
	seedCooldownAccount(t, db, cooldownSeed{id: "acc-clean", tenant: adminTenant})
	seedCooldownAccount(t, db, cooldownSeed{id: "acc-foreign", tenant: "foreign", parked: true})

	code, counts := serveCooldownReset(t, db, http.MethodPost, "/api/health/cooldowns/reset")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, int64(2), counts.ClearedAccounts)
	require.Equal(t, int64(4), counts.ClearedModels)

	for _, id := range []string{"acc-a", "acc-b"} {
		acc, err := db.Accounts().Get(ctx, id)
		require.NoError(t, err)
		require.Nil(t, acc.CooldownUntil, "account %s still parked", id)
		require.Zero(t, acc.BackoffLevel)
		require.False(t, acc.CreditsExhausted)
		active, err := db.Routing().IsModelCooldownActive(ctx, id, "gpt-4")
		require.NoError(t, err)
		require.False(t, active)
		h, err := db.Health().Get(ctx, id, "__all__")
		require.NoError(t, err)
		require.Equal(t, "unhealthy", h.Status)
	}

	foreign, err := db.Accounts().Get(ctx, "acc-foreign")
	require.NoError(t, err)
	require.NotNil(t, foreign.CooldownUntil, "foreign cooldown must survive a tenant reset")

	// Second run is a no-op with zero counts.
	code, counts = serveCooldownReset(t, db, http.MethodPost, "/api/health/cooldowns/reset")
	require.Equal(t, http.StatusOK, code)
	require.Zero(t, counts.ClearedAccounts)
	require.Zero(t, counts.ClearedModels)
}
