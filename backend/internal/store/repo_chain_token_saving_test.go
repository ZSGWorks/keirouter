package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChainRepo_TokenSavingRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := t.Context()

	now := time.Now()
	chain := Chain{
		ID:          "ch-ts-1",
		TenantID:    DefaultTenantID,
		Name:        "saver-chain",
		Strategy:    "priority",
		TokenSaving: `{"rtk_enabled":false,"caveman_enabled":true,"caveman_level":"ultra"}`,
		Steps: []ChainStep{{
			ID: "st-1", ChainID: "ch-ts-1", Position: 0,
			Provider: "openai", Model: "gpt-4o", CreatedAt: now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, db.Chains().Create(ctx, chain))

	got, err := db.Chains().Get(ctx, "ch-ts-1")
	require.NoError(t, err)
	require.Equal(t, chain.TokenSaving, got.TokenSaving)

	// Chains without overrides keep the empty blob (inherit).
	plain := Chain{
		ID: "ch-ts-2", TenantID: DefaultTenantID, Name: "plain", Strategy: "priority",
		Steps:     []ChainStep{{ID: "st-2", ChainID: "ch-ts-2", Position: 0, Provider: "openai", Model: "gpt-4o", CreatedAt: now}},
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Chains().Create(ctx, plain))
	gotPlain, err := db.Chains().Get(ctx, "ch-ts-2")
	require.NoError(t, err)
	require.Equal(t, "", gotPlain.TokenSaving)

	// Update clears/sets the blob.
	chain.TokenSaving = ""
	require.NoError(t, db.Chains().Update(ctx, chain))
	gotAfter, err := db.Chains().Get(ctx, "ch-ts-1")
	require.NoError(t, err)
	require.Equal(t, "", gotAfter.TokenSaving)
}