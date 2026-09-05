-- Per-chain token-saving overrides. Stores a JSON blob of tri-state overrides
-- for the global endpoint token-saving settings (toggles + levels). NULL JSON
-- values / absent keys mean "inherit the global setting"; empty column value
-- means no overrides at all.
ALTER TABLE chains ADD COLUMN token_saving TEXT NOT NULL DEFAULT '';