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
	enabled, err := r.client.Setting.Query().
		Where(setting.KeyEQ(service.SettingKeyCustomModelConfigEnabled)).
		Only(ctx)
	if ent.IsNotFound(err) || (err == nil && !strings.EqualFold(enabled.Value, "true")) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query custom model config feature switch: %w", err)
	}

	config, err := r.client.CustomModelConfig.Query().
		Where(custommodelconfig.ModelNameEqualFold(strings.TrimSpace(modelName))).
		Only(ctx)
	if err == nil {
		return hasCapability(config.Capabilities, capability), nil
	}
	if !ent.IsNotFound(err) {
		return false, fmt.Errorf("query custom model capabilities: %w", err)
	}

	normalizedModelName := strings.ToLower(strings.TrimSpace(modelName))
	prefixConfigs, err := r.client.CustomModelConfig.Query().
		Where(custommodelconfig.PrefixMatchEQ(true)).
		All(ctx)
	if err != nil {
		return false, fmt.Errorf("query custom model prefix capabilities: %w", err)
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
		return false, nil
	}
	return hasCapability(best.Capabilities, capability), nil
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
