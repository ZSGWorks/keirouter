-- User-declared capability overrides for custom models.
--
-- When non-empty, holds a JSON object of capability name -> bool (e.g.
-- {"vision":true,"tools":false}). Stated flags replace the built-in heuristic
-- capability resolution (pattern tables) for that model; an empty value keeps
-- heuristic behavior.
ALTER TABLE custom_models ADD COLUMN capabilities TEXT NOT NULL DEFAULT '';