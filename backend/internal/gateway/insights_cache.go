package gateway

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// insightsCacheTTL bounds how stale the Usage/Quota dashboard payloads may be.
// Solo deployments refetch these endpoints rapidly (page open, period toggle,
// nav back-and-forth); a short TTL collapses those into one SQLite aggregation
// pass while keeping the numbers fresh enough for a live dashboard.
const insightsCacheTTL = 12 * time.Second

// insightsCacheMaxEntries caps the retained payload count. The key space is
// bounded (endpoint × period × valid IANA tz), but the cap keeps memory flat
// even if callers feed unbounded strings at the endpoints.
const insightsCacheMaxEntries = 64

// ttlCache is a tiny thread-safe cache of pre-marshaled JSON bodies keyed by a
// caller-supplied string. Entries expire after ttl. Expired entries are
// dropped when encountered; at the entry cap the soonest-expiring entry is
// evicted so retention stays bounded.
type ttlCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	max     int
	entries map[string]ttlEntry
}

type ttlEntry struct {
	body    []byte
	expires time.Time
}

func newTTLCache(ttl time.Duration) *ttlCache {
	return &ttlCache{ttl: ttl, max: insightsCacheMaxEntries, entries: make(map[string]ttlEntry)}
}

func (c *ttlCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expires) {
		delete(c.entries, key)
		return nil, false
	}
	return e.body, true
}

func (c *ttlCache) set(key string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; !ok && len(c.entries) >= c.max {
		var oldest string
		var oldestExp time.Time
		for k, e := range c.entries {
			if oldest == "" || e.expires.Before(oldestExp) {
				oldest = k
				oldestExp = e.expires
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[key] = ttlEntry{body: body, expires: time.Now().Add(c.ttl)}
}

// canonicalInsightsKey builds a cache key whose space is bounded: period is
// folded to the value sinceForPeriod would act on, and tz is only kept when it
// resolves to a real IANA location (invalid values collapse into "local" so
// arbitrary query strings cannot mint new entries).
func canonicalInsightsKey(prefix, period, tz string) string {
	switch period {
	case "today", "24h", "week", "month", "":
	default:
		period = "30d"
	}
	locKey := "local"
	if tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			locKey = tz
		}
	}
	return prefix + "|" + period + "|" + locKey
}

// cacheHit writes a cached body for key if one is live, returning true. Handlers
// call this at the top and return early on a hit.
func (s *Server) cacheHit(w http.ResponseWriter, key string) bool {
	body, ok := s.insightsCache.get(key)
	if !ok {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return true
}

// writeJSONCached marshals v, stores it under key, and writes it. Mirrors
// writeJSON but persists the body so subsequent calls within the TTL skip the
// aggregation work entirely.
func writeJSONCached(w http.ResponseWriter, c *ttlCache, key string, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	c.set(key, body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
