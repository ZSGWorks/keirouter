package capability

import "testing"

func TestMergeChainProfilesEmpty(t *testing.T) {
	merged := MergeChainProfiles(nil)
	if merged != defaultProfile() {
		t.Errorf("MergeChainProfiles(nil) = %+v, want floor %+v", merged, defaultProfile())
	}
}

func TestMergeChainProfilesVisionUnion(t *testing.T) {
	textOnly := defaultProfile()
	vision := Profile{Vision: true, Reasoning: true, ContextWindow: 200000, MaxOutput: 128000}

	merged := MergeChainProfiles([]Profile{textOnly, vision})
	if !merged.Vision {
		t.Errorf("vision union failed: %+v", merged)
	}
	if !merged.Tools {
		t.Errorf("tools union failed: %+v", merged)
	}
	if !merged.Reasoning {
		t.Errorf("reasoning union failed: %+v", merged)
	}
	if merged.ContextWindow != 200000 || merged.MaxOutput != 128000 {
		t.Errorf("limits not maxed: ctx=%d out=%d", merged.ContextWindow, merged.MaxOutput)
	}
}

func TestMergeChainProfilesLimitsAreMax(t *testing.T) {
	small := Profile{ContextWindow: 128000, MaxOutput: 8192}
	large := Profile{ContextWindow: 1000000, MaxOutput: 64000}

	merged := MergeChainProfiles([]Profile{small, large})
	if merged.ContextWindow != 1000000 {
		t.Errorf("ContextWindow = %d, want 1000000", merged.ContextWindow)
	}
	if merged.MaxOutput != 64000 {
		t.Errorf("MaxOutput = %d, want 64000", merged.MaxOutput)
	}
}

func TestMergeChainProfilesTextOnlyNoVision(t *testing.T) {
	a := Resolve("", "glm-5").Profile
	b := Resolve("", "deepseek-chat").Profile
	merged := MergeChainProfiles([]Profile{a, b})
	if merged.Vision {
		t.Errorf("text-only chain must not advertise vision: %+v", merged)
	}
}

func TestMergeChainProfilesGlm53FlashVision(t *testing.T) {
	// Reproduces the reported case: a chain step targeting glm-5.3-flash must
	// surface vision in the merged chain profile.
	a := Resolve("", "glm-5").Profile
	flash := Resolve("", "glm-5.3-flash").Profile
	merged := MergeChainProfiles([]Profile{a, flash})
	if !merged.Vision {
		t.Errorf("chain containing glm-5.3-flash must advertise vision: %+v", merged)
	}
}
