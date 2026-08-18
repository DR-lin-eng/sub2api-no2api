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
	if ent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query custom model capabilities: %w", err)
	}

	for _, configured := range config.Capabilities {
		if strings.EqualFold(strings.TrimSpace(configured), strings.TrimSpace(capability)) {
			return true, nil
		}
	}
	return false, nil
}
