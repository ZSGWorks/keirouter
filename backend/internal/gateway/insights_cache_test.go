package gateway

import (
	"strings"
	"testing"
	"time"
)

func TestTTLCacheGetDeletesExpired(t *testing.T) {
	c := newTTLCache(1 * time.Millisecond)
	c.set("k", []byte("v"))
	time.Sleep(2 * time.Millisecond)
	if _, ok := c.get("k"); ok {
		t.Fatal("expected expired entry to miss")
	}
	c.mu.Lock()
	_, exists := c.entries["k"]
	c.mu.Unlock()
	if exists {
		t.Fatal("expected expired entry to be deleted on read miss")
	}
}

func TestTTLCacheSetEvictsOldestExpiryAtCap(t *testing.T) {
	c := newTTLCache(time.Minute)
	c.max = 2
	c.set("a", []byte("1"))
	time.Sleep(2 * time.Millisecond)
	c.set("b", []byte("2"))
	c.set("c", []byte("3"))
	if _, ok := c.get("a"); ok {
		t.Fatal("expected soonest-expiring entry to be evicted")
	}
	if _, ok := c.get("b"); !ok {
		t.Fatal("expected newer entry to survive")
	}
	if _, ok := c.get("c"); !ok {
		t.Fatal("expected newest entry to survive")
	}
}

func TestCanonicalInsightsKeyBoundsKeyGrowth(t *testing.T) {
	seen := make(map[string]int)
	periods := []string{"", "today", "24h", "week", "month", "bogus-1", "bogus-2"}
	for _, period := range periods {
		for i := 0; i < 500; i++ {
			tz := "Not/AZone" + strings.Repeat("x", i) + "!"
			seen[canonicalInsightsKey("insights", period, tz)]++
		}
	}
	if len(seen) > len(periods)*2 {
		t.Fatalf("expected bounded key space, got %d distinct keys", len(seen))
	}
	if canonicalInsightsKey("p", "month", "Asia/Jakarta") != "p|month|Asia/Jakarta" {
		t.Fatal("expected valid tz and period to be preserved")
	}
	if canonicalInsightsKey("p", "weird", "Bad/Zone!") != "p|30d|local" {
		t.Fatal("expected invalid period/tz to collapse to canonical defaults")
	}
}
