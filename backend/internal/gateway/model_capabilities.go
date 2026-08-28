package gateway

import (
	"github.com/mydisha/keirouter/backend/internal/capability"
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
