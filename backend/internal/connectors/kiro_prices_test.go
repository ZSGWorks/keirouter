package connectors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestKiroModelPrices_Coverage verifies that every Kiro price entry emitted
// by the generator carries positive input and output rates, so usage
// statistics can report a non-zero cost rather than silently defaulting to
// free. Iterating the generator keeps this exhaustive as new base models and
// suffix variants are added.
func TestKiroModelPrices_Coverage(t *testing.T) {
	entries := kiroModelPrices()
	require.NotEmpty(t, entries, "kiro price table must not be empty")
	for _, mp := range entries {
		require.Equalf(t, "kiro", mp.Provider, "unexpected provider %q", mp.Provider)
		require.Greaterf(t, mp.InputPerM, 0.0, "input price must be > 0 for kiro/%s", mp.Model)
		require.Greaterf(t, mp.OutputPerM, 0.0, "output price must be > 0 for kiro/%s", mp.Model)
	}
}

// TestKiroModelPrices_NewClaudeVersions verifies the Sonnet/Opus 4.5–4.8
// variants (and their synthetic suffixes) resolve to the expected retail rates.
func TestKiroModelPrices_NewClaudeVersions(t *testing.T) {
	cases := []struct {
		model              string
		wantInput, wantOut float64
	}{
		{"claude-sonnet-5", 3, 15},
		{"claude-sonnet-5-thinking-agentic", 3, 15},
		{"claude-sonnet-4.6", 3, 15},
		{"claude-sonnet-4.6-thinking-agentic", 3, 15},
		{"claude-sonnet-4.7", 3, 15},
		{"claude-sonnet-4.8", 3, 15},
		{"claude-opus-4.5", 15, 75},
		{"claude-opus-4.5-thinking", 15, 75},
		{"claude-opus-4.6", 15, 75},
		{"claude-opus-4.7-thinking", 15, 75},
		{"claude-opus-4.8-thinking-agentic", 15, 75},
	}
	for _, tc := range cases {
		mp, ok := ModelPriceByProviderModel("kiro", tc.model)
		require.Truef(t, ok, "missing price for kiro/%s", tc.model)
		require.Equalf(t, tc.wantInput, mp.InputPerM, "input rate for kiro/%s", tc.model)
		require.Equalf(t, tc.wantOut, mp.OutputPerM, "output rate for kiro/%s", tc.model)
	}
}
