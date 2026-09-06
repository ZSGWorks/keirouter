package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/stretchr/testify/require"
)

func TestAdminUpdatePlanPersistsValidatedFields(t *testing.T) {
	s, db := newCustomProviderTestServer(t)
	plan := store.Plan{
		ID: "plan-1", TenantID: adminTenant, Name: "Starter", Period: "monthly",
		AlertPct: 80, HardCutoff: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.Plans().Create(context.Background(), plan))

	rec := httptest.NewRecorder()
	s.adminUpdatePlan(rec, planUpdateRequestForTest(`{"name":"Growth","limit_usd":2.5,"rpm_limit":100,"period":"weekly","allowed_models":["openai/gpt-4o"]}`, plan.ID))

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"id":"plan-1","name":"Growth","description":"","limit_micros":2500000,"limit_tokens":0,"rpm_limit":100,"tpm_limit":0,"concurrency_limit":0,"period":"weekly","alert_pct":80,"hard_cutoff":true,"allowed_models":["openai/gpt-4o"]}`, rec.Body.String())
	updated, err := db.Plans().Get(context.Background(), plan.ID)
	require.NoError(t, err)
	require.Equal(t, "Growth", updated.Name)
	require.Equal(t, int64(2_500_000), updated.LimitMicros)
	require.Equal(t, int64(100), updated.RPMLimit)
	require.Equal(t, "weekly", updated.Period)
	require.Equal(t, "openai/gpt-4o", updated.AllowedModels)
}

func TestAdminUpdatePlanRejectsInvalidFieldWithoutPersisting(t *testing.T) {
	s, db := newCustomProviderTestServer(t)
	plan := store.Plan{ID: "plan-1", TenantID: adminTenant, Name: "Starter", Period: "monthly", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	require.NoError(t, db.Plans().Create(context.Background(), plan))

	rec := httptest.NewRecorder()
	s.adminUpdatePlan(rec, planUpdateRequestForTest(`{"limit_tokens":-1}`, plan.ID))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":{"message":"limit_tokens must not be negative","type":"invalid_request_error"}}`, rec.Body.String())
	updated, err := db.Plans().Get(context.Background(), plan.ID)
	require.NoError(t, err)
	require.Zero(t, updated.LimitTokens)
}

func planUpdateRequestForTest(body, id string) *http.Request {
	req := httptest.NewRequest(http.MethodPatch, "/plans/"+id, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
