package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mydisha/keirouter/backend/internal/store"
)

func TestParseChainTokenSavingEmpty(t *testing.T) {
	for _, raw := range []string{"", "   ", "not json"} {
		if got := parseChainTokenSaving(raw); got != nil {
			t.Fatalf("parseChainTokenSaving(%q) = %+v, want nil", raw, got)
		}
	}
}

func TestParseChainTokenSavingValid(t *testing.T) {
	off := false
	lvl := "ultra"
	got := parseChainTokenSaving(`{"rtk_enabled":false,"ponytail_level":"ultra"}`)
	if got == nil || got.RTKEnabled == nil || !*got.RTKEnabled == true && off == false {
		if got == nil {
			t.Fatal("want non-nil override")
		}
	}
	if got.RTKEnabled == nil || *got.RTKEnabled {
		t.Fatalf("RTKEnabled = %+v, want false", got.RTKEnabled)
	}
	if got.PonytailLevel == nil || *got.PonytailLevel != lvl {
		t.Fatalf("PonytailLevel = %+v, want ultra", got.PonytailLevel)
	}
	if got.TerseEnabled != nil {
		t.Fatalf("TerseEnabled = %+v, want nil (inherit)", got.TerseEnabled)
	}
}

func TestApplyChainOverridesNil(t *testing.T) {
	es := defaultEndpointSettings()
	out := applyChainOverrides(es, nil)
	if out.RTKEnabled != es.RTKEnabled || out.CavemanEnabled != es.CavemanEnabled {
		t.Fatal("nil override must not change settings")
	}
}

func TestApplyChainOverridesPartial(t *testing.T) {
	es := defaultEndpointSettings() // RTK on, caveman/terse/ponytail/headroom off
	off := false
	on := true
	ov := &ChainTokenSaving{
		RTKEnabled:     &off,
		CavemanEnabled: &on,
		CavemanLevel:   strPtr("ultra"),
	}
	out := applyChainOverrides(es, ov)
	if out.RTKEnabled {
		t.Fatal("RTK should be forced off on this chain")
	}
	if !out.CavemanEnabled || out.CavemanLevel != "ultra" {
		t.Fatalf("caveman = %v/%q, want on/ultra", out.CavemanEnabled, out.CavemanLevel)
	}
	// Untouched fields inherit global values.
	if out.TerseEnabled != es.TerseEnabled || out.PonytailLevel != es.PonytailLevel || out.HeadroomEnabled != es.HeadroomEnabled {
		t.Fatal("fields without overrides must inherit global values")
	}
}

func TestApplyChainOverridesCavemanTerseExclusion(t *testing.T) {
	on := true
	// A chain that forces terse on while global has caveman on: terse wins the
	// explicit override, the runtime safety net drops caveman.
	es := defaultEndpointSettings()
	es.CavemanEnabled = true
	out := applyChainOverrides(es, &ChainTokenSaving{TerseEnabled: &on})
	if !out.TerseEnabled || out.CavemanEnabled {
		t.Fatalf("want terse on + caveman off, got terse=%v caveman=%v", out.TerseEnabled, out.CavemanEnabled)
	}
}

func TestEffectiveTokenSavingNoChain(t *testing.T) {
	s := &Server{}
	es := s.effectiveTokenSaving(context.Background(), "")
	if es.RTKEnabled != defaultEndpointSettings().RTKEnabled {
		t.Fatal("empty blob must inherit defaults")
	}
}

func strPtr(v string) *string { return &v }

func TestValidateChainTokenSavingLevels(t *testing.T) {
	if _, err := validateChainTokenSaving(&ChainTokenSaving{RTKFilterLevel: strPtr("nope")}); err == nil {
		t.Fatal("expected invalid rtk_filter_level error")
	}
	if _, err := validateChainTokenSaving(&ChainTokenSaving{CavemanLevel: strPtr("bogus")}); err == nil {
		t.Fatal("expected invalid caveman_level error")
	}
	if _, err := validateChainTokenSaving(&ChainTokenSaving{TerseLevel: strPtr("bogus")}); err == nil {
		t.Fatal("expected invalid terse_level error")
	}
	if _, err := validateChainTokenSaving(&ChainTokenSaving{PonytailLevel: strPtr("bogus")}); err == nil {
		t.Fatal("expected invalid ponytail_level error")
	}
	raw, err := validateChainTokenSaving(&ChainTokenSaving{CavemanLevel: strPtr("lite")})
	if err != nil || raw == "" {
		t.Fatalf("valid level rejected: %q %v", raw, err)
	}
}

func TestValidateChainTokenSavingCavemanTerseConflict(t *testing.T) {
	on := true
	if _, err := validateChainTokenSaving(&ChainTokenSaving{CavemanEnabled: &on, TerseEnabled: &on}); err == nil {
		t.Fatal("expected caveman+terse conflict error")
	}
}

func TestValidateChainTokenSavingNil(t *testing.T) {
	raw, err := validateChainTokenSaving(nil)
	if err != nil || raw != "" {
		t.Fatalf("nil override should persist as empty, got %q %v", raw, err)
	}
}

func TestResolveTargetsCarriesChainTokenSaving(t *testing.T) {
	chainOn := `{"rtk_enabled":false}`
	chains := &fakeChains{chains: []store.Chain{
		{ID: "c1", Name: "plain", Steps: []store.ChainStep{{Position: 0, Provider: "openai", Model: "gpt-4o"}}},
		{ID: "c2", Name: "custom", TokenSaving: chainOn, Steps: []store.ChainStep{{Position: 0, Provider: "openai", Model: "gpt-4o"}}},
	}}

	res, err := resolveTargets(context.Background(), chains, &fakeAliases{}, nil, "t1", "chain:plain")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TokenSaving != "" {
		t.Fatalf("chain without overrides: TokenSaving = %q, want empty", res.TokenSaving)
	}

	res, err = resolveTargets(context.Background(), chains, &fakeAliases{}, nil, "t1", "custom")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TokenSaving != chainOn {
		t.Fatalf("TokenSaving = %q, want %q", res.TokenSaving, chainOn)
	}

	// Direct provider/model resolution carries no overrides.
	res, err = resolveTargets(context.Background(), chains, &fakeAliases{}, nil, "t1", "openai/gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TokenSaving != "" {
		t.Fatalf("direct target: TokenSaving = %q, want empty", res.TokenSaving)
	}
}

func TestChainTokenSavingRoundTrip(t *testing.T) {
	on := true
	ov := &ChainTokenSaving{PonytailEnabled: &on, PonytailLevel: strPtr("lite")}
	raw, err := validateChainTokenSaving(ov)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseChainTokenSaving(raw)
	if parsed == nil || parsed.PonytailEnabled == nil || !*parsed.PonytailEnabled || parsed.PonytailLevel == nil || *parsed.PonytailLevel != "lite" {
		b, _ := json.Marshal(parsed)
		t.Fatalf("round trip mismatch: %s", b)
	}
}
