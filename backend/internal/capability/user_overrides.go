package capability

import (
	"encoding/json"
	"strings"
	"sync"
)

// SourceUser marks a capability resolution that came from an explicit
// user declaration (custom model capability ticks) rather than the built-in
// heuristic tables.
const SourceUser CapabilitySource = "user"

// UserCaps holds explicitly declared capabilities for a user-registered
// custom model. Nil pointers are unstated: they inherit the floor default.
// Stated false flags force a capability off, overriding heuristic tables
// that would otherwise claim it (e.g. a text-only gemma variant matching the
// generic *gemma* vision pattern).
type UserCaps struct {
	Vision           *bool `json:"vision,omitempty"`
	PDF              *bool `json:"pdf,omitempty"`
	AudioInput       *bool `json:"audio_input,omitempty"`
	VideoInput       *bool `json:"video_input,omitempty"`
	ImageOutput      *bool `json:"image_output,omitempty"`
	AudioOutput      *bool `json:"audio_output,omitempty"`
	Search           *bool `json:"search,omitempty"`
	Tools            *bool `json:"tools,omitempty"`
	Reasoning        *bool `json:"reasoning,omitempty"`
	StructuredOutput *bool `json:"structured_output,omitempty"`
}

// stated reports whether any field was explicitly set. Empty declarations carry
// no information, so they are dropped instead of shadowing heuristics.
func (u UserCaps) stated() bool {
	return u.Vision != nil || u.PDF != nil || u.AudioInput != nil || u.VideoInput != nil ||
		u.ImageOutput != nil || u.AudioOutput != nil || u.Search != nil || u.Tools != nil ||
		u.Reasoning != nil || u.StructuredOutput != nil
}

// merge applies every stated flag over p and returns the result. Both true and
// false stated flags win over whatever the floor or a heuristic set.
func (u UserCaps) merge(p Profile) Profile {
	if u.Vision != nil {
		p.Vision = *u.Vision
	}
	if u.PDF != nil {
		p.PDF = *u.PDF
	}
	if u.AudioInput != nil {
		p.AudioInput = *u.AudioInput
	}
	if u.VideoInput != nil {
		p.VideoInput = *u.VideoInput
	}
	if u.ImageOutput != nil {
		p.ImageOutput = *u.ImageOutput
	}
	if u.AudioOutput != nil {
		p.AudioOutput = *u.AudioOutput
	}
	if u.Search != nil {
		p.Search = *u.Search
	}
	if u.Tools != nil {
		p.Tools = *u.Tools
	}
	if u.Reasoning != nil {
		p.Reasoning = *u.Reasoning
	}
	if u.StructuredOutput != nil {
		p.StructuredOutput = *u.StructuredOutput
	}
	return p
}

// ParseUserCaps decodes a capabilities JSON object (capability name -> bool)
// as persisted on custom models. Unknown keys are ignored so older dashboards
// and newer servers stay compatible. Returns ok=false when nothing is stated.
func ParseUserCaps(data string) (UserCaps, bool) {
	if data == "" {
		return UserCaps{}, false
	}
	var raw map[string]bool
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return UserCaps{}, false
	}
	caps := UserCaps{}
	stated := false
	boolRef := func(key string, dst **bool) {
		if v, ok := raw[key]; ok {
			*dst = &v
			stated = true
		}
	}
	boolRef("vision", &caps.Vision)
	boolRef("pdf", &caps.PDF)
	boolRef("audio_input", &caps.AudioInput)
	boolRef("video_input", &caps.VideoInput)
	boolRef("image_output", &caps.ImageOutput)
	boolRef("audio_output", &caps.AudioOutput)
	boolRef("search", &caps.Search)
	boolRef("tools", &caps.Tools)
	boolRef("reasoning", &caps.Reasoning)
	boolRef("structured_output", &caps.StructuredOutput)
	if !stated {
		return UserCaps{}, false
	}
	return caps, true
}

// userOverride is one user-declared capability entry, parsed once at load time
// so the hot path never re-parses JSON.
type userOverride struct {
	model string // model id as registered on the provider
	caps  UserCaps
}

// The registry maps provider id -> user-declared model capabilities, mirroring
// the pattern of connectors.SetDynamicModels: in-memory state refreshed from
// the DB at startup and after every custom-model mutation.
var (
	userMu       sync.RWMutex
	userRegistry = map[string][]userOverride{}
)

// SetUserOverrides replaces the registry from a provider -> modelID -> JSON
// capabilities map. Called at startup from the store loader.
func SetUserOverrides(byProvider map[string]map[string]string) {
	userMu.Lock()
	defer userMu.Unlock()
	next := make(map[string][]userOverride, len(byProvider))
	for provider, models := range byProvider {
		var entries []userOverride
		for model, data := range models {
			caps, ok := ParseUserCaps(data)
			if !ok {
				continue
			}
			entries = append(entries, userOverride{model: model, caps: caps})
		}
		if len(entries) > 0 {
			next[provider] = entries
		}
	}
	userRegistry = next
}

// AddUserOverride sets the capability declaration for one custom model.
func AddUserOverride(provider, model string, caps UserCaps) {
	if !caps.stated() {
		RemoveUserOverride(provider, model)
		return
	}
	userMu.Lock()
	defer userMu.Unlock()
	entries := userRegistry[provider]
	for i := range entries {
		if entries[i].model == model {
			entries[i].caps = caps
			return
		}
	}
	userRegistry[provider] = append(entries, userOverride{model: model, caps: caps})
}

// RemoveUserOverride drops the declaration for one custom model, restoring
// heuristic resolution.
func RemoveUserOverride(provider, model string) {
	userMu.Lock()
	defer userMu.Unlock()
	entries := userRegistry[provider]
	for i := range entries {
		if entries[i].model == model {
			entries = append(entries[:i], entries[i+1:]...)
			if len(entries) == 0 {
				delete(userRegistry, provider)
			} else {
				userRegistry[provider] = entries
			}
			return
		}
	}
}

// lookupUserOverride returns the user declaration for (provider, model), trying
// the full id first and then the vendor-prefix-stripped suffix, matching
// Resolve's canonical-id step.
func lookupUserOverride(provider, model string) (UserCaps, bool) {
	userMu.RLock()
	defer userMu.RUnlock()
	entries := userRegistry[provider]
	for i := range entries {
		if entries[i].model == model {
			return entries[i].caps, true
		}
	}
	if i := strings.LastIndexByte(model, '/'); i >= 0 {
		base := model[i+1:]
		for i := range entries {
			if entries[i].model == base {
				return entries[i].caps, true
			}
		}
	}
	return UserCaps{}, false
}
