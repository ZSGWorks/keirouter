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
	seeding  map[string]chan struct{}
	routing  RoutingSource

	pendingMu sync.Mutex
	inFlight  int
	persistWG sync.WaitGroup
}

func (r *rotationCache) waitForPersistence() {
	r.persistWG.Wait()
}

// rotationCursor is the minimal rotation state both chains and targets share.
type rotationCursor struct {
	lastIndex int
	hitCount  int
}

// rotationKey identifies a rotation state row by kind and key.
type rotationKey struct {
	kind string
	key  string
}

func (k rotationKey) id() string { return k.kind + "/" + k.key }

func newRotationCache(routing RoutingSource) *rotationCache {
	return &rotationCache{
		cursors:  make(map[string]rotationCursor),
		sizes:    make(map[string]int),
		affinity: make(map[string]store.AccountAffinity),
		loaded:   make(map[string]bool),
		seeding:  make(map[string]chan struct{}),
		routing:  routing,
	}
}

// seed loads one key's persisted state without blocking unrelated rotations.
// Errors are swallowed: missing state means zero cursor, and a failed seed
// simply falls back to the in-memory zero value.
func (r *rotationCache) seed(key rotationKey) {
	for {
		if r.trySeed(key) {
			return
		}
	}
}

// trySeed attempts to load persisted state for one key. Returns true when
// the key is loaded (or was already loaded), false when another goroutine
// is mid-flight and the caller should retry.
func (r *rotationCache) trySeed(key rotationKey) bool {
	id := key.id()
	r.mu.Lock()
	if r.loaded[id] {
		r.mu.Unlock()
		return true
	}
	if done := r.seeding[id]; done != nil {
		r.mu.Unlock()
		<-done
		return false
	}
	done := make(chan struct{})
	r.seeding[id] = done
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	var cursor rotationCursor
	var affinity store.AccountAffinity
	if key.kind == "affinity" {
		if state, err := r.routing.GetAccountAffinity(ctx, key.key); err == nil {
			affinity = state
		}
	} else {
		cursor = r.loadCursor(ctx, key)
	}
	cancel()

	r.commitSeed(seedResult{key: key, cursor: cursor, affinity: affinity}, done)
	return true
}

// seedResult carries the loaded state for one key into commitSeed.
type seedResult struct {
	key      rotationKey
	cursor   rotationCursor
	affinity store.AccountAffinity
}

// commitSeed writes the loaded state under the mutex, skipping if another
// goroutine already loaded the same key while this flight was in progress.
func (r *rotationCache) commitSeed(res seedResult, done chan struct{}) {
	id := res.key.id()
	r.mu.Lock()
	if r.loaded[id] {
		delete(r.seeding, id)
		close(done)
		r.mu.Unlock()
		return
	}
	if res.key.kind == "affinity" {
		r.affinity[res.key.key] = res.affinity
	} else {
		r.cursors[id] = res.cursor
	}
	r.loaded[id] = true
	delete(r.seeding, id)
	close(done)
	r.mu.Unlock()
}

// loadCursor reads a chain or target cursor from the store.
func (r *rotationCache) loadCursor(ctx context.Context, key rotationKey) rotationCursor {
	if key.kind == "chain" {
		if state, err := r.routing.GetChainRotationState(ctx, key.key); err == nil {
			return rotationCursor{lastIndex: state.LastIndex, hitCount: state.HitCount}
		}
		return rotationCursor{}
	}
	if state, err := r.routing.GetTargetRotationState(ctx, key.key); err == nil {
		return rotationCursor{lastIndex: state.LastIndex, hitCount: state.HitCount}
	}
	return rotationCursor{}
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
	rk := rotationKey{kind: kind, key: key}
	id := rk.id()
	r.seed(rk)
	r.mu.Lock()
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
	r.seed(rotationKey{kind: "affinity", key: scopeKey})
	r.mu.Lock()
	state := r.affinity[scopeKey]
	r.mu.Unlock()
	return state
}

// setAffinity stores or clears an affinity pin and persists async.
func (r *rotationCache) setAffinity(state store.AccountAffinity) {
	r.mu.Lock()
	// Memory is now authoritative for this key; a later seed must not
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
	r.persistWG.Add(1)
	r.pendingMu.Unlock()
	go func() {
		defer r.persistWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		write(ctx)
		r.pendingMu.Lock()
		r.inFlight--
		r.pendingMu.Unlock()
	}()
}
