package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mydisha/keirouter/backend/internal/store"
)

func TestProbeAccountQuotaCanceledBeforeSemaphoreDoesNotBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		(&Server{}).probeAccountQuota(ctx, &wg, make(chan struct{}), map[string]any{}, store.Account{}, nil)
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled probe blocked")
	}
}
