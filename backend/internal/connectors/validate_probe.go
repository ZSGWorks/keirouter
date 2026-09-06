package connectors

import (
	"context"

	"github.com/mydisha/keirouter/backend/internal/core"
)

// ProbeModelFor resolves a real model id to use for credential validation
// probes (e.g. the minimal chat request in Validate). Resolution order:
// models.dev snapshot LLM first, then user-registered custom LLM models.
// It returns "" when no model is known; callers must skip model-bound probes
// rather than sending an invented model id.
func ProbeModelFor(provider string) string {
	for _, mdl := range LLMModelsForProvider(provider) {
		if mdl.Kind == core.ServiceLLM {
			return mdl.ID
		}
	}
	return ""
}

// ProbeModelForCredentials returns a discovered probe model from the cached
// catalog or, when necessary, the provider's live model endpoint. It never
// invents a model id: callers skip model-bound probes when both sources fail.
func ProbeModelForCredentials(ctx context.Context, provider string, creds core.Credentials, source LiveModelSource) string {
	if model := ProbeModelFor(provider); model != "" {
		return model
	}
	models, err := source.ListModels(ctx, creds)
	if err != nil {
		return ""
	}
	for _, model := range models {
		if model.SupportsKind(core.ServiceLLM) && model.ID != "" {
			return model.ID
		}
	}
	return ""
}
