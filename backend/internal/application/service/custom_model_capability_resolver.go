package service

import "context"

// CustomModelCapabilityResolver resolves administrator-defined model
// capabilities without coupling application services to persistence details.
type CustomModelCapabilityResolver interface {
	HasCapability(ctx context.Context, modelName, capability string) (bool, error)
	ResolveRequestAdapter(ctx context.Context, modelName string) (map[string]any, bool, error)
}
