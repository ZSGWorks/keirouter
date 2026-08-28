package pipeline

import (
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
)

func TestUnsupportedCloudflareImageError(t *testing.T) {
	req := &core.ChatRequest{Messages: []core.Message{{Content: []core.ContentPart{{Type: core.PartImage}}}}}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"image URL unsupported", &core.ProviderError{StatusCode: 400, Message: "image_url is not supported"}, true},
		{"vision unsupported", &core.ProviderError{StatusCode: 422, Message: "vision input unsupported"}, true},
		{"content must be string", &core.ProviderError{StatusCode: 400, Message: "multimodal content part must be string"}, true},
		{"auth", &core.ProviderError{StatusCode: 400, Message: "invalid API key"}, false},
		{"rate", &core.ProviderError{StatusCode: 429, Message: "image input unsupported"}, false},
		{"generic bad request", &core.ProviderError{StatusCode: 400, Message: "invalid request"}, false},
		{"server", &core.ProviderError{StatusCode: 500, Message: "vision unsupported"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnsupportedCloudflareImageError(tt.err, "cloudflare-ai", "@cf/example/new-model", req); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
