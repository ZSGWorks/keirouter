package gateway

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mydisha/keirouter/backend/internal/capability"
	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/core"
	"github.com/mydisha/keirouter/backend/internal/store"
)

// modelEntry is one entry in a /v1/models listing, in the OpenAI shape plus
// KeiRouter extensions (provider, kind, dimensions).
type modelEntry struct {
	ID               string                      `json:"id"`
	Object           string                      `json:"object"`
	OwnedBy          string                      `json:"owned_by"`
	Provider         string                      `json:"provider,omitempty"`
	Kind             string                      `json:"kind,omitempty"`
	Name             string                      `json:"name,omitempty"`
	Dimensions       int                         `json:"dimensions,omitempty"`
	Capabilities     *modelCapabilities          `json:"capabilities,omitempty"`
	CapabilitySource capability.CapabilitySource `json:"capability_source,omitempty"`
	InputModalities  []string                    `json:"input_modalities,omitempty"`
	OutputModalities []string                    `json:"output_modalities,omitempty"`
}

type liveModelResults struct {
	mu     sync.Mutex
	models map[string][]connectors.ModelSpec
	closed bool
}

func (r *liveModelResults) add(provider string, models []connectors.ModelSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.models[provider] = models
	}
}

func (r *liveModelResults) close() {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
}

// handleListModels reports targetable models: the tenant's chains (as virtual
// models) plus every catalogued LLM model in provider/model form. This lets a
// client discover what it can pass in the `model` field.
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	tenantID := tenantOf(key)

	data := make([]modelEntry, 0, 64)
	seen := make(map[string]struct{}, 64)
	usableProviders := s.usableModelProviders(r.Context(), tenantID)
	data = s.appendChainModels(r.Context(), tenantID, data, seen)
	data = appendCatalogModels(data, seen, usableProviders, core.ServiceLLM)
	data = appendLiveModels(data, seen, s.fetchLiveModels(r.Context(), tenantID), "")

	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// appendChainModels exposes chains as combo models callable by their bare
// name. Each chain entry carries a merged capability payload derived from its
// steps, so clients (e.g. the OpenCode plugin) see accurate modality metadata
// instead of assuming a text-only model.
func (s *Server) appendChainModels(ctx context.Context, tenantID string, data []modelEntry, seen map[string]struct{}) []modelEntry {
	chains, err := s.chains.ListByTenant(ctx, tenantID)
	if err != nil {
		return data
	}
	for _, chain := range chains {
		caps := chainCapabilities(chain)
		data = appendModelEntry(data, seen, modelEntry{
			ID: chain.Name, Object: "model", OwnedBy: "combo", Kind: string(core.ServiceLLM), Name: chain.Name,
			Capabilities:     &caps.caps,
			CapabilitySource: capability.SourceChain,
			InputModalities:  caps.input,
			OutputModalities: caps.output,
		})
	}
	return data
}

// chainCapabilities merges the resolved profiles of a chain's steps into the
// wire capability payload plus modality arrays for the /v1/models entry.
func chainCapabilities(chain store.Chain) struct {
	caps   modelCapabilities
	input  []string
	output []string
} {
	profiles := make([]capability.Profile, 0, len(chain.Steps))
	for _, step := range chain.Steps {
		profiles = append(profiles, capability.ResolveForServiceKind(step.Provider, step.Model, core.ServiceLLM).Profile)
	}
	merged := capability.MergeChainProfiles(profiles)
	return struct {
		caps   modelCapabilities
		input  []string
		output []string
	}{
		caps:   modelCapabilitiesFromProfile(merged),
		input:  inputModalitiesFromProfile(merged),
		output: outputModalitiesFromProfile(merged),
	}
}

// modelCapabilitiesFromProfile projects a resolved profile onto the wire
// capability struct shared with per-model catalog entries.
func modelCapabilitiesFromProfile(p capability.Profile) modelCapabilities {
	return modelCapabilities{
		Vision: p.Vision, PDF: p.PDF, AudioInput: p.AudioInput, VideoInput: p.VideoInput,
		ImageOutput: p.ImageOutput, AudioOutput: p.AudioOutput, Search: p.Search,
		Tools: p.Tools, Reasoning: p.Reasoning, StructuredOutput: p.StructuredOutput,
		ContextWindow: p.ContextWindow, MaxOutput: p.MaxOutput,
	}
}

// inputModalitiesFromProfile lists the canonical input modality strings a
// resolved profile accepts.
func inputModalitiesFromProfile(p capability.Profile) []string {
	mods := []string{"text"}
	if p.Vision {
		mods = append(mods, "image")
	}
	if p.AudioInput {
		mods = append(mods, "audio")
	}
	if p.VideoInput {
		mods = append(mods, "video")
	}
	if p.PDF {
		mods = append(mods, "pdf")
	}
	return mods
}

// outputModalitiesFromProfile lists the canonical output modality strings a
// resolved profile emits.
func outputModalitiesFromProfile(p capability.Profile) []string {
	mods := []string{"text"}
	if p.ImageOutput {
		mods = append(mods, "image")
	}
	if p.AudioOutput {
		mods = append(mods, "audio")
	}
	return mods
}

// appendCatalogModels includes only models dispatchable with tenant accounts.
func appendCatalogModels(data []modelEntry, seen map[string]struct{}, usableProviders map[string]bool, kind core.ServiceKind) []modelEntry {
	for _, providerModel := range connectors.ModelsByKind(kind) {
		if !usableProviders[providerModel.Provider] {
			continue
		}
		model := providerModel.Model
		caps, source := capabilityPayload(providerModel.Provider, model.ID, model.Kind)
		entry := modelEntry{
			ID:           providerModel.Provider + "/" + model.ID,
			Object:       "model",
			OwnedBy:      providerModel.Provider,
			Provider:     providerModel.Provider,
			Kind:         string(model.Kind),
			Name:         model.Name,
			Dimensions:   model.Dimensions,
			Capabilities: &caps, CapabilitySource: source,
		}
		applyModalityArrays(&entry, providerModel.Provider, model.ID, model.Kind)
		data = appendModelEntry(data, seen, entry)
	}
	return data
}

// applyModalityArrays fills the input/output modality arrays on a model entry
// from the resolved capability profile. Media-service kinds (imageToText, tts,
// ...) imply modalities beyond the model profile, so resolution goes through
// ResolveForServiceKind.
func applyModalityArrays(entry *modelEntry, provider, model string, kind core.ServiceKind) {
	p := capability.ResolveForServiceKind(provider, model, kind).Profile
	entry.InputModalities = inputModalitiesFromProfile(p)
	entry.OutputModalities = outputModalitiesFromProfile(p)
}

// appendLiveModels supplements static entries with successful live discovery.
func appendLiveModels(data []modelEntry, seen map[string]struct{}, liveModels map[string][]connectors.ModelSpec, kind core.ServiceKind) []modelEntry {
	for provider, models := range liveModels {
		for _, model := range models {
			if kind != "" && model.Kind != kind {
				continue
			}
			caps, source := capabilityPayload(provider, model.ID, model.Kind)
			entry := modelEntry{
				ID:           provider + "/" + model.ID,
				Object:       "model",
				OwnedBy:      provider,
				Provider:     provider,
				Kind:         string(model.Kind),
				Name:         model.Name,
				Dimensions:   model.Dimensions,
				Capabilities: &caps, CapabilitySource: source,
			}
			applyModalityArrays(&entry, provider, model.ID, model.Kind)
			data = appendModelEntry(data, seen, entry)
		}
	}
	return data
}

// handleListModelsByKind serves GET /v1/models/{kind}: it lists every model of
// the requested service kind (llm, embedding, image, stt, tts, search, fetch)
// across the provider catalog, plus a special "chains" view.
func (s *Server) handleListModelsByKind(w http.ResponseWriter, r *http.Request) {
	kindParam := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "kind")))

	// "chains" is a convenience view of the tenant's routing chains.
	if kindParam == "chains" {
		s.handleListModels(w, r)
		return
	}

	kind := core.ServiceKind(kindParam)
	if !core.ValidServiceKind(kind) {
		writeError(w, http.StatusBadRequest, "unknown model kind: "+kindParam)
		return
	}

	data := make([]modelEntry, 0, 64)
	seen := make(map[string]struct{}, 64)
	key, _ := authedKey(r.Context())
	usableProviders := s.usableModelProviders(r.Context(), tenantOf(key))
	data = appendCatalogModels(data, seen, usableProviders, kind)
	data = appendLiveModels(data, seen, s.fetchLiveModels(r.Context(), tenantOf(key)), kind)
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "kind": kindParam, "data": data})
}

// fetchLiveModels queries providers that support live model discovery, using
// the first connected account's credentials. Returns a map of provider →
// models. Errors are silently skipped (live discovery is best-effort).
func (s *Server) fetchLiveModels(ctx context.Context, tenantID string) map[string][]connectors.ModelSpec {
	if s.accounts == nil || s.vault == nil {
		return nil
	}
	// Bound total discovery latency even when queued providers are unavailable.
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	results := liveModelResults{models: map[string][]connectors.ModelSpec{}}
	var wg sync.WaitGroup
	probes := make(chan struct{}, 4)
	for _, spec := range connectors.Catalog() {
		provider := spec.ID
		src := connectors.GetLiveModelSource(provider)
		if src == nil {
			continue
		}
		select {
		case probes <- struct{}{}:
		case <-probeCtx.Done():
			results.close()
			return results.models
		}
		wg.Add(1)
		go func(provider string, src connectors.LiveModelSource) {
			defer wg.Done()
			defer func() { <-probes }()
			models := s.fetchLiveModelsForProvider(probeCtx, tenantID, provider, src)
			if len(models) == 0 {
				return
			}
			results.add(provider, models)
		}(provider, src)
	}
	return waitForLiveModelProbes(probeCtx, &wg, &results)
}

func waitForLiveModelProbes(ctx context.Context, wg *sync.WaitGroup, results *liveModelResults) map[string][]connectors.ModelSpec {
	completed := make(chan struct{})
	go func() {
		wg.Wait()
		close(completed)
	}()
	select {
	case <-completed:
		return results.models
	case <-ctx.Done():
		results.close()
		return results.models
	}
}

func (s *Server) fetchLiveModelsForProvider(ctx context.Context, tenantID, provider string, src connectors.LiveModelSource) []connectors.ModelSpec {
	accounts, err := s.accounts.ListByProvider(ctx, tenantID, provider)
	if err != nil {
		return nil
	}
	account, ok := firstUsableAccount(accounts)
	if !ok {
		return nil
	}
	creds, err := s.vault.Open(account)
	if err != nil {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	models, err := src.ListModels(probeCtx, creds)
	if err != nil {
		return nil
	}
	return models
}

func firstUsableAccount(accounts []store.Account) (store.Account, bool) {
	for _, account := range accounts {
		if !account.Disabled && !account.NeedsReconnect {
			return account, true
		}
	}
	return store.Account{}, false
}

func (s *Server) usableModelProviders(ctx context.Context, tenantID string) map[string]bool {
	usable := map[string]bool{}
	if s.accounts == nil {
		return usable
	}
	accs, err := s.accounts.ListByTenant(ctx, tenantID)
	if err != nil {
		return usable
	}
	for _, acc := range accs {
		if acc.Provider == "" || acc.Disabled || acc.NeedsReconnect {
			continue
		}
		usable[acc.Provider] = true
	}
	return usable
}

func appendModelEntry(data []modelEntry, seen map[string]struct{}, entry modelEntry) []modelEntry {
	if entry.ID == "" {
		return data
	}
	if _, ok := seen[entry.ID]; ok {
		return data
	}
	seen[entry.ID] = struct{}{}
	return append(data, entry)
}

// handleModelInfo serves GET /v1/models/info?id=<provider/model>: it returns
// metadata for a single model (kind, dimensions, provider, name).
func (s *Server) handleModelInfo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "id query parameter is required")
		return
	}

	provider, model, ok := strings.Cut(id, "/")
	if !ok || provider == "" || model == "" {
		writeError(w, http.StatusBadRequest, "id must be in provider/model form")
		return
	}
	key, _ := authedKey(r.Context())
	if !s.usableModelProviders(r.Context(), tenantOf(key))[provider] {
		writeError(w, http.StatusNotFound, "unknown model: "+id)
		return
	}

	spec, found := connectors.FindModel(provider, model)
	if !found {
		writeError(w, http.StatusNotFound, "unknown model: "+id)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         id,
		"provider":   provider,
		"model":      spec.ID,
		"name":       spec.Name,
		"kind":       string(spec.Kind),
		"dimensions": spec.Dimensions,
	})
}
