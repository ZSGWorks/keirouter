package dispatch

import (
	"context"
	"sync"
	"time"

	"github.com/mydisha/keirouter/backend/internal/store"
)

// rotationEntryMax bounds each rotation-state map so long-running processes
// with many chains/targets/affinity keys stay bounded. Sweeps are lazy: once
// a map reaches the bound it is trimmed (expired affinity entries first,
// otherwise halved), mirroring the lazy-sweep pattern of recentFailures.
const rotationEntryMax = 4096

// maxRotationPersistGoroutines caps concurrent background persistence writes;
// when saturated, persistence is skipped (memory stays authoritative).
const maxRotationPersistGoroutines = 32

// rotationCache keeps chain/target rotation cursors and account affinity in
// process memory, seeded from the persisted store on first use per key.
// Advances and re-pins are served from memory (one mutex-protected
// read-modify-write per request) and persisted best-effort in a background
// goroutine, replacing 2-4 synchronous SQLite roundtrips per planned request.
//
// Durability trade-off: rotation cursors and affinity pins are heuristics, so
// losing the last few advances on a crash is harmless — routing redistributes
// on the next request. The store rows remain the cross-restart seed.
type rotationCache struct {
	mu       sync.Mutex
	cursors  map[string]rotationCursor
	sizes    map[string]int
	affinity map[string]store.AccountAffinity
	loaded   map[string]bool
	routing  RoutingSource

	pendingMu sync.Mutex
	inFlight  int
}

// rotationCursor is the minimal rotation state both chains and targets share.
type rotationCursor struct {
	lastIndex int
	hitCount  int
}

func newRotationCache(routing RoutingSource) *rotationCache {
	return &rotationCache{
		cursors:  make(map[string]rotationCursor),
		sizes:    make(map[string]int),
		affinity: make(map[string]store.AccountAffinity),
		loaded:   make(map[string]bool),
		routing:  routing,
	}
}

// seedLocked loads one key's persisted state into memory the first time it is
// requested. Errors are swallowed: missing state means zero cursor, and a
// failed seed simply falls back to the in-memory zero value.
func (r *rotationCache) seedLocked(kind, key string) {
	id := kind + "/" + key
	if r.loaded[id] {
		return
	}
	r.loaded[id] = true
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	switch kind {
	case "chain":
		if state, err := r.routing.GetChainRotationState(ctx, key); err == nil {
			r.cursors[id] = rotationCursor{lastIndex: state.LastIndex, hitCount: state.HitCount}
		}
	case "target":
		if state, err := r.routing.GetTargetRotationState(ctx, key); err == nil {
			r.cursors[id] = rotationCursor{lastIndex: state.LastIndex, hitCount: state.HitCount}
		}
	case "affinity":
		if state, err := r.routing.GetAccountAffinity(ctx, key); err == nil {
			r.affinity[key] = state
		}
	}
}

// advanceChain atomically reads the chain cursor for this request and stores
// the advanced cursor for the next one, persisting async. Returns the cursor.
func (r *rotationCache) advanceChain(chainID string, length, stickyLimit int) int {
	return r.advance("chain", chainID, length, stickyLimit)
}

// advanceTarget is advanceChain for per-target (provider/model) account cursors.
func (r *rotationCache) advanceTarget(scopeKey string, length, stickyLimit int) int {
	return r.advance("target", scopeKey, length, stickyLimit)
}

// advance atomically reads, advances, and stores one rotation cursor and
// persists it async. kind selects the chain vs target store row.
func (r *rotationCache) advance(kind, key string, length, stickyLimit int) int {
	id := kind + "/" + key
	r.mu.Lock()
	r.seedLocked(kind, key)
	cur := r.cursors[id]
	cursor, nextCursor, nextHitCount := advanceRotationState(length, cur.lastIndex, cur.hitCount, stickyLimit)
	if len(r.cursors) > rotationEntryMax {
		r.cursors = make(map[string]rotationCursor, rotationEntryMax/2)
		r.sizes = make(map[string]int, rotationEntryMax/2)
	}
	r.cursors[id] = rotationCursor{lastIndex: nextCursor, hitCount: nextHitCount}
	next := rotationCursor{lastIndex: nextCursor, hitCount: nextHitCount}
	r.mu.Unlock()

	r.persist(func(ctx context.Context) {
		switch kind {
		case "chain":
			_ = r.routing.SetChainRotationState(ctx, store.ChainRotation{ChainID: key, LastIndex: next.lastIndex, HitCount: next.hitCount})
		case "target":
			_ = r.routing.SetTargetRotationState(ctx, store.TargetRotation{ScopeKey: key, LastIndex: next.lastIndex, HitCount: next.hitCount})
		}
	})
	return cursor
}

// pinAffinity returns the stored affinity for a key (zero value when none).
func (r *rotationCache) pinAffinity(scopeKey string) store.AccountAffinity {
	r.mu.Lock()
	r.seedLocked("affinity", scopeKey)
	state := r.affinity[scopeKey]
	r.mu.Unlock()
	return state
}

// setAffinity stores or clears an affinity pin and persists async.
func (r *rotationCache) setAffinity(state store.AccountAffinity) {
	r.mu.Lock()
	// Memory is now authoritative for this key; a later seedLocked must not
	// overwrite the fresh pin with the stale store row.
	r.loaded["affinity/"+state.ScopeKey] = true
	if len(r.affinity) > rotationEntryMax {
		now := time.Now()
		for k, v := range r.affinity {
			if !v.ExpiresAt.After(now) {
				delete(r.affinity, k)
			}
		}
		if len(r.affinity) > rotationEntryMax {
			r.affinity = make(map[string]store.AccountAffinity, rotationEntryMax/2)
		}
	}
	r.affinity[state.ScopeKey] = state
	r.mu.Unlock()
	r.persist(func(ctx context.Context) {
		_ = r.routing.SetAccountAffinity(ctx, state)
	})
}

// evictAffinity clears a pin only when it still points at accountID, so a
// concurrently re-pinned account is not wiped.
func (r *rotationCache) evictAffinity(scopeKey, accountID string) {
	r.mu.Lock()
	r.loaded["affinity/"+scopeKey] = true
	state := r.affinity[scopeKey]
	if state.AccountID != accountID {
		r.mu.Unlock()
		return
	}
	cleared := store.AccountAffinity{ScopeKey: scopeKey, AccountID: "", ExpiresAt: time.Unix(0, 0)}
	r.affinity[scopeKey] = cleared
	r.mu.Unlock()
	r.persist(func(ctx context.Context) {
		_ = r.routing.SetAccountAffinity(ctx, cleared)
	})
}

// persist runs one background write per change. A small in-flight counter
// caps concurrent writers so a request burst cannot spawn unbounded
// goroutines; saturated calls skip persistence.
func (r *rotationCache) persist(write func(context.Context)) {
	r.pendingMu.Lock()
	if r.inFlight >= maxRotationPersistGoroutines {
		r.pendingMu.Unlock()
		return
	}
	r.inFlight++
	r.pendingMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		write(ctx)
		r.pendingMu.Lock()
		r.inFlight--
		r.pendingMu.Unlock()
	}()
}
