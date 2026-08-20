package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/custommodelconfig"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

type CustomModelCapabilityRepository struct {
	client *ent.Client
}

func NewCustomModelCapabilityRepository(client *ent.Client) service.CustomModelCapabilityResolver {
	return &CustomModelCapabilityRepository{client: client}
}

func (r *CustomModelCapabilityRepository) HasCapability(
	ctx context.Context,
	modelName string,
	capability string,
) (bool, error) {
	config, enabled, err := r.resolveConfig(ctx, modelName)
	if err != nil || !enabled || config == nil {
		return false, err
	}
	return hasCapability(config.Capabilities, capability), nil
}

func (r *CustomModelCapabilityRepository) ResolveVideoAPIType(
	ctx context.Context,
	modelName string,
) (string, bool, error) {
	config, enabled, err := r.resolveConfig(ctx, modelName)
	if err != nil || !enabled || config == nil {
		return "", false, err
	}
	if !hasCapability(config.Capabilities, "video") {
		return "", false, nil
	}
	videoAPIType := strings.ToLower(strings.TrimSpace(config.VideoAPIType))
	if videoAPIType == "" {
		return "", false, nil
	}
	return videoAPIType, true, nil
}

func (r *CustomModelCapabilityRepository) ResolveRequestAdapter(
	ctx context.Context,
	modelName string,
) (map[string]any, bool, error) {
	config, enabled, err := r.resolveConfig(ctx, modelName)
	if err != nil || !enabled || config == nil || config.TemplateID == nil {
		return nil, false, err
	}
	template, err := r.client.CustomModelRequestTemplate.Get(ctx, *config.TemplateID)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("query custom model request template: %w", err)
	}
	if len(template.RequestAdapter) == 0 {
		return nil, false, nil
	}
	return template.RequestAdapter, true, nil
}

func (r *CustomModelCapabilityRepository) resolveConfig(
	ctx context.Context,
	modelName string,
) (*ent.CustomModelConfig, bool, error) {
	enabled, err := r.client.Setting.Query().
		Where(setting.KeyEQ(service.SettingKeyCustomModelConfigEnabled)).
		Only(ctx)
	if ent.IsNotFound(err) || (err == nil && !strings.EqualFold(enabled.Value, "true")) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("query custom model config feature switch: %w", err)
	}

	config, err := r.client.CustomModelConfig.Query().
		Where(custommodelconfig.ModelNameEqualFold(strings.TrimSpace(modelName))).
		Only(ctx)
	if err == nil {
		return config, true, nil
	}
	if !ent.IsNotFound(err) {
		return nil, false, fmt.Errorf("query custom model config: %w", err)
	}

	normalizedModelName := strings.ToLower(strings.TrimSpace(modelName))
	prefixConfigs, err := r.client.CustomModelConfig.Query().
		Where(custommodelconfig.PrefixMatchEQ(true)).
		All(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("query custom model prefix config: %w", err)
	}

	var best *ent.CustomModelConfig
	for _, candidate := range prefixConfigs {
		prefix := strings.ToLower(strings.TrimSpace(candidate.ModelName))
		if prefix == "" || !strings.HasPrefix(normalizedModelName, prefix) {
			continue
		}
		if best == nil || len(prefix) > len(strings.TrimSpace(best.ModelName)) {
			best = candidate
		}
	}
	if best == nil {
		return nil, true, nil
	}
	return best, true, nil
}

func hasCapability(capabilities []string, capability string) bool {
	normalizedCapability := strings.TrimSpace(capability)
	for _, configured := range capabilities {
		if strings.EqualFold(strings.TrimSpace(configured), normalizedCapability) {
			return true
		}
	}
	return false
}
