package health

import (
	"context"
	"testing"
	"time"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/stretchr/testify/require"
)

func TestCompletedHealthBucketRemainsInRollingWindowAndAcceptsLateEvents(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Migrate(ctx))

	repo := db.ProviderHealth()
	service := New(Config{Enabled: true, RollingWindow: 15 * time.Minute}, nil, repo)
	bucketTime := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
	event := ProviderTelemetryEvent{
		Timestamp: bucketTime.Add(5 * time.Second), Provider: "anthropic", Model: "claude-sonnet",
		Capability: "chat_completions", Status: "success", LatencyMs: 1200,
	}
	service.ingest(event)
	service.flushSnapshots(ctx, false)

	key := store.HealthKey("anthropic", "", "claude-sonnet", "chat_completions")
	service.mu.Lock()
	state := service.states[key]
	require.NotNil(t, state)
	require.Len(t, state.buckets, 1, "completed bucket must remain available to rolling current")
	service.mu.Unlock()

	service.flushCurrent(ctx)
	current, err := repo.GetCurrent(ctx, "anthropic", "", "claude-sonnet", "chat_completions")
	require.NoError(t, err)
	require.Equal(t, int64(1), current.RequestCount)

	// The event arrives after the first snapshot. The retained bucket becomes
	// dirty and replaces that snapshot on the next flush.
	event.Timestamp = bucketTime.Add(45 * time.Second)
	event.LatencyMs = 1800
	service.ingest(event)
	service.flushSnapshots(ctx, false)
	service.flushCurrent(ctx)

	snapshots, err := repo.ListSnapshots(ctx, "anthropic", "", "claude-sonnet", "", bucketTime.Add(-time.Minute))
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, int64(2), snapshots[0].RequestCount)
	current, err = repo.GetCurrent(ctx, "anthropic", "", "claude-sonnet", "chat_completions")
	require.NoError(t, err)
	require.Equal(t, int64(2), current.RequestCount)
}

// TestHealthBucketFreezeAndPruneBoundsMemory verifies that a persisted bucket
// past the rolling window is frozen to percentile markers (raw samples
// discarded) but stays queryable up to MaxHistoryWindow, and that buckets
// beyond MaxHistoryWindow are pruned entirely.
func TestHealthBucketFreezeAndPruneBoundsMemory(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Migrate(ctx))
	repo := db.ProviderHealth()

	service := New(Config{
		Enabled:          true,
		RollingWindow:    5 * time.Minute,
		MaxHistoryWindow: time.Hour,
	}, nil, repo)

	old := time.Now().UTC().Add(-20 * time.Minute)
	service.ingest(ProviderTelemetryEvent{
		Timestamp: old, Provider: "anthropic", Model: "claude-sonnet",
		Capability: "chat_completions", Status: "success", LatencyMs: 1200, TTFTMs: 200,
	})
	service.flushSnapshots(ctx, false)
	service.flushSnapshots(ctx, false)

	key := store.HealthKey("anthropic", "", "claude-sonnet", "chat_completions")
	bucketMinute := old.Truncate(time.Minute).Unix()
	service.mu.Lock()
	state := service.states[key]
	require.NotNil(t, state)
	bucket, ok := state.buckets[bucketMinute]
	require.True(t, ok, "bucket within MaxHistoryWindow must be retained")
	require.True(t, bucket.frozen, "persisted bucket past rolling window must freeze")
	require.Nil(t, bucket.latencies, "raw latency samples must be discarded on freeze")
	require.Equal(t, 1200, bucket.latP95, "frozen p95 must preserve the sample")
	service.mu.Unlock()

	within := service.ProviderStatsSince(time.Hour)
	require.Len(t, within, 1)
	require.Equal(t, int64(1), within[0].Requests)
	require.Equal(t, 1200, within[0].LatencyP95Ms)
	require.Equal(t, 200, within[0].TTFTP95Ms)

	require.Empty(t, service.ProviderStatsSince(5*time.Minute), "aged bucket must be excluded from the rolling-only window")

	// A bucket older than MaxHistoryWindow is pruned after its snapshot write.
	prune := New(Config{
		Enabled:          true,
		RollingWindow:    5 * time.Minute,
		MaxHistoryWindow: 10 * time.Minute,
	}, nil, repo)
	prune.ingest(ProviderTelemetryEvent{
		Timestamp: time.Now().UTC().Add(-20 * time.Minute), Provider: "openai", Model: "gpt-4o",
		Capability: "chat_completions", Status: "success", LatencyMs: 300,
	})
	prune.flushSnapshots(ctx, false)

	pruneKey := store.HealthKey("openai", "", "gpt-4o", "chat_completions")
	prune.mu.Lock()
	prunedState := prune.states[pruneKey]
	require.NotNil(t, prunedState)
	require.Empty(t, prunedState.buckets, "bucket past MaxHistoryWindow must be pruned")
	prune.mu.Unlock()
}
