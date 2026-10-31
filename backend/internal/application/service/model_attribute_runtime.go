package service

import (
	"context"
	"strings"
)

type compiledModelAttributeFields struct {
	fields []ModelAttributeField
	err    error
}

// ResolveModelAttributeFields returns immutable fields compiled with the same
// runtime snapshot as model matching. Request validation never reparses template
// JSON or revalidates all declarations on the gateway hot path.
func (s *CustomModelConfigService) ResolveModelAttributeFields(ctx context.Context, model, modality string) ([]ModelAttributeField, error) {
	runtime, err := s.runtime(ctx)
	if err != nil || runtime == nil || !runtime.enabled {
		return nil, err
	}
	config := runtime.match(model)
	if config == nil || config.TemplateID == nil {
		return nil, nil
	}
	compiled := runtime.attributes[*config.TemplateID][strings.ToLower(strings.TrimSpace(modality))]
	return compiled.fields, compiled.err
}

func resolveModelAttributeFields(ctx context.Context, resolver CustomModelCapabilityResolver, model, modality string) ([]ModelAttributeField, error) {
	if compiled, ok := resolver.(interface {
		ResolveModelAttributeFields(context.Context, string, string) ([]ModelAttributeField, error)
	}); ok {
		return compiled.ResolveModelAttributeFields(ctx, model, modality)
	}
	attributes, configured, err := resolver.ResolveModelAttributes(ctx, model)
	if err != nil || !configured {
		return nil, err
	}
	return modelAttributeFields(attributes, modality)
}
