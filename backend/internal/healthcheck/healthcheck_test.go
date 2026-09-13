package healthcheck

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/vault"
)

// stubConnector records upstream calls for cooldown-skip assertions.
type stubConnector struct {
	chatCalls     atomic.Int32
	validateCalls atomic.Int32
	chatErr       error
	validateErr   error
}

func (s *stubConnector) ID() string { return "probe-test" }

func (s *stubConnector) Dialect() core.Dialect { return "openai" }

func (s *stubConnector) Chat(context.Context, *core.ChatRequest, core.Credentials) (*core.ChatResponse, error) {
	s.chatCalls.Add(1)
	return &core.ChatResponse{}, s.chatErr
}

func (s *stubConnector) Stream(context.Context, *core.ChatRequest, core.Credentials, core.StreamConfig) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk)
	close(ch)
	return ch, nil
}

func (s *stubConnector) Validate(context.Context, core.Credentials) error {
	s.validateCalls.Add(1)
	return s.validateErr
}

type stubConns struct{ conn core.Connector }

func (s stubConns) Get(string) (core.Connector, error) { return s.conn, nil }

func newProbeTestDB(t *testing.T) *store.DB {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Tenants().EnsureDefault(ctx))
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedProbeAccount(t *testing.T, db *store.DB, id string) {
	t.Helper()
	now := time.Now()
	require.NoError(t, db.Accounts().Create(context.Background(), store.Account{
		ID:        id,
		TenantID:  store.DefaultTenantID,
		Provider:  "probe-test",
		Label:     "probe",
		AuthKind:  store.AuthNone,
		Metadata:  "{}",
		CreatedAt: now,
		UpdatedAt: now,
	}))
}

// seedProbeHealth plants a recent usage row so CheckOnce selects the model,
// plus a health row for untouched-state assertions. It returns the health row.
func seedProbeHealth(t *testing.T, db *store.DB, accountID, model string) store.AccountHealth {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	require.NoError(t, db.Usage().Record(ctx, store.UsageRecord{
		ID:        accountID + "\x00" + model,
		TenantID:  store.DefaultTenantID,
		AccountID: accountID,
		Provider:  "probe-test",
		Model:     model,
		Status:    "success",
		CreatedAt: now,
	}))
	h := store.AccountHealth{
		ID:            accountID + "\x00" + model,
		TenantID:      store.DefaultTenantID,
		AccountID:     accountID,
		Provider:      "probe-test",
		Model:         model,
		Status:        "healthy",
		LastCheckedAt: now,
		UpdatedAt:     now,
		LastOKAt:      &now,
	}
	require.NoError(t, db.Health().Upsert(context.Background(), h))
	// Re-read as the untouched baseline: storage truncates timezones.
	stored, err := db.Health().Get(context.Background(), accountID, model)
	require.NoError(t, err)
	return stored
}

func newProbeChecker(db *store.DB, conn *stubConnector) *Checker {
	c := New(Config{
		Enabled:              true,
		Interval:             time.Second,
		Timeout:              5 * time.Second,
		MaxParallel:          8,
		FailureThreshold:     3,
		SuccessThreshold:     1,
		RecentModelWindow:    24 * time.Hour,
		MaxModelsPerProvider: 8,
	}, nil, db.Accounts(), db.Health(), stubConns{conn: conn}, vault.New(nil))
	c.SetCooldownSource(db.Routing())
	return c
}

// A dispatcher cooldown — model level or account level — skips the billable
// probe and leaves the health row exactly as it was.
func TestCheckOnce_SkipsCooledTargets(t *testing.T) {
	cases := map[string]func(context.Context, *store.DB){
		"model cooldown": func(ctx context.Context, db *store.DB) {
			require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-cool", "m1", time.Now().Add(time.Hour)))
		},
		"account cooldown": func(ctx context.Context, db *store.DB) {
			require.NoError(t, db.Accounts().SetCooldown(ctx, "acc-cool", time.Now().Add(time.Hour)))
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			db := newProbeTestDB(t)
			ctx := context.Background()
			seedProbeAccount(t, db, "acc-cool")
			before := seedProbeHealth(t, db, "acc-cool", "m1")
			seed(ctx, db)

			conn := &stubConnector{}
			newProbeChecker(db, conn).CheckOnce(ctx, store.DefaultTenantID, false)

			require.Zero(t, conn.chatCalls.Load(), "cooled-down target must not be probed")
			after, err := db.Health().Get(ctx, "acc-cool", "m1")
			require.NoError(t, err)
			require.Equal(t, before, after, "skipped probe must not touch health state")
		})
	}
}

// Cheap __all__ credential validations keep running under cooldown as an
// early recovery signal.
func TestCheckOnce_ValidateRunsUnderCooldown(t *testing.T) {
	db := newProbeTestDB(t)
	ctx := context.Background()
	seedProbeAccount(t, db, "acc-validate")
	seedProbeHealth(t, db, "acc-validate", "__all__")
	require.NoError(t, db.Accounts().SetCooldown(ctx, "acc-validate", time.Now().Add(time.Hour)))

	conn := &stubConnector{}
	newProbeChecker(db, conn).CheckOnce(ctx, store.DefaultTenantID, false)

	require.Equal(t, int32(1), conn.validateCalls.Load(), "Validate must still run under cooldown")
	require.Zero(t, conn.chatCalls.Load(), "no billable probe must run")
}

// force=true deliberately re-probes cooled-down targets (manual check-now).
func TestCheckOnce_ForceBypassesCooldown(t *testing.T) {
	db := newProbeTestDB(t)
	ctx := context.Background()
	seedProbeAccount(t, db, "acc-force")
	seedProbeHealth(t, db, "acc-force", "m1")
	require.NoError(t, db.Accounts().SetCooldown(ctx, "acc-force", time.Now().Add(time.Hour)))
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-force", "m1", time.Now().Add(time.Hour)))

	conn := &stubConnector{}
	newProbeChecker(db, conn).CheckOnce(ctx, store.DefaultTenantID, true)

	require.Equal(t, int32(1), conn.chatCalls.Load(), "force must re-probe cooled-down targets")
}

// Without a cooldown source, model cooldowns are unknown so probes run
// (fail-open); only the loaded account row can gate.
func TestCheckOnce_NoCooldownSourceProbesAll(t *testing.T) {
	db := newProbeTestDB(t)
	ctx := context.Background()
	seedProbeAccount(t, db, "acc-plain")
	seedProbeHealth(t, db, "acc-plain", "m1")
	require.NoError(t, db.Routing().SetModelCooldown(ctx, "acc-plain", "m1", time.Now().Add(time.Hour)))

	conn := &stubConnector{}
	c := New(Config{
		Enabled:              true,
		Interval:             time.Second,
		Timeout:              5 * time.Second,
		MaxParallel:          8,
		FailureThreshold:     3,
		SuccessThreshold:     1,
		RecentModelWindow:    24 * time.Hour,
		MaxModelsPerProvider: 8,
	}, nil, db.Accounts(), db.Health(), stubConns{conn: conn}, vault.New(nil))
	c.CheckOnce(ctx, store.DefaultTenantID, false)

	require.Equal(t, int32(1), conn.chatCalls.Load(), "missing source must not silence probes")
}

// A healthy model with no cooldown is still probed and recorded (real
// traffic recovery path untouched: record() behavior unchanged).
func TestCheckOnce_HealthyModelProbed(t *testing.T) {
	db := newProbeTestDB(t)
	ctx := context.Background()
	seedProbeAccount(t, db, "acc-ok")
	seedProbeHealth(t, db, "acc-ok", "m1")

	conn := &stubConnector{}
	newProbeChecker(db, conn).CheckOnce(ctx, store.DefaultTenantID, false)

	require.Equal(t, int32(1), conn.chatCalls.Load())
	after, err := db.Health().Get(ctx, "acc-ok", "m1")
	require.NoError(t, err)
	require.Equal(t, "healthy", after.Status)
	require.Equal(t, 1, after.ConsecutiveSuccesses)
}
