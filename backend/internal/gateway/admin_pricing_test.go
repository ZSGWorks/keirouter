package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminRefreshPricing(t *testing.T) {
	t.Run("refreshes catalog", testAdminRefreshPricingSuccess)
	t.Run("returns callback failure", testAdminRefreshPricingFailure)
	t.Run("requires coordinator", testAdminRefreshPricingMissingCoordinator)
	t.Run("forwards canceled request context", testAdminRefreshPricingCanceledContext)
}

func testAdminRefreshPricingSuccess(t *testing.T) {
	called := false
	s := &Server{log: slog.Default(), refreshPricingCatalog: func(context.Context) error { called = true; return nil }}
	rr := httptest.NewRecorder()
	s.adminRefreshPricing(rr, httptest.NewRequest(http.MethodPost, "/api/pricing/refresh", nil))
	if !called {
		t.Fatal("refresh coordinator was not called")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rr.Code)
	}
	if strings.TrimSpace(rr.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("body=%q, want status ok", rr.Body.String())
	}
}

func testAdminRefreshPricingFailure(t *testing.T) {
	assertAdminRefreshPricingStatus(t, func(context.Context) error { return errors.New("upstream unavailable") }, http.StatusInternalServerError)
}

func testAdminRefreshPricingMissingCoordinator(t *testing.T) {
	assertAdminRefreshPricingStatus(t, nil, http.StatusInternalServerError)
}

func testAdminRefreshPricingCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	s := &Server{log: slog.Default(), refreshPricingCatalog: func(refreshCtx context.Context) error { called = true; return refreshCtx.Err() }}
	rr := httptest.NewRecorder()
	s.adminRefreshPricing(rr, httptest.NewRequest(http.MethodPost, "/api/pricing/refresh", nil).WithContext(ctx))
	if !called || rr.Code != http.StatusInternalServerError {
		t.Fatalf("called=%v status=%d, want true/500", called, rr.Code)
	}
}

func assertAdminRefreshPricingStatus(t *testing.T, refresh func(context.Context) error, want int) {
	t.Helper()
	s := &Server{log: slog.Default(), refreshPricingCatalog: refresh}
	rr := httptest.NewRecorder()
	s.adminRefreshPricing(rr, httptest.NewRequest(http.MethodPost, "/api/pricing/refresh", nil))
	if rr.Code != want {
		t.Fatalf("status=%d, want %d", rr.Code, want)
	}
}
