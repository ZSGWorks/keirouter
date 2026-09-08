package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/mydisha/keirouter/backend/internal/caveman"
	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/ponytail"
	"github.com/mydisha/keirouter/backend/internal/store"
	"github.com/mydisha/keirouter/backend/internal/terse"
)

// mountAdmin registers the dashboard admin endpoints on the given router. These
// manage API keys, provider accounts, routing chains, budgets, and usage.

func parseCodexConsumeResponse(statusCode int, body []byte) map[string]any {
	var data struct {
		Code         string `json:"code"`
		Outcome      string `json:"outcome"`
		WindowsReset int    `json:"windows_reset"`
		Message      string `json:"message"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &data)
	}

	outcome := data.Outcome
	if outcome == "" {
		outcome = data.Code
	}
	ok := statusCode < 400 && (outcome == "reset" || outcome == "alreadyRedeemed" || outcome == "already_redeemed" || data.WindowsReset > 0)
	noCredit := statusCode < 400 && (outcome == "noCredit" || outcome == "no_credit")
	if data.Message == "" {
		switch outcome {
		case "nothingToReset", "nothing_to_reset":
			data.Message = "No eligible rate-limit window needs resetting."
		case "noCredit", "no_credit":
			data.Message = "No reset credits are available."
		}
	}

	return map[string]any{
		"ok":            ok,
		"no_credit":     noCredit,
		"status":        statusCode,
		"code":          data.Code,
		"outcome":       outcome,
		"windows_reset": data.WindowsReset,
		"message":       data.Message,
	}
}

// ---- chains -----------------------------------------------------------------

func (s *Server) adminListChains(w http.ResponseWriter, r *http.Request) {
	chains, err := s.chains.ListByTenant(r.Context(), adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make([]map[string]any, 0, len(chains))
	for _, c := range chains {
		steps := make([]map[string]any, 0, len(c.Steps))
		for _, st := range c.Steps {
			steps = append(steps, map[string]any{"provider": st.Provider, "model": st.Model, "position": st.Position})
		}
		entry := map[string]any{
			"id": c.ID, "name": c.Name, "strategy": c.Strategy, "steps": steps,
		}
		if c.FallbackProvider != "" && c.FallbackModel != "" {
			entry["fallback_provider"] = c.FallbackProvider
			entry["fallback_model"] = c.FallbackModel
		}
		if ov := parseChainTokenSaving(c.TokenSaving); ov != nil {
			entry["token_saving"] = ov
		}
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"chains": out})
}

// validateChainTokenSaving checks a client-supplied per-chain token-saving
// override against the same rules the global endpoint settings enforce, then
// returns it marshalled for persistence. An empty/absent override yields "".
func validateChainTokenSaving(ov *ChainTokenSaving) (string, error) {
	if ov == nil {
		return "", nil
	}
	if err := validateChainTokenSavingLevels(ov); err != nil {
		return "", err
	}
	// Caveman and terse inject conflicting system-prompt directives; mirroring
	// the global settings rule, a chain may not enable both.
	cavOn := ov.CavemanEnabled != nil && *ov.CavemanEnabled
	terseOn := ov.TerseEnabled != nil && *ov.TerseEnabled
	if cavOn && terseOn {
		return "", errors.New("caveman_enabled and terse_enabled cannot both be enabled")
	}
	raw, err := json.Marshal(ov)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// validateChainTokenSavingLevels checks the optional level overrides against
// the same enums the global endpoint settings enforce. Table-driven so each
// level field contributes one row, not one conditional block.
func validateChainTokenSavingLevels(ov *ChainTokenSaving) error {
	checks := []struct {
		value *string
		valid func(string) bool
		msg   string
	}{
		{ov.RTKFilterLevel, func(s string) bool {
			switch s {
			case "none", "minimal", "aggressive":
				return true
			}
			return false
		}, "rtk_filter_level must be none, minimal, or aggressive"},
		{ov.CavemanLevel, func(s string) bool { return caveman.ValidLevel(caveman.Level(s)) }, "caveman_level must be lite, full, ultra, wenyan-lite, wenyan-full, or wenyan-ultra"},
		{ov.TerseLevel, func(s string) bool { return terse.ValidLevel(terse.Level(s)) }, "terse_level must be light, medium, or aggressive"},
		{ov.PonytailLevel, func(s string) bool { return ponytail.ValidLevel(ponytail.Level(s)) }, "ponytail_level must be lite, full, or ultra"},
	}
	for _, c := range checks {
		if c.value != nil && *c.value != "" && !c.valid(*c.value) {
			return errors.New(c.msg)
		}
	}
	return nil
}

// chainExportEntry builds the export/import JSON shape for one chain.
func chainExportEntry(c store.Chain) map[string]any {
	steps := make([]map[string]any, 0, len(c.Steps))
	for _, st := range c.Steps {
		steps = append(steps, map[string]any{
			"provider": st.Provider, "model": st.Model, "position": st.Position,
		})
	}
	entry := map[string]any{
		"name": c.Name, "strategy": c.Strategy, "steps": steps,
	}
	if c.FallbackProvider != "" && c.FallbackModel != "" {
		entry["fallback_provider"] = c.FallbackProvider
		entry["fallback_model"] = c.FallbackModel
	}
	if ov := parseChainTokenSaving(c.TokenSaving); ov != nil {
		entry["token_saving"] = ov
	}
	return entry
}

// importedChain mirrors the exported chain JSON shape for import.
type importedChain struct {
	Name             string            `json:"name"`
	Strategy         string            `json:"strategy"`
	FallbackProvider string            `json:"fallback_provider"`
	FallbackModel    string            `json:"fallback_model"`
	TokenSaving      *ChainTokenSaving `json:"token_saving"`
	Steps            []struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Position int    `json:"position"`
	} `json:"steps"`
}

// chainFromImport builds a store.Chain from an imported chain payload. An
// invalid token-saving override yields an error; the import loop skips
// such rows.
func chainFromImport(c importedChain) (store.Chain, error) {
	ts, err := validateChainTokenSaving(c.TokenSaving)
	if err != nil {
		return store.Chain{}, err
	}
	now := time.Now()
	chain := store.Chain{
		ID:               uuid.NewString(),
		TenantID:         adminTenant,
		Name:             c.Name,
		Strategy:         defaultStr(c.Strategy, "priority"),
		FallbackProvider: c.FallbackProvider,
		FallbackModel:    c.FallbackModel,
		TokenSaving:      ts,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	for _, st := range c.Steps {
		chain.Steps = append(chain.Steps, store.ChainStep{
			ID: uuid.NewString(), ChainID: chain.ID, Position: st.Position,
			Provider: st.Provider, Model: st.Model, CreatedAt: now,
		})
	}
	return chain, nil
}

func (s *Server) adminCreateChain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name             string            `json:"name"`
		Strategy         string            `json:"strategy"`
		FallbackProvider string            `json:"fallback_provider"`
		FallbackModel    string            `json:"fallback_model"`
		TokenSaving      *ChainTokenSaving `json:"token_saving"`
		Steps            []struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		} `json:"steps"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Name == "" || len(body.Steps) == 0 {
		writeError(w, http.StatusBadRequest, "name and at least one step are required")
		return
	}
	if err := validateChainName(body.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Validate fallback provider if set.
	if body.FallbackProvider != "" {
		if _, ok := connectors.SpecByID(body.FallbackProvider); !ok {
			writeError(w, http.StatusBadRequest, "unknown fallback provider: "+body.FallbackProvider)
			return
		}
		if body.FallbackModel == "" {
			writeError(w, http.StatusBadRequest, "fallback_model is required when fallback_provider is set")
			return
		}
	}

	now := time.Now()
	chain := store.Chain{
		ID:               uuid.NewString(),
		TenantID:         adminTenant,
		Name:             body.Name,
		Strategy:         defaultStr(body.Strategy, "priority"),
		FallbackProvider: body.FallbackProvider,
		FallbackModel:    body.FallbackModel,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if !applyTokenSavingToChain(w, &chain, body.TokenSaving) {
		return
	}
	for i, st := range body.Steps {
		if _, ok := connectors.SpecByID(st.Provider); !ok {
			writeError(w, http.StatusBadRequest, "unknown provider in step: "+st.Provider)
			return
		}
		chain.Steps = append(chain.Steps, store.ChainStep{
			ID: uuid.NewString(), ChainID: chain.ID, Position: i,
			Provider: st.Provider, Model: st.Model, CreatedAt: now,
		})
	}
	if err := s.chains.Create(r.Context(), chain); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	s.invalidateConfigCaches()
	writeJSON(w, http.StatusCreated, map[string]any{"id": chain.ID, "name": chain.Name})
}

func (s *Server) adminDeleteChain(w http.ResponseWriter, r *http.Request) {
	if err := s.chains.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	s.invalidateConfigCaches()
	w.WriteHeader(http.StatusNoContent)
}

// applyTokenSavingToChain validates an optional per-chain token-saving
// override and stores it on the chain. Returns false when the override was
// supplied but invalid (response already written).
func applyTokenSavingToChain(w http.ResponseWriter, c *store.Chain, ov *ChainTokenSaving) bool {
	if ov == nil {
		return true
	}
	tokenSaving, err := validateChainTokenSaving(ov)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	c.TokenSaving = tokenSaving
	return true
}

func (s *Server) adminUpdateChain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.chains.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "chain not found")
		return
	}

	var body struct {
		Name             *string           `json:"name"`
		Strategy         *string           `json:"strategy"`
		FallbackProvider *string           `json:"fallback_provider"`
		FallbackModel    *string           `json:"fallback_model"`
		TokenSaving      *ChainTokenSaving `json:"token_saving"`
		Steps            *[]struct {
			Provider string `json:"provider"`
			Model    string `json:"model"`
		} `json:"steps"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	if !applyTokenSavingToChain(w, &existing, body.TokenSaving) {
		return
	}
	if body.Name != nil {
		if err := validateChainName(*body.Name); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		existing.Name = *body.Name
	}
	if body.Strategy != nil {
		existing.Strategy = *body.Strategy
	}
	if body.FallbackProvider != nil {
		existing.FallbackProvider = *body.FallbackProvider
	}
	if body.FallbackModel != nil {
		existing.FallbackModel = *body.FallbackModel
	}
	if body.Steps != nil {
		now := time.Now()
		existing.Steps = make([]store.ChainStep, len(*body.Steps))
		for i, st := range *body.Steps {
			existing.Steps[i] = store.ChainStep{
				ID:        uuid.NewString(),
				ChainID:   id,
				Position:  i,
				Provider:  st.Provider,
				Model:     st.Model,
				CreatedAt: now,
			}
		}
	}

	if err := s.chains.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	s.invalidateConfigCaches()
	writeJSON(w, http.StatusOK, map[string]any{"id": existing.ID, "name": existing.Name})
}

// ---- plans ------------------------------------------------------------------

// validateChainName rejects combo names that would conflict with routing resolution.
// Names must be alphanumeric with hyphens/underscores only, no slashes, colons,
// or leading/trailing whitespace. This prevents ambiguity with "provider/model"
// and "chain:name" formats in resolveTargets.
func validateChainName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("combo name is required")
	}
	if len(name) > 128 {
		return fmt.Errorf("combo name too long (max 128 characters)")
	}
	if strings.ContainsAny(name, "/:\\@#?") {
		return fmt.Errorf("combo name cannot contain / : \\ @ # ? characters")
	}
	if strings.HasPrefix(name, "chain:") {
		return fmt.Errorf("combo name cannot start with 'chain:' prefix")
	}
	// Must match ^[a-zA-Z0-9][a-zA-Z0-9_-]*$
	for i, c := range name {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if c == '-' || c == '_' {
			if i == 0 {
				return fmt.Errorf("combo name must start with a letter or digit")
			}
			continue
		}
		return fmt.Errorf("combo name can only contain letters, digits, hyphens, and underscores")
	}
	return nil
}
