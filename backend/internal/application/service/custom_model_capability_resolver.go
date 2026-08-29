package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const customModelRuntimeCacheTTL = 5 * time.Second

var (
	ErrCustomModelConfigNotFound = infraerrors.NotFound(
		"CUSTOM_MODEL_CONFIG_NOT_FOUND", "custom model config not found",
	)
	ErrCustomModelConfigDuplicate = infraerrors.Conflict(
		"CUSTOM_MODEL_CONFIG_DUPLICATE", "custom model config already exists",
	)
	ErrCustomModelConfigInvalid = infraerrors.BadRequest(
		"CUSTOM_MODEL_CONFIG_INVALID", "custom model config is invalid",
	)
	ErrCustomModelTemplateNotFound = infraerrors.NotFound(
		"CUSTOM_MODEL_TEMPLATE_NOT_FOUND", "custom model request template not found",
	)
	ErrCustomModelTemplateDuplicate = infraerrors.Conflict(
		"CUSTOM_MODEL_TEMPLATE_DUPLICATE", "custom model request template already exists",
	)
)

// CustomModelCapabilityResolver is the read-only gateway-facing contract.
// Implementations must not perform a database query for every model request.
type CustomModelCapabilityResolver interface {
	HasCapability(ctx context.Context, modelName, capability string) (bool, error)
	ResolveVideoAPIType(ctx context.Context, modelName string) (string, bool, error)
	ResolveRequestAdapter(ctx context.Context, modelName string) (map[string]any, bool, error)
}

type CustomModelConfig struct {
	ID           int64
	ModelName    string
	PrefixMatch  bool
	Capabilities []string
	VideoAPIType string
	TemplateID   *int64
	TemplateName string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CustomModelRequestTemplate struct {
	ID             int64
	Name           string
	Description    string
	RequestAdapter map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CustomModelRuntimeSnapshot struct {
	Enabled   bool
	Configs   []CustomModelConfig
	Templates map[int64]CustomModelRequestTemplate
}

type CreateCustomModelConfigInput struct {
	ModelName    string
	PrefixMatch  bool
	Capabilities []string
	VideoAPIType string
	TemplateID   *int64
}

type UpdateCustomModelConfigInput struct {
	PrefixMatch   *bool
	Capabilities  *[]string
	VideoAPIType  *string
	TemplateID    *int64
	TemplateIDSet bool
}

type CreateCustomModelRequestTemplateInput struct {
	Name           string
	Description    string
	RequestAdapter map[string]any
}

type UpdateCustomModelRequestTemplateInput struct {
	Name           *string
	Description    *string
	RequestAdapter *map[string]any
}

type CustomModelConfigRepository interface {
	LoadRuntimeSnapshot(ctx context.Context) (*CustomModelRuntimeSnapshot, error)
	ListConfigs(ctx context.Context) ([]CustomModelConfig, error)
	GetConfig(ctx context.Context, id int64) (*CustomModelConfig, error)
	CreateConfig(ctx context.Context, input CreateCustomModelConfigInput) (*CustomModelConfig, error)
	UpdateConfig(ctx context.Context, id int64, input UpdateCustomModelConfigInput) (*CustomModelConfig, error)
	DeleteConfig(ctx context.Context, id int64) error
	ListTemplates(ctx context.Context) ([]CustomModelRequestTemplate, error)
	GetTemplate(ctx context.Context, id int64) (*CustomModelRequestTemplate, error)
	CreateTemplate(ctx context.Context, input CreateCustomModelRequestTemplateInput) (*CustomModelRequestTemplate, error)
	UpdateTemplate(ctx context.Context, id int64, input UpdateCustomModelRequestTemplateInput) (*CustomModelRequestTemplate, error)
	DeleteTemplate(ctx context.Context, id int64) error
}

type compiledCustomModelRuntime struct {
	enabled   bool
	configs   []CustomModelConfig
	exact     map[string]*CustomModelConfig
	prefixes  []*CustomModelConfig
	templates map[int64]CustomModelRequestTemplate
}

type CustomModelConfigService struct {
	repo CustomModelConfigRepository

	cacheMu      sync.RWMutex
	cache        *compiledCustomModelRuntime
	cacheExpires time.Time
	cacheTTL     time.Duration
	now          func() time.Time
}

func NewCustomModelConfigService(repo CustomModelConfigRepository) *CustomModelConfigService {
	return &CustomModelConfigService{repo: repo, cacheTTL: customModelRuntimeCacheTTL, now: time.Now}
}

func (s *CustomModelConfigService) List(ctx context.Context) ([]CustomModelConfig, error) {
	return s.repo.ListConfigs(ctx)
}

func (s *CustomModelConfigService) ListEnabled(ctx context.Context) ([]CustomModelConfig, error) {
	runtime, err := s.runtime(ctx)
	if err != nil || runtime == nil || !runtime.enabled {
		return []CustomModelConfig{}, err
	}
	result := make([]CustomModelConfig, 0, len(runtime.configs))
	for _, config := range runtime.configs {
		result = append(result, cloneCustomModelConfig(config))
	}
	return result, nil
}

func (s *CustomModelConfigService) Get(ctx context.Context, id int64) (*CustomModelConfig, error) {
	if id <= 0 {
		return nil, ErrCustomModelConfigInvalid
	}
	return s.repo.GetConfig(ctx, id)
}

func (s *CustomModelConfigService) Create(
	ctx context.Context,
	input CreateCustomModelConfigInput,
) (*CustomModelConfig, error) {
	var err error
	input.ModelName, input.Capabilities, input.VideoAPIType, err = normalizeCustomModelConfig(
		input.ModelName, input.Capabilities, input.VideoAPIType,
	)
	if err != nil {
		return nil, err
	}
	if input.TemplateID != nil {
		if *input.TemplateID <= 0 {
			return nil, ErrCustomModelConfigInvalid
		}
		if _, err := s.repo.GetTemplate(ctx, *input.TemplateID); err != nil {
			return nil, err
		}
	}
	configs, err := s.repo.ListConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for _, existing := range configs {
		if strings.EqualFold(existing.ModelName, input.ModelName) {
			return nil, ErrCustomModelConfigDuplicate
		}
	}
	created, err := s.repo.CreateConfig(ctx, input)
	if err == nil {
		s.InvalidateRuntimeCache()
	}
	return created, err
}

func (s *CustomModelConfigService) Update(
	ctx context.Context,
	id int64,
	input UpdateCustomModelConfigInput,
) (*CustomModelConfig, error) {
	if id <= 0 {
		return nil, ErrCustomModelConfigInvalid
	}
	current, err := s.repo.GetConfig(ctx, id)
	if err != nil {
		return nil, err
	}
	capabilities := current.Capabilities
	if input.Capabilities != nil {
		capabilities = *input.Capabilities
	}
	videoAPIType := current.VideoAPIType
	if input.VideoAPIType != nil {
		videoAPIType = *input.VideoAPIType
	}
	_, capabilities, videoAPIType, err = normalizeCustomModelConfig(
		current.ModelName, capabilities, videoAPIType,
	)
	if err != nil {
		return nil, err
	}
	input.Capabilities = &capabilities
	input.VideoAPIType = &videoAPIType
	if input.TemplateIDSet && input.TemplateID != nil {
		if *input.TemplateID <= 0 {
			return nil, ErrCustomModelConfigInvalid
		}
		if _, err := s.repo.GetTemplate(ctx, *input.TemplateID); err != nil {
			return nil, err
		}
	}
	updated, err := s.repo.UpdateConfig(ctx, id, input)
	if err == nil {
		s.InvalidateRuntimeCache()
	}
	return updated, err
}

func (s *CustomModelConfigService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrCustomModelConfigInvalid
	}
	err := s.repo.DeleteConfig(ctx, id)
	if err == nil {
		s.InvalidateRuntimeCache()
	}
	return err
}

func (s *CustomModelConfigService) ListTemplates(ctx context.Context) ([]CustomModelRequestTemplate, error) {
	return s.repo.ListTemplates(ctx)
}

func (s *CustomModelConfigService) CreateTemplate(
	ctx context.Context,
	input CreateCustomModelRequestTemplateInput,
) (*CustomModelRequestTemplate, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || len(input.Name) > 100 || len(input.Description) > 500 {
		return nil, ErrCustomModelConfigInvalid
	}
	if err := validateCustomModelRequestAdapter(input.RequestAdapter); err != nil {
		return nil, err
	}
	items, err := s.repo.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	for _, existing := range items {
		if strings.EqualFold(existing.Name, input.Name) {
			return nil, ErrCustomModelTemplateDuplicate
		}
	}
	created, err := s.repo.CreateTemplate(ctx, input)
	if err == nil {
		s.InvalidateRuntimeCache()
	}
	return created, err
}

func (s *CustomModelConfigService) UpdateTemplate(
	ctx context.Context,
	id int64,
	input UpdateCustomModelRequestTemplateInput,
) (*CustomModelRequestTemplate, error) {
	if id <= 0 {
		return nil, ErrCustomModelConfigInvalid
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" || len(name) > 100 {
			return nil, ErrCustomModelConfigInvalid
		}
		input.Name = &name
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		if len(description) > 500 {
			return nil, ErrCustomModelConfigInvalid
		}
		input.Description = &description
	}
	if input.RequestAdapter != nil {
		if err := validateCustomModelRequestAdapter(*input.RequestAdapter); err != nil {
			return nil, err
		}
	}
	if input.Name != nil {
		items, err := s.repo.ListTemplates(ctx)
		if err != nil {
			return nil, err
		}
		for _, existing := range items {
			if existing.ID != id && strings.EqualFold(existing.Name, *input.Name) {
				return nil, ErrCustomModelTemplateDuplicate
			}
		}
	}
	updated, err := s.repo.UpdateTemplate(ctx, id, input)
	if err == nil {
		s.InvalidateRuntimeCache()
	}
	return updated, err
}

func (s *CustomModelConfigService) DeleteTemplate(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrCustomModelConfigInvalid
	}
	err := s.repo.DeleteTemplate(ctx, id)
	if err == nil {
		s.InvalidateRuntimeCache()
	}
	return err
}

func (s *CustomModelConfigService) HasCapability(
	ctx context.Context,
	modelName string,
	capability string,
) (bool, error) {
	config, enabled, err := s.resolveConfig(ctx, modelName)
	if err != nil || !enabled || config == nil {
		return false, err
	}
	return hasCustomModelCapability(config.Capabilities, capability), nil
}

func (s *CustomModelConfigService) ResolveVideoAPIType(
	ctx context.Context,
	modelName string,
) (string, bool, error) {
	config, enabled, err := s.resolveConfig(ctx, modelName)
	if err != nil || !enabled || config == nil {
		return "", false, err
	}
	if !hasCustomModelCapability(config.Capabilities, "video") || config.VideoAPIType == "" {
		return "", false, nil
	}
	return config.VideoAPIType, true, nil
}

func (s *CustomModelConfigService) ResolveRequestAdapter(
	ctx context.Context,
	modelName string,
) (map[string]any, bool, error) {
	runtime, err := s.runtime(ctx)
	if err != nil || runtime == nil || !runtime.enabled {
		return nil, false, err
	}
	config := runtime.match(modelName)
	if config == nil || config.TemplateID == nil {
		return nil, false, nil
	}
	template, ok := runtime.templates[*config.TemplateID]
	if !ok || len(template.RequestAdapter) == 0 {
		return nil, false, nil
	}
	// Runtime snapshots are immutable after compilation. The gateway adapter only
	// reads this map, so avoid a JSON round trip on every image request.
	return template.RequestAdapter, true, nil
}

func (s *CustomModelConfigService) resolveConfig(
	ctx context.Context,
	modelName string,
) (*CustomModelConfig, bool, error) {
	runtime, err := s.runtime(ctx)
	if err != nil || runtime == nil || !runtime.enabled {
		return nil, false, err
	}
	return runtime.match(modelName), true, nil
}

func (s *CustomModelConfigService) runtime(ctx context.Context) (*compiledCustomModelRuntime, error) {
	now := s.now()
	s.cacheMu.RLock()
	if s.cache != nil && now.Before(s.cacheExpires) {
		cached := s.cache
		s.cacheMu.RUnlock()
		return cached, nil
	}
	s.cacheMu.RUnlock()

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	now = s.now()
	if s.cache != nil && now.Before(s.cacheExpires) {
		return s.cache, nil
	}
	snapshot, err := s.repo.LoadRuntimeSnapshot(ctx)
	if err != nil {
		if s.cache != nil {
			s.cacheExpires = now.Add(time.Second)
			return s.cache, nil
		}
		return nil, err
	}
	compiled := compileCustomModelRuntime(snapshot)
	s.cache = compiled
	s.cacheExpires = now.Add(s.cacheTTL)
	return compiled, nil
}

func (s *CustomModelConfigService) InvalidateRuntimeCache() {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	s.cacheExpires = time.Time{}
	s.cacheMu.Unlock()
}

func (r *compiledCustomModelRuntime) match(modelName string) *CustomModelConfig {
	if r == nil || !r.enabled {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(modelName))
	if key == "" {
		return nil
	}
	if config, ok := r.exact[key]; ok {
		return config
	}
	for _, config := range r.prefixes {
		if strings.HasPrefix(key, config.ModelName) {
			return config
		}
	}
	return nil
}

func compileCustomModelRuntime(snapshot *CustomModelRuntimeSnapshot) *compiledCustomModelRuntime {
	runtime := &compiledCustomModelRuntime{
		exact: map[string]*CustomModelConfig{}, templates: map[int64]CustomModelRequestTemplate{},
	}
	if snapshot == nil {
		return runtime
	}
	runtime.enabled = snapshot.Enabled
	for id, template := range snapshot.Templates {
		template.RequestAdapter = cloneCustomModelJSONMap(template.RequestAdapter)
		runtime.templates[id] = template
	}
	for _, config := range snapshot.Configs {
		config = cloneCustomModelConfig(config)
		runtime.configs = append(runtime.configs, cloneCustomModelConfig(config))
		key := strings.ToLower(strings.TrimSpace(config.ModelName))
		if key == "" {
			continue
		}
		config.ModelName = key
		compiledConfig := config
		if config.PrefixMatch {
			runtime.prefixes = append(runtime.prefixes, &compiledConfig)
			continue
		}
		runtime.exact[key] = &compiledConfig
	}
	sort.SliceStable(runtime.prefixes, func(i, j int) bool {
		return len(runtime.prefixes[i].ModelName) > len(runtime.prefixes[j].ModelName)
	})
	return runtime
}

func normalizeCustomModelConfig(modelName string, capabilities []string, videoAPIType string) (string, []string, string, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || len(modelName) > 255 {
		return "", nil, "", ErrCustomModelConfigInvalid
	}
	seen := make(map[string]struct{}, len(capabilities))
	normalized := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.ToLower(strings.TrimSpace(capability))
		switch capability {
		case "image", "video", "audio":
		default:
			return "", nil, "", ErrCustomModelConfigInvalid
		}
		if _, ok := seen[capability]; ok {
			continue
		}
		seen[capability] = struct{}{}
		normalized = append(normalized, capability)
	}
	if len(normalized) == 0 {
		return "", nil, "", ErrCustomModelConfigInvalid
	}
	videoAPIType = strings.ToLower(strings.TrimSpace(videoAPIType))
	if _, hasVideo := seen["video"]; !hasVideo {
		videoAPIType = ""
	} else if videoAPIType != "" && videoAPIType != "grok" && videoAPIType != "agnes" {
		return "", nil, "", ErrCustomModelConfigInvalid
	}
	return modelName, normalized, videoAPIType, nil
}

func hasCustomModelCapability(capabilities []string, capability string) bool {
	capability = strings.ToLower(strings.TrimSpace(capability))
	for _, configured := range capabilities {
		if strings.EqualFold(strings.TrimSpace(configured), capability) {
			return true
		}
	}
	return false
}

func cloneCustomModelConfig(config CustomModelConfig) CustomModelConfig {
	config.Capabilities = append([]string(nil), config.Capabilities...)
	if config.TemplateID != nil {
		id := *config.TemplateID
		config.TemplateID = &id
	}
	return config
}

func cloneCustomModelJSONMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil || cloned == nil {
		return map[string]any{}
	}
	return cloned
}
