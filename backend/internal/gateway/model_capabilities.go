package gateway

import (
	"github.com/mydisha/keirouter/backend/internal/capability"
	"github.com/mydisha/keirouter/backend/internal/connectors"
	"github.com/mydisha/keirouter/backend/internal/core"
)

type modelCapabilities struct {
	Vision           bool `json:"vision"`
	PDF              bool `json:"pdf"`
	AudioInput       bool `json:"audio_input"`
	VideoInput       bool `json:"video_input"`
	ImageOutput      bool `json:"image_output"`
	AudioOutput      bool `json:"audio_output"`
	Search           bool `json:"search"`
	Tools            bool `json:"tools"`
	Reasoning        bool `json:"reasoning"`
	StructuredOutput bool `json:"structured_output"`
	ContextWindow    int  `json:"context_window"`
	MaxOutput        int  `json:"max_output"`
}

func capabilityPayloadForModel(provider string, model connectors.ModelSpec, requestedKind core.ServiceKind) (modelCapabilities, capability.CapabilitySource) {
	profile, source := capabilityProfileForModel(provider, model, requestedKind)
	return modelCapabilitiesFromProfile(profile), source
}

func capabilityProfileForModel(provider string, model connectors.ModelSpec, requestedKind core.ServiceKind) (capability.Profile, capability.CapabilitySource) {
	if requestedKind != "" {
		resolution := capability.ResolveForServiceKind(provider, model.ID, requestedKind)
		return resolution.Profile, resolution.Source
	}

	profiles := make([]capability.Profile, 0, len(model.SupportedKinds()))
	resolution := capability.ResolveForServiceKind(provider, model.ID, model.Kind)
	for _, kind := range model.SupportedKinds() {
		profiles = append(profiles, capability.ResolveForServiceKind(provider, model.ID, kind).Profile)
	}
	return capability.MergeChainProfiles(profiles), resolution.Source
}

func capabilityPayload(provider, model string, kind core.ServiceKind) (modelCapabilities, capability.CapabilitySource) {
	r := capability.ResolveForServiceKind(provider, model, kind)
	p := r.Profile
	return modelCapabilities{
		Vision: p.Vision, PDF: p.PDF, AudioInput: p.AudioInput, VideoInput: p.VideoInput,
		ImageOutput: p.ImageOutput, AudioOutput: p.AudioOutput, Search: p.Search,
		Tools: p.Tools, Reasoning: p.Reasoning, StructuredOutput: p.StructuredOutput,
		ContextWindow: p.ContextWindow, MaxOutput: p.MaxOutput,
	}, r.Source
}
