package xai

import "strings"

// Model describes an xAI model in OpenAI-compatible /models shape.
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created,omitempty"`
	OwnedBy     string `json:"owned_by"`
	DisplayName string `json:"display_name,omitempty"`
}

const DefaultTextModel = "grok-4.5"

var defaultModels = []Model{
	{ID: "grok-4.6", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.6"},
	{ID: "grok-4.5", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.5"},
	{ID: "grok-4.3", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.3"},
	{ID: "grok-build-0.1", Object: "model", OwnedBy: "xai", DisplayName: "Grok Build 0.1"},
	{ID: "grok-composer-2.5-fast", Object: "model", OwnedBy: "xai", DisplayName: "Grok Composer 2.5 Fast"},
	{ID: "grok-4.20-0309-reasoning", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Reasoning"},
	{ID: "grok-4.20-0309-non-reasoning", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Non Reasoning"},
	{ID: "grok-4.20-multi-agent-0309", Object: "model", OwnedBy: "xai", DisplayName: "Grok 4.20 Multi Agent"},
	{ID: "grok-imagine", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine"},
	{ID: "grok-imagine-image", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Image"},
	{ID: "grok-imagine-image-quality", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Image Quality"},
	{ID: "grok-imagine-edit", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Edit"},
	{ID: "grok-imagine-video", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video"},
	{ID: "grok-imagine-video-1.5", Object: "model", OwnedBy: "xai", DisplayName: "Grok Imagine Video 1.5"},
}

func DefaultModels() []Model {
	out := make([]Model, len(defaultModels))
	copy(out, defaultModels)
	return out
}

func DefaultModelIDs() []string {
	models := DefaultModels()
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

func DefaultModelMapping() map[string]string {
	mapping := make(map[string]string, len(defaultModels)+13)
	for _, model := range defaultModels {
		mapping[model.ID] = model.ID
	}
	mapping["grok"] = DefaultTextModel
	mapping["grok-latest"] = DefaultTextModel
	mapping["grok-4.6-latest"] = "grok-4.6"
	mapping["grok-4.5-latest"] = DefaultTextModel
	mapping["grok-4.3-latest"] = "grok-4.3"
	mapping["grok-build"] = "grok-build-0.1"
	mapping["grok-build-latest"] = DefaultTextModel
	mapping["grok-composer"] = "grok-composer-2.5-fast"
	mapping["composer-2.5"] = "grok-composer-2.5-fast"
	mapping["grok-4.20-reasoning"] = "grok-4.20-0309-reasoning"
	mapping["grok-4.20-non-reasoning"] = "grok-4.20-0309-non-reasoning"
	mapping["grok-4.20-multi-agent"] = "grok-4.20-multi-agent-0309"
	mapping["grok-4.20-multi-agent-latest"] = "grok-4.20-multi-agent-0309"
	return mapping
}

// StripGrokProviderPrefix removes provider prefixes accepted for xAI models
// while leaving unrelated slash-containing model IDs untouched.
func StripGrokProviderPrefix(model string) string {
	trimmed := strings.TrimSpace(model)
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"xai/", "x-ai/", "grok/"} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	return trimmed
}

// ResolveGrokTextResponsesModelID canonicalizes known Grok text aliases while
// preserving unknown native IDs for account-level mappings.
func ResolveGrokTextResponsesModelID(model string, defaultText ...string) string {
	fallback := DefaultTextModel
	if len(defaultText) > 0 {
		if candidate := strings.TrimSpace(defaultText[0]); candidate != "" {
			fallback = candidate
		}
	}

	normalized := strings.ToLower(StripGrokProviderPrefix(model))
	switch normalized {
	case "":
		return fallback
	case "grok", "grok-latest", "grok-4.5", "grok-4.5-latest", "grok-build-latest":
		return fallback
	case "grok-4.6", "grok-4.6-latest":
		return "grok-4.6"
	case "grok-4.3", "grok-4.3-latest":
		return "grok-4.3"
	case "grok-build", "grok-build-0.1":
		return "grok-build-0.1"
	case "grok-composer", "composer-2.5", "grok-composer-2.5-fast":
		return "grok-composer-2.5-fast"
	case "grok-4.20-reasoning", "grok-4.20-0309-reasoning":
		return "grok-4.20-0309-reasoning"
	case "grok-4.20-non-reasoning", "grok-4.20-0309-non-reasoning":
		return "grok-4.20-0309-non-reasoning"
	case "grok-4.20-multi-agent", "grok-4.20-multi-agent-latest", "grok-4.20-multi-agent-0309":
		return "grok-4.20-multi-agent-0309"
	default:
		return StripGrokProviderPrefix(model)
	}
}
