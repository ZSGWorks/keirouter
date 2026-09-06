package connectors

import (
	"testing"

	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

// TestOpenAICompatible_ProviderHeaders verifies per-provider header builders,
// including auth selection, CLI fingerprints, and custom-header overrides.
func TestOpenAICompatible_ProviderHeaders(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		creds core.Credentials
		want  map[string]string
	}{
		{
			name:  "azure prefers access token and adds organization",
			id:    "azure",
			creds: core.Credentials{AccessToken: "token", APIKey: "key", Extra: map[string]string{"organization": "org"}},
			want:  map[string]string{"Authorization": "Bearer token", "OpenAI-Organization": "org"},
		},
		{
			name:  "cline prefixes workos token",
			id:    "cline",
			creds: core.Credentials{APIKey: "key"},
			want: map[string]string{
				"Authorization":  "Bearer workos:key",
				"HTTP-Referer":   "https://cline.bot",
				"X-Title":        "Cline",
				"X-CLIENT-TYPE":  "keirouter",
				"X-PLATFORM":     "unknown",
				"X-IS-MULTIROOT": "false",
			},
		},
		{
			name:  "codebuddy sends CLI fingerprint",
			id:    "codebuddy",
			creds: core.Credentials{APIKey: "key"},
			want: map[string]string{
				"Authorization":       "Bearer key",
				"User-Agent":          codeBuddyUserAgent,
				"X-Product":           "SaaS",
				"X-IDE-Type":          "CLI",
				"X-IDE-Name":          "CLI",
				"X-Requested-With":    "XMLHttpRequest",
				"x-codebuddy-request": "1",
			},
		},
		{
			name:  "agentrouter sends Roo Code fingerprint",
			id:    "agentrouter",
			creds: core.Credentials{APIKey: "key"},
			want: map[string]string{
				"Authorization":               "Bearer key",
				"HTTP-Referer":                "https://github.com/RooVetGit/Roo-Cline",
				"X-Title":                     "Roo Code",
				"User-Agent":                  agentRouterUserAgent,
				"X-Stainless-Arch":            "x64",
				"X-Stainless-Lang":            "js",
				"X-Stainless-OS":              "Windows",
				"X-Stainless-Package-Version": "5.12.2",
				"X-Stainless-Retry-Count":     "0",
				"X-Stainless-Runtime":         "node",
				"X-Stainless-Runtime-Version": "v24.14.0",
			},
		},
		{
			name:  "kimchi sends required user agent",
			id:    "kimchi",
			creds: core.Credentials{APIKey: "key"},
			want:  map[string]string{"Authorization": "Bearer key", "User-Agent": kimchiUserAgent},
		},
		{
			name:  "custom headers override provider defaults",
			id:    "codebuddy",
			creds: core.Credentials{APIKey: "key", Headers: map[string]string{"User-Agent": "custom"}},
			want: map[string]string{
				"Authorization":       "Bearer key",
				"User-Agent":          "custom",
				"X-Product":           "SaaS",
				"X-IDE-Type":          "CLI",
				"X-IDE-Name":          "CLI",
				"X-Requested-With":    "XMLHttpRequest",
				"x-codebuddy-request": "1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NewOpenAICompatible(tt.id, "").headers(tt.creds))
		})
	}
}
