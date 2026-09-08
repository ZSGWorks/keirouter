package dispatch

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mydisha/keirouter/backend/internal/store"
)

// countingRouting records store reads/writes so tests can prove memory-first
// behavior: many advances, few store roundtrips.
type countingRouting struct {
	mu          sync.Mutex
	chainGets   int
	targetGets  int
	affinityGet int
	chainSets   int
	targetSets  int
	affinitySet int
}

func (c *countingRouting) SetModelCooldown(ctx context.Context, accountID, model string, until time.Time) error {
	return nil
}
func (c *countingRouting) ClearModelCooldown(ctx context.Context, accountID, model string) error {
	return nil
}
func (c *countingRouting) IsModelCooldownActive(ctx context.Context, accountID, model string) (bool, error) {
	return false, nil
}
func (c *countingRouting) ActiveCooldowns(ctx context.Context, accountIDs []string, model string) (map[string]bool, error) {
	return nil, nil
}
func (c *countingRouting) ActiveCooldownExpirations(ctx context.Context, accountIDs []string, model string) (map[string]time.Time, error) {
	return nil, nil
}
func (c *countingRouting) GetChainRotationState(ctx context.Context, chainID string) (store.ChainRotation, error) {
	c.mu.Lock()
	c.chainGets++
	c.mu.Unlock()
	return store.ChainRotation{ChainID: chainID}, nil
}
func (c *countingRouting) SetChainRotationState(ctx context.Context, state store.ChainRotation) error {
	c.mu.Lock()
	c.chainSets++
	c.mu.Unlock()
	return nil
}
func (c *countingRouting) GetTargetRotationState(ctx context.Context, scopeKey string) (store.TargetRotation, error) {
	c.mu.Lock()
	c.targetGets++
	c.mu.Unlock()
	return store.TargetRotation{ScopeKey: scopeKey}, nil
}
func (c *countingRouting) SetTargetRotationState(ctx context.Context, state store.TargetRotation) error {
	c.mu.Lock()
	c.targetSets++
	c.mu.Unlock()
	return nil
}
func (c *countingRouting) GetAccountAffinity(ctx context.Context, scopeKey string) (store.AccountAffinity, error) {
	c.mu.Lock()
	c.affinityGet++
	c.mu.Unlock()
	return store.AccountAffinity{ScopeKey: scopeKey}, nil
}
func (c *countingRouting) SetAccountAffinity(ctx context.Context, state store.AccountAffinity) error {
	c.mu.Lock()
	c.affinitySet++
	c.mu.Unlock()
	return nil
}

func TestRotationCacheConcurrentAdvancesCoverAllCursors(t *testing.T) {
	rt := &countingRouting{}
	cache := newRotationCache(rt)

	const length = 4
	const n = 200
	var mu sync.Mutex
	seen := make(map[int]int)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cursor := cache.advanceChain("chain1", length, 1)
			mu.Lock()
			seen[cursor]++
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(seen) != length {
		t.Fatalf("cursors used = %v; want all %d", seen, length)
	}
	for cursor, count := range seen {
		if count != n/length {
			t.Fatalf("cursor %d used %d times; want %d", cursor, count, n/length)
		}
	}
}

func TestRotationCacheSeedsOncePerKey(t *testing.T) {
	rt := &countingRouting{}
	cache := newRotationCache(rt)
	for i := 0; i < 50; i++ {
		cache.advanceChain("c1", 3, 1)
		cache.advanceTarget("t1", 3, 1)
		cache.pinAffinity("a1")
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.chainGets != 1 || rt.targetGets != 1 || rt.affinityGet != 1 {
		t.Fatalf("store reads chain=%d target=%d affinity=%d; want 1/1/1", rt.chainGets, rt.targetGets, rt.affinityGet)
	}
}

func TestRotationCacheEvictAffinityGuardsForeignAccount(t *testing.T) {
	rt := &countingRouting{}
	cache := newRotationCache(rt)
	cache.setAffinity(store.AccountAffinity{ScopeKey: "s1", AccountID: "acct-a"})
	// Another request re-pinned to acct-b concurrently.
	cache.setAffinity(store.AccountAffinity{ScopeKey: "s1", AccountID: "acct-b"})
	// Stale failure eviction for acct-a must not wipe acct-b's pin.
	cache.evictAffinity("s1", "acct-a")
	pin := cache.pinAffinity("s1")
	if pin.AccountID != "acct-b" {
		t.Fatalf("pin = %q; want acct-b preserved", pin.AccountID)
	}
	cache.evictAffinity("s1", "acct-b")
	if pin := cache.pinAffinity("s1"); pin.AccountID != "" {
		t.Fatalf("pin = %q; want cleared", pin.AccountID)
	}
}

func TestPlanWithConcurrentRotationNoLock(t *testing.T) {
	d, _ := newDispatchTest(t, testAccount("a1", 1))
	rt := &countingRouting{}
	d.SetRoutingSource(rt)
	d.rotationState()

	ctx := context.Background()
	targets := []Target{{Provider: "openai", Model: "m1"}, {Provider: "openai", Model: "m2"}}
	opts := PlanOptions{Strategy: StrategyRoundRobin, ChainID: "c1", StickyLimit: 1}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.PlanWith(ctx, store.DefaultTenantID, targets, nil, opts)
		}()
	}
	wg.Wait()
}
