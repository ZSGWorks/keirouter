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
		NeedsReconnect: true,
	}
	if s.parked {
		until := time.Now().Add(time.Hour)
		acc.CooldownUntil = &until
		acc.BackoffLevel = 2
		acc.CreditsExhausted = true
	}
	createCooldownAccount(t, db, acc)
	if !s.parked {
		return
	}
	require.NoError(t, db.Routing().SetModelCooldown(ctx, s.id, "gpt-4", time.Now().Add(time.Hour)))
	require.NoError(t, db.Routing().SetModelCooldown(ctx, s.id, "gpt-5", time.Now().Add(time.Hour)))
	require.NoError(t, db.Health().Upsert(ctx, store.AccountHealth{
		ID: "h-" + s.id, TenantID: s.tenant, AccountID: s.id, Provider: "openai",
		Model: "__all__", Status: "unhealthy", ConsecutiveFailures: 3,
		LastCheckedAt: time.Now(), LastError: "boom", UpdatedAt: time.Now(),
	}))
}

// serveAdminRoute mounts the admin routes on a throwaway server and serves one
// request, so each cooldown test only supplies its method and path.
func serveAdminRoute(t *testing.T, db *store.DB, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{db: db, accounts: db.Accounts()}
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) { s.mountAdmin(r) })
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func serveCooldownReset(t *testing.T, db *store.DB, method, path string) (int, cooldownResetCounts) {
	t.Helper()
	rec := serveAdminRoute(t, db, method, path)
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
	// An already-expired cooldown must not count as cleared: only actively
	// parked rows match the reset predicate.
	expired := time.Now().Add(-time.Hour)
	require.NoError(t, db.Accounts().Create(ctx, store.Account{
		ID: "acc-stale", TenantID: adminTenant, Provider: "openai",
		AuthKind: store.AuthAPIKey, CooldownUntil: &expired,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))

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

	// The stale expired cooldown was not part of the reset and survives.
	stale, err := db.Accounts().Get(ctx, "acc-stale")
	require.NoError(t, err)
	require.NotNil(t, stale.CooldownUntil, "expired cooldown must survive a tenant reset (GC sweeps it)")

	// Second run is a no-op with zero counts.
	code, counts = serveCooldownReset(t, db, http.MethodPost, "/api/health/cooldowns/reset")
	require.Equal(t, http.StatusOK, code)
	require.Zero(t, counts.ClearedAccounts)
	require.Zero(t, counts.ClearedModels)
}

type cooldownListEntry struct {
	AccountID         string    `json:"account_id"`
	Label             string    `json:"label"`
	Scope             string    `json:"scope"`
	Models            []string  `json:"models"`
	Reason            string    `json:"reason"`
	ReasonLabel       string    `json:"reason_label"`
	BackoffLevel      int       `json:"backoff_level"`
	ExpiresAt         time.Time `json:"expires_at"`
	RetryAfterSeconds int64     `json:"retry_after_seconds"`
}

type cooldownListGroup struct {
	Provider     string              `json:"provider"`
	ProviderName string              `json:"provider_name"`
	Accounts     []cooldownListEntry `json:"accounts"`
}

type cooldownListResponse struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Providers   []cooldownListGroup `json:"providers"`
}

func serveCooldownList(t *testing.T, db *store.DB) (int, cooldownListResponse) {
	t.Helper()
	rec := serveAdminRoute(t, db, http.MethodGet, "/api/cooldowns")
	var body cooldownListResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body=%s", rec.Body.String())
	}
	return rec.Code, body
}

func createCooldownAccount(t *testing.T, db *store.DB, acc store.Account) {
	t.Helper()
	if acc.TenantID == "" {
		acc.TenantID = adminTenant
	}
	if acc.Provider == "" {
		acc.Provider = "openai"
	}
	if acc.Label == "" {
		acc.Label = acc.ID
	}
	if acc.AuthKind == "" {
		acc.AuthKind = store.AuthAPIKey
	}
	if acc.CreatedAt.IsZero() {
		acc.CreatedAt = time.Now()
	}
	if acc.UpdatedAt.IsZero() {
		acc.UpdatedAt = time.Now()
	}
	require.NoError(t, db.Accounts().Create(context.Background(), acc))
}

func TestAdminListCooldownsMergesAccountAndModelLocks(t *testing.T) {
	db := openCooldownTestDB(t)
	ctx := context.Background()
	until := time.Now().Add(time.Hour)

	createCooldownAccount(t, db, store.Account{
		ID: "acc-both", Provider: "openai", Label: "primary",
		CooldownUntil: &until, BackoffLevel: 2, CreditsExhausted: true,
	})
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-both", "gpt-4", time.Now().Add(30*time.Minute)))
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-both", "gpt-5", time.Now().Add(2*time.Hour)))

	code, body := serveCooldownList(t, db)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, body.Providers, 1)

	group := body.Providers[0]
	require.Equal(t, "openai", group.Provider)
	require.Equal(t, "OpenAI", group.ProviderName)
	require.Len(t, group.Accounts, 1)

	row := group.Accounts[0]
	require.Equal(t, "acc-both", row.AccountID)
	require.Equal(t, "primary", row.Label)
	require.Equal(t, "both", row.Scope)
	require.ElementsMatch(t, []string{"gpt-4", "gpt-5"}, row.Models)
	require.Equal(t, "credits_exhausted", row.Reason)
	require.Equal(t, "Credits/quota exhausted", row.ReasonLabel)
	require.Equal(t, 2, row.BackoffLevel)
	require.Greater(t, row.RetryAfterSeconds, int64(0))
	require.WithinDuration(t, time.Now().Add(2*time.Hour), row.ExpiresAt, time.Minute)
}

func TestAdminListCooldownsExcludesExpiredDisabledAndForeign(t *testing.T) {
	db := openCooldownTestDB(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	createCooldownAccount(t, db, store.Account{ID: "acc-expired", Provider: "openai", CooldownUntil: &past})
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-expired", "gpt-4", past))

	createCooldownAccount(t, db, store.Account{ID: "acc-disabled", Provider: "openai", Disabled: true, CooldownUntil: &future})
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-disabled", "gpt-4", future))

	require.NoError(t, db.Tenants().Upsert(ctx, store.Tenant{ID: "tenant-other", Name: "Other", CreatedAt: time.Now()}))
	createCooldownAccount(t, db, store.Account{ID: "acc-foreign", TenantID: "tenant-other", Provider: "openai", CooldownUntil: &future})
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-foreign", "gpt-4", future))

	code, body := serveCooldownList(t, db)
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, body.Providers)
	require.WithinDuration(t, time.Now(), body.GeneratedAt, time.Minute)
}

func TestAdminListCooldownsInfersReasonAndScope(t *testing.T) {
	db := openCooldownTestDB(t)
	ctx := context.Background()
	future := time.Now().Add(time.Hour)

	createCooldownAccount(t, db, store.Account{ID: "acc-model", Provider: "openai"})
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-model", "gpt-4", future))

	createCooldownAccount(t, db, store.Account{ID: "acc-sentinel", Provider: "openai"})
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-sentinel", allModelsSentinel, future))

	createCooldownAccount(t, db, store.Account{ID: "acc-rate", Provider: "anthropic", CooldownUntil: &future, BackoffLevel: 1})

	code, body := serveCooldownList(t, db)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, body.Providers, 2)
	require.Equal(t, "anthropic", body.Providers[0].Provider)
	require.Equal(t, "openai", body.Providers[1].Provider)

	byLabel := map[string]cooldownListEntry{}
	for _, group := range body.Providers {
		for _, row := range group.Accounts {
			byLabel[row.Label] = row
		}
	}

	model := byLabel["acc-model"]
	require.Equal(t, "model", model.Scope)
	require.Equal(t, "model_rate_limit", model.Reason)
	require.Equal(t, "Model rate limit", model.ReasonLabel)
	require.Equal(t, []string{"gpt-4"}, model.Models)

	sentinel := byLabel["acc-sentinel"]
	require.Equal(t, "account", sentinel.Scope)
	require.Equal(t, "rate_limit", sentinel.Reason)
	require.Empty(t, sentinel.Models)

	rate := byLabel["acc-rate"]
	require.Equal(t, "account", rate.Scope)
	require.Equal(t, "rate_limit", rate.Reason)
	require.Equal(t, 1, rate.BackoffLevel)
}

func TestListActiveModelCooldownsScopesTenantAndExpiry(t *testing.T) {
	db := openCooldownTestDB(t)
	ctx := context.Background()
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	createCooldownAccount(t, db, store.Account{ID: "acc-keep", Provider: "openai"})
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-keep", "gpt-4", future))
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-keep", "gpt-5", past))

	require.NoError(t, db.Tenants().Upsert(ctx, store.Tenant{ID: "tenant-other", Name: "Other", CreatedAt: time.Now()}))
	createCooldownAccount(t, db, store.Account{ID: "acc-foreign", TenantID: "tenant-other", Provider: "openai"})
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-foreign", "gpt-4", future))

	rows, err := db.Routing().ListActiveModelCooldowns(ctx, adminTenant, time.Now())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "acc-keep", rows[0].AccountID)
	require.Equal(t, "gpt-4", rows[0].Model)
	require.WithinDuration(t, future, rows[0].CooldownUntil, time.Minute)
}
