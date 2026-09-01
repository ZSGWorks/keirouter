package capability

import (
	"testing"
)

func TestParseUserCaps(t *testing.T) {
	if _, ok := ParseUserCaps(""); ok {
		t.Error("empty string should not state anything")
	}
	if _, ok := ParseUserCaps("not-json{"); ok {
		t.Error("invalid JSON should be rejected")
	}
	uc, ok := ParseUserCaps(`{"vision":true,"tools":false}`)
	if !ok {
		t.Fatal("valid caps should parse")
	}
	if uc.Vision == nil || !*uc.Vision {
		t.Error("vision should be stated true")
	}
	if uc.Tools == nil || *uc.Tools {
		t.Error("tools should be stated false")
	}
	// Unknown keys are ignored, not errors.
	if _, ok := ParseUserCaps(`{"telepathy":true}`); ok {
		t.Error("unknown-only declaration should not state anything")
	}
}

func boolPtr(b bool) *bool { return &b }

func TestUserOverrideBeatsPattern(t *testing.T) {
	// *gemma* pattern claims Vision for all gemma models; a text-only custom
	// Ollama model must be able to force it off.
	AddUserOverride("ollama-local", "gemma3:4b", UserCaps{Vision: boolPtr(false)})
	t.Cleanup(func() { RemoveUserOverride("ollama-local", "gemma3:4b") })

	res := Resolve("ollama-local", "gemma3:4b")
	if res.Source != SourceUser {
		t.Errorf("expected SourceUser, got %v", res.Source)
	}
	if res.Profile.Vision {
		t.Error("stated vision=false should override the *gemma* pattern")
	}
	if res.VisionState != SupportUnsupported {
		t.Errorf("vision state should be definite unsupported, got %v", res.VisionState)
	}
	// Floor defaults still fill unstated fields.
	if !res.Profile.Tools {
		t.Error("unstated caps inherit floor (tools=true)")
	}

	// The tick applies to stripping: an image request gets a placeholder.
	if ImagePolicy("ollama-local", "gemma3:4b") != ImageStrip {
		t.Error("vision=false should use ImageStrip policy")
	}

	// Same model on a different provider is untouched by the override.
	if res2 := Resolve("openai", "gemma3:4b"); res2.Profile.Vision != Resolve("openai", "gemma3:4b").Profile.Vision {
		t.Error("other providers must be unaffected")
	}
}

func TestUserOverrideForceVisionOn(t *testing.T) {
	AddUserOverride("ollama", "my-llava-finetune", UserCaps{Vision: boolPtr(true), Tools: boolPtr(false)})
	t.Cleanup(func() {
		RemoveUserOverride("ollama-local", "gemma3:4b")
		RemoveUserOverride("ollama", "my-llava-finetune")
	})

	res := Resolve("ollama", "my-llava-finetune")
	if res.Source != SourceUser {
		t.Errorf("expected SourceUser, got %v", res.Source)
	}
	if !res.Profile.Vision {
		t.Error("stated vision=true should grant vision for an unknown model")
	}
	if res.Profile.Tools {
		t.Error("stated tools=false should disable tools")
	}
	if ImagePolicy("ollama", "my-llava-finetune") != ImagePreserve {
		t.Error("vision=true should preserve images")
	}
}

func TestUserOverrideVendorPrefixLookup(t *testing.T) {
	AddUserOverride("ollama", "llava:13b", UserCaps{Vision: boolPtr(true)})
	t.Cleanup(func() { RemoveUserOverride("ollama", "llava:13b") })

	// Resolution with a vendor-prefixed model id must find the bare-id entry,
	// matching Resolve's canonical-id vendor-prefix stripping.
	res := Resolve("ollama", "ollama/llava:13b")
	if res.Source != SourceUser || !res.Profile.Vision {
		t.Error("vendor-prefixed model id should find the bare-id user override")
	}
}

func TestUserOverrideRemoveRestoresHeuristic(t *testing.T) {
	// *llama-4* pattern sets Vision=true and 1M context.
	before := Resolve("ollama", "llama-4-scout")
	AddUserOverride("ollama", "llama-4-scout", UserCaps{Vision: boolPtr(false)})
	during := Resolve("ollama", "llama-4-scout")
	RemoveUserOverride("ollama", "llama-4-scout")
	after := Resolve("ollama", "llama-4-scout")

	if during.Source != SourceUser || during.Profile.Vision {
		t.Error("override should force vision off while present")
	}
	if after.Source == SourceUser || after.Profile.Vision != before.Profile.Vision {
		t.Error("removal should restore heuristic resolution")
	}
}

func TestUserOverridesSetReplacesRegistry(t *testing.T) {
	SetUserOverrides(map[string]map[string]string{
		"prov-a": {"m1": `{"vision":true}`},
		"prov-b": {"m2": `{"tools":false}`},
	})
	t.Cleanup(func() { SetUserOverrides(nil) })

	if r := Resolve("prov-a", "m1"); r.Source != SourceUser || !r.Profile.Vision {
		t.Error("prov-a/m1 should carry the stated override")
	}
	if r := Resolve("prov-b", "m1"); r.Source == SourceUser {
		t.Error("unrelated provider must not inherit the override")
	}
	if r := Resolve("prov-b", "m2"); r.Source != SourceUser || r.Profile.Tools {
		t.Error("prov-b/m1 tools=false must disable tools")
	}

	// Empty declarations are dropped; the next Set clears stale entries.
	SetUserOverrides(map[string]map[string]string{"prov-a": {"m1": ""}})
	if r := Resolve("prov-a", "m1"); r.Source == SourceUser {
		t.Error("empty declaration should clear the override")
	}
}

func TestAddUserOverrideEmptyRemoves(t *testing.T) {
	AddUserOverride("p1", "m1", UserCaps{PDF: boolPtr(true)})
	if _, ok := lookupUserOverride("p1", "m1"); !ok {
		t.Fatal("override should be registered")
	}
	AddUserOverride("p1", "m1", UserCaps{})
	if _, ok := lookupUserOverride("p1", "m1"); ok {
		t.Error("empty declaration should remove the override")
	}
}
