package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/custommodelconfig"
	"github.com/Wei-Shaw/sub2api/ent/custommodelrequesttemplate"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

type customModelConfigRepository struct {
	client *ent.Client
}

func NewCustomModelConfigRepository(client *ent.Client) service.CustomModelConfigRepository {
	return &customModelConfigRepository{client: client}
}

func (r *customModelConfigRepository) LoadRuntimeSnapshot(
	ctx context.Context,
) (*service.CustomModelRuntimeSnapshot, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("custom model config repository is unavailable")
	}
	enabledSetting, err := r.client.Setting.Query().
		Where(setting.KeyEQ(service.SettingKeyCustomModelConfigEnabled)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return emptyCustomModelRuntimeSnapshot(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("query custom model config feature switch: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(enabledSetting.Value), "true") {
		return emptyCustomModelRuntimeSnapshot(), nil
	}

	configs, err := r.client.CustomModelConfig.Query().
		Order(ent.Asc(custommodelconfig.FieldModelName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list custom model configs for runtime: %w", err)
	}
	templates, err := r.client.CustomModelRequestTemplate.Query().
		Order(ent.Asc(custommodelrequesttemplate.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list custom model templates for runtime: %w", err)
	}
	templateMap := make(map[int64]service.CustomModelRequestTemplate, len(templates))
	for _, item := range templates {
		mapped := customModelTemplateFromEnt(item)
		templateMap[mapped.ID] = mapped
	}
	result := &service.CustomModelRuntimeSnapshot{
		Enabled:   true,
		Configs:   make([]service.CustomModelConfig, 0, len(configs)),
		Templates: templateMap,
	}
	for _, item := range configs {
		result.Configs = append(result.Configs, customModelConfigFromEnt(item, templateMap))
	}
	return result, nil
}

func (r *customModelConfigRepository) ListConfigs(
	ctx context.Context,
) ([]service.CustomModelConfig, error) {
	configs, err := r.client.CustomModelConfig.Query().
		Order(ent.Asc(custommodelconfig.FieldModelName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list custom model configs: %w", err)
	}
	templates, err := r.client.CustomModelRequestTemplate.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list custom model templates: %w", err)
	}
	templateMap := make(map[int64]service.CustomModelRequestTemplate, len(templates))
	for _, item := range templates {
		mapped := customModelTemplateFromEnt(item)
		templateMap[mapped.ID] = mapped
	}
	result := make([]service.CustomModelConfig, 0, len(configs))
	for _, item := range configs {
		result = append(result, customModelConfigFromEnt(item, templateMap))
	}
	return result, nil
}

func (r *customModelConfigRepository) GetConfig(
	ctx context.Context,
	id int64,
) (*service.CustomModelConfig, error) {
	item, err := r.client.CustomModelConfig.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, service.ErrCustomModelConfigNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get custom model config: %w", err)
	}
	templates := map[int64]service.CustomModelRequestTemplate{}
	if item.TemplateID != nil {
		template, templateErr := r.client.CustomModelRequestTemplate.Get(ctx, *item.TemplateID)
		if templateErr == nil {
			mapped := customModelTemplateFromEnt(template)
			templates[mapped.ID] = mapped
		} else if !ent.IsNotFound(templateErr) {
			return nil, fmt.Errorf("get custom model config template: %w", templateErr)
		}
	}
	mapped := customModelConfigFromEnt(item, templates)
	return &mapped, nil
}

func (r *customModelConfigRepository) CreateConfig(
	ctx context.Context,
	input service.CreateCustomModelConfigInput,
) (*service.CustomModelConfig, error) {
	builder := r.client.CustomModelConfig.Create().
		SetModelName(input.ModelName).
		SetPrefixMatch(input.PrefixMatch).
		SetCapabilities(input.Capabilities).
		SetVideoAPIType(input.VideoAPIType).
		SetNillableTemplateID(input.TemplateID)
	item, err := builder.Save(ctx)
	if ent.IsConstraintError(err) {
		return nil, service.ErrCustomModelConfigDuplicate
	}
	if err != nil {
		return nil, fmt.Errorf("create custom model config: %w", err)
	}
	return r.GetConfig(ctx, item.ID)
}

func (r *customModelConfigRepository) UpdateConfig(
	ctx context.Context,
	id int64,
	input service.UpdateCustomModelConfigInput,
) (*service.CustomModelConfig, error) {
	builder := r.client.CustomModelConfig.UpdateOneID(id)
	if input.PrefixMatch != nil {
		builder.SetPrefixMatch(*input.PrefixMatch)
	}
	if input.Capabilities != nil {
		builder.SetCapabilities(*input.Capabilities)
	}
	if input.VideoAPIType != nil {
		builder.SetVideoAPIType(*input.VideoAPIType)
	}
	if input.TemplateIDSet {
		if input.TemplateID == nil {
			builder.ClearTemplateID()
		} else {
			builder.SetTemplateID(*input.TemplateID)
		}
	}
	if _, err := builder.Save(ctx); ent.IsNotFound(err) {
		return nil, service.ErrCustomModelConfigNotFound
	} else if ent.IsConstraintError(err) {
		return nil, service.ErrCustomModelConfigDuplicate
	} else if err != nil {
		return nil, fmt.Errorf("update custom model config: %w", err)
	}
	return r.GetConfig(ctx, id)
}

func (r *customModelConfigRepository) DeleteConfig(ctx context.Context, id int64) error {
	err := r.client.CustomModelConfig.DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return service.ErrCustomModelConfigNotFound
	}
	if err != nil {
		return fmt.Errorf("delete custom model config: %w", err)
	}
	return nil
}

func (r *customModelConfigRepository) ListTemplates(
	ctx context.Context,
) ([]service.CustomModelRequestTemplate, error) {
	items, err := r.client.CustomModelRequestTemplate.Query().
		Order(ent.Asc(custommodelrequesttemplate.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list custom model templates: %w", err)
	}
	result := make([]service.CustomModelRequestTemplate, 0, len(items))
	for _, item := range items {
		result = append(result, customModelTemplateFromEnt(item))
	}
	return result, nil
}

func (r *customModelConfigRepository) GetTemplate(
	ctx context.Context,
	id int64,
) (*service.CustomModelRequestTemplate, error) {
	item, err := r.client.CustomModelRequestTemplate.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, service.ErrCustomModelTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get custom model template: %w", err)
	}
	mapped := customModelTemplateFromEnt(item)
	return &mapped, nil
}

func (r *customModelConfigRepository) CreateTemplate(
	ctx context.Context,
	input service.CreateCustomModelRequestTemplateInput,
) (*service.CustomModelRequestTemplate, error) {
	item, err := r.client.CustomModelRequestTemplate.Create().
		SetName(input.Name).
		SetDescription(input.Description).
		SetRequestAdapter(input.RequestAdapter).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return nil, service.ErrCustomModelTemplateDuplicate
	}
	if err != nil {
		return nil, fmt.Errorf("create custom model template: %w", err)
	}
	mapped := customModelTemplateFromEnt(item)
	return &mapped, nil
}

func (r *customModelConfigRepository) UpdateTemplate(
	ctx context.Context,
	id int64,
	input service.UpdateCustomModelRequestTemplateInput,
) (*service.CustomModelRequestTemplate, error) {
	builder := r.client.CustomModelRequestTemplate.UpdateOneID(id)
	if input.Name != nil {
		builder.SetName(*input.Name)
	}
	if input.Description != nil {
		builder.SetDescription(*input.Description)
	}
	if input.RequestAdapter != nil {
		builder.SetRequestAdapter(*input.RequestAdapter)
	}
	item, err := builder.Save(ctx)
	if ent.IsNotFound(err) {
		return nil, service.ErrCustomModelTemplateNotFound
	}
	if ent.IsConstraintError(err) {
		return nil, service.ErrCustomModelTemplateDuplicate
	}
	if err != nil {
		return nil, fmt.Errorf("update custom model template: %w", err)
	}
	mapped := customModelTemplateFromEnt(item)
	return &mapped, nil
}

func (r *customModelConfigRepository) DeleteTemplate(ctx context.Context, id int64) error {
	err := r.client.CustomModelRequestTemplate.DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		return service.ErrCustomModelTemplateNotFound
	}
	if err != nil {
		return fmt.Errorf("delete custom model template: %w", err)
	}
	return nil
}

func emptyCustomModelRuntimeSnapshot() *service.CustomModelRuntimeSnapshot {
	return &service.CustomModelRuntimeSnapshot{
		Configs:   []service.CustomModelConfig{},
		Templates: map[int64]service.CustomModelRequestTemplate{},
	}
}

func customModelConfigFromEnt(
	item *ent.CustomModelConfig,
	templates map[int64]service.CustomModelRequestTemplate,
) service.CustomModelConfig {
	mapped := service.CustomModelConfig{
		ID:           item.ID,
		ModelName:    item.ModelName,
		PrefixMatch:  item.PrefixMatch,
		Capabilities: append([]string(nil), item.Capabilities...),
		VideoAPIType: item.VideoAPIType,
		TemplateID:   item.TemplateID,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
	if item.TemplateID != nil {
		mapped.TemplateName = templates[*item.TemplateID].Name
	}
	return mapped
}

func customModelTemplateFromEnt(item *ent.CustomModelRequestTemplate) service.CustomModelRequestTemplate {
	adapter := item.RequestAdapter
	if adapter == nil {
		adapter = map[string]any{}
	}
	return service.CustomModelRequestTemplate{
		ID:             item.ID,
		Name:           item.Name,
		Description:    item.Description,
		RequestAdapter: adapter,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}
