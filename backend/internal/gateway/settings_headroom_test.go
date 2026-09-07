package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/config"
	"github.com/mydisha/keirouter/backend/internal/headroom"
	"github.com/mydisha/keirouter/backend/internal/store"
)

func TestEndpointSettingsDropsLegacyHeadroomURL(t *testing.T) {
	var settings EndpointSettings
	if err := json.Unmarshal([]byte(`{"headroom_enabled":true,"headroom_url":"https://legacy.example"}`), &settings); err != nil {
		t.Fatalf("unmarshal legacy settings: %v", err)
	}
	if !settings.HeadroomEnabled {
		t.Fatal("headroom_enabled was not preserved")
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if strings.Contains(string(encoded), "headroom_url") {
		t.Fatalf("legacy headroom_url survived re-save: %s", encoded)
	}
}

func TestHeadroomConfigUsesBundledRuntime(t *testing.T) {
	t.Setenv(headroom.RuntimeModeEnv, headroom.RuntimeCompose)
	config := (&Server{}).headroomConfigFrom(EndpointSettings{HeadroomEnabled: true, HeadroomTimeoutMs: 3000})
	if config.URL != "http://headroom:8787" {
		t.Fatalf("headroom URL = %q, want Compose runtime URL", config.URL)
	}
}

func TestLegacyHeadroomURLIsDroppedWhenRetainedSettingsAreSaved(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	if err != nil {
		t.Fatalf("open settings store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate settings store: %v", err)
	}
	s := &Server{settings: db.Settings()}
	if err := s.settings.Set(ctx, endpointSettingsKey, `{"headroom_enabled":true,"headroom_url":"https://legacy.example","headroom_timeout_ms":3000}`); err != nil {
		t.Fatalf("seed legacy settings: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/endpoint", strings.NewReader(`{"headroom_enabled":true,"headroom_compress_user_messages":true,"headroom_timeout_ms":4000}`))
	res := httptest.NewRecorder()
	s.adminUpdateEndpointSettings(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", res.Code, res.Body.String())
	}
	raw, err := s.settings.Get(ctx, endpointSettingsKey)
	if err != nil {
		t.Fatalf("read saved settings: %v", err)
	}
	if strings.Contains(raw, "headroom_url") {
		t.Fatalf("legacy URL survived save: %s", raw)
	}
	if !strings.Contains(raw, `"headroom_timeout_ms":4000`) {
		t.Fatalf("retained Headroom controls were not saved: %s", raw)
	}
}

func TestHeadroomTestIgnoresLegacyURLBody(t *testing.T) {
	t.Setenv(headroom.RuntimeModeEnv, "")
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/settings/headroom-test", bytes.NewBufferString(`{"url":"https://legacy.example","timeout_ms":1000}`))
	res := httptest.NewRecorder()
	s.adminTestHeadroom(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("test status = %d, body = %s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "legacy.example") {
		t.Fatalf("legacy URL influenced bundled runtime probe: %s", res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "127.0.0.1:8787") {
		t.Fatalf("response did not report the native bundled runtime: %s", res.Body.String())
	}
}
