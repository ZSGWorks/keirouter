package capability

import "testing"

func TestCloudflareVisionResolution(t *testing.T) {
	unknown := Resolve("cloudflare-ai", "@cf/example/new-model")
	if unknown.VisionState != SupportUnknown || unknown.Profile.Vision || ImagePolicy("cloudflare-ai", "@cf/example/new-model") != ImageOptimistic {
		t.Fatalf("unknown Cloudflare resolution = %+v", unknown)
	}

	textOnly := Resolve("cloudflare-ai", "@cf/zai-org/glm-4.7-flash")
	if textOnly.VisionState != SupportUnsupported || ImagePolicy("cloudflare-ai", "@cf/zai-org/glm-4.7-flash") != ImageStrip {
		t.Fatalf("text-only Cloudflare resolution = %+v", textOnly)
	}

	glm := Resolve("cloudflare-ai", "@cf/zai-org/glm-5.3-flash")
	if glm.Source != SourceProvider || glm.VisionState != SupportSupported || !glm.Profile.Vision || !glm.Profile.Reasoning || !glm.Profile.Tools || glm.Profile.ThinkingFormat != "openai" || glm.Profile.ContextWindow != 1048576 {
		t.Fatalf("GLM 5.3 resolution = %+v", glm)
	}
	if Resolve("", "glm-5.2").Profile.Vision {
		t.Fatal("generic GLM 5.2 must not gain vision")
	}
}
