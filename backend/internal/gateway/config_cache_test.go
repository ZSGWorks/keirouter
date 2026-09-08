package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/store"
)

func TestConfigCacheHitAndExpiry(t *testing.T) {
	c := newConfigCache[string]()
	c.set("k", "v")
	if got, ok := c.get("k"); !ok || got != "v" {
		t.Fatalf("get = %q, %v; want hit", got, ok)
	}
	c.ttl = -time.Second
	c.set("k2", "v2")
	if _, ok := c.get("k2"); ok {
		t.Fatal("expired entry should miss")
	}
	c.invalidate()
	c.ttl = time.Minute
	if _, ok := c.get("k"); ok {
		t.Fatal("invalidate should clear entries")
	}
}

func TestChainsCacheConcurrentInitialization(t *testing.T) {
	s := &Server{}
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.chainsCache() == nil {
				t.Error("chains cache is nil")
			}
		}()
	}
	wg.Wait()
}

func TestCachedChainSourceHitsCache(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := &Server{chains: db.Chains(), aliases: db.Aliases()}
	if err := db.Tenants().Upsert(ctx, store.Tenant{ID: "t1", Name: "t1"}); err != nil {
		t.Fatalf("upsert tenant: %v", err)
	}
	chain := store.Chain{ID: "c1", TenantID: "t1", Name: "route1"}
	if err := db.Chains().Create(ctx, chain); err != nil {
		t.Fatalf("create chain: %v", err)
	}

	src := s.chainSource()
	first, err := src.ListByTenant(ctx, "t1")
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	if len(first) != 1 || first[0].Name != "route1" {
		t.Fatalf("first = %+v", first)
	}

	// Mutate behind the cache's back: cached read must still see the old list.
	if err := db.Chains().Delete(ctx, "c1"); err != nil {
		t.Fatalf("delete chain: %v", err)
	}
	cached, err := src.ListByTenant(ctx, "t1")
	if err != nil {
		t.Fatalf("cached list: %v", err)
	}
	if len(cached) != 1 {
		t.Fatalf("expected cached hit with 1 chain, got %d", len(cached))
	}

	// Invalidation must drop the stale entry.
	s.invalidateConfigCaches()
	updated, err := src.ListByTenant(ctx, "t1")
	if err != nil {
		t.Fatalf("post-invalidate list: %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("post-invalidate list = %+v; want no chains", updated)
	}
}

func TestCachedAliasSourceCachesMiss(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := &Server{aliases: db.Aliases()}
	src := s.aliasSource()

	if _, err := src.Get(ctx, "nope"); err != store.ErrNotFound {
		t.Fatalf("first miss err = %v", err)
	}
	// The not-found result is cached; store mutation stays invisible until invalidate.
	if err := db.Aliases().Set(ctx, "nope", "openai/gpt-4o"); err != nil {
		t.Fatalf("set alias: %v", err)
	}
	if _, err := src.Get(ctx, "nope"); err != store.ErrNotFound {
		t.Fatalf("cached miss should persist, got err = %v", err)
	}
	s.invalidateConfigCaches()
	rec, err := src.Get(ctx, "nope")
	if err != nil || rec.Target != "openai/gpt-4o" {
		t.Fatalf("post-invalidate get = %+v, %v", rec, err)
	}
}

func TestPlanLimitsCacheStoresNotFound(t *testing.T) {
	c := newConfigCache[planLimitsLookup]()
	c.set("p1", planLimitsLookup{found: false})
	lookup, ok := c.get("p1")
	if !ok || lookup.found {
		t.Fatalf("lookup = %+v ok=%v; want cached miss", lookup, ok)
	}
}
