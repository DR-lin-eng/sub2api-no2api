package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"

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
	ResolveModelAttributes(ctx context.Context, modelName string) (map[string]any, bool, error)
	ResolveMediaLimits(ctx context.Context, modelName string) (map[string]any, bool, error)
	ValidateVideoRequest(ctx context.Context, modelName string, body []byte) error
}

type CustomModelConfig struct {
	ID           int64
	ModelName    string
	PrefixMatch  bool
	Capabilities []string
	VideoAPIType string
	TemplateID   *int64
	TemplateName string
	MediaLimits  map[string]any
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CustomModelRequestTemplate struct {
	ID             int64
	Name           string
	Description    string
	RequestAdapter map[string]any
	Attributes     map[string]any
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
	MediaLimits  map[string]any
}

type UpdateCustomModelConfigInput struct {
	PrefixMatch    *bool
	Capabilities   *[]string
	VideoAPIType   *string
	TemplateID     *int64
	TemplateIDSet  bool
	MediaLimits    map[string]any
	MediaLimitsSet bool
}

type CreateCustomModelRequestTemplateInput struct {
	Name           string
	Description    string
	RequestAdapter map[string]any
	Attributes     map[string]any
}

type UpdateCustomModelRequestTemplateInput struct {
	Name           *string
	Description    *string
	RequestAdapter *map[string]any
	Attributes     *map[string]any
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
	mediaLimits, err := normalizeCustomModelMediaLimits(input.MediaLimits)
	if err != nil {
		return nil, err
	}
	input.MediaLimits = mediaLimits
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
	mediaLimits := current.MediaLimits
	if input.MediaLimitsSet {
		mediaLimits = input.MediaLimits
	}
	mediaLimits, err = normalizeCustomModelMediaLimits(mediaLimits)
	if err != nil {
		return nil, err
	}
	input.MediaLimits = mediaLimits
	input.MediaLimitsSet = true
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
	attributes, err := normalizeModelAttributes(input.Attributes)
	if err != nil {
		return nil, err
	}
	input.Attributes = attributes
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
	if input.Attributes != nil {
		normalized, err := normalizeModelAttributes(*input.Attributes)
		if err != nil {
			return nil, err
		}
		input.Attributes = &normalized
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

func (s *CustomModelConfigService) ResolveModelAttributes(
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
	if !ok || len(template.Attributes) == 0 {
		return nil, false, nil
	}
	// Runtime snapshots are immutable after compilation; callers only read this map.
	return template.Attributes, true, nil
}

func (s *CustomModelConfigService) ResolveMediaLimits(
	ctx context.Context,
	modelName string,
) (map[string]any, bool, error) {
	config, enabled, err := s.resolveConfig(ctx, modelName)
	if err != nil || !enabled || config == nil || len(config.MediaLimits) == 0 {
		return nil, false, err
	}
	// Runtime snapshots are immutable after compilation; callers only read this map.
	return config.MediaLimits, true, nil
}

// ValidateVideoRequest 按模型级媒体限制校验视频生成请求体。
// 未配置限制时直接通过；duration 为负数（上游自动时长）时不参与区间校验。
func (s *CustomModelConfigService) ValidateVideoRequest(
	ctx context.Context,
	modelName string,
	body []byte,
) error {
	limits, configured, err := s.ResolveMediaLimits(ctx, modelName)
	if err != nil || !configured {
		return err
	}
	allowUpload, _ := limits["allow_upload"].(bool)
	counts := mediaStudioVideoFileCounts(body)
	for _, kind := range []string{"image", "video", "audio"} {
		count := counts[kind]
		if count == 0 {
			continue
		}
		if !allowUpload {
			return infraerrors.BadRequest(
				"CUSTOM_MODEL_MEDIA_LIMIT_EXCEEDED",
				fmt.Sprintf("model %s does not allow uploading %s files", modelName, kind),
			)
		}
		if allowed, declared := mediaStudioLimitFileType(limits, kind); declared && !allowed {
			return infraerrors.BadRequest(
				"CUSTOM_MODEL_MEDIA_LIMIT_EXCEEDED",
				fmt.Sprintf("model %s does not accept %s files", modelName, kind),
			)
		}
		if maxFiles, ok := mediaStudioLimitCount(limits, "max_files", kind); ok {
			if maxFiles <= 0 {
				return infraerrors.BadRequest(
					"CUSTOM_MODEL_MEDIA_LIMIT_EXCEEDED",
					fmt.Sprintf("model %s does not accept %s files", modelName, kind),
				)
			}
			if count > maxFiles {
				return infraerrors.BadRequest(
					"CUSTOM_MODEL_MEDIA_LIMIT_EXCEEDED",
					fmt.Sprintf("model %s allows at most %d %s file(s), got %d", modelName, maxFiles, kind, count),
				)
			}
		}
	}
	minDuration, maxDuration, hasDuration := customModelMediaDurationRange(limits)
	if !hasDuration {
		return nil
	}
	seconds := gjson.GetBytes(body, "duration").Int()
	if seconds > 0 && (seconds < int64(minDuration) || seconds > int64(maxDuration)) {
		return infraerrors.BadRequest(
			"CUSTOM_MODEL_MEDIA_LIMIT_EXCEEDED",
			fmt.Sprintf(
				"model %s allows duration between %d and %d seconds, got %d",
				modelName, minDuration, maxDuration, seconds,
			),
		)
	}
	return nil
}

// mediaStudioVideoFileCounts 统计请求体里各类参考文件的数量。
// 同时兼容数组写法（images/videos/audios）与单值写法（image/video/audio/first_frame）。
func mediaStudioVideoFileCounts(body []byte) map[string]int {
	counts := map[string]int{}
	add := func(path, kind string) {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			return
		}
		if value.IsArray() {
			value.ForEach(func(_, item gjson.Result) bool {
				if strings.TrimSpace(item.String()) != "" {
					counts[kind]++
				}
				return true
			})
			return
		}
		if strings.TrimSpace(value.String()) != "" {
			counts[kind]++
		}
	}
	add("images", "image")
	add("image", "image")
	add("first_frame", "image")
	add("videos", "video")
	add("video", "video")
	add("audios", "audio")
	add("audio", "audio")
	return counts
}

func customModelMediaDurationRange(limits map[string]any) (int, int, bool) {
	raw, ok := limits["duration"].(map[string]any)
	if !ok {
		if typed, ok := limits["duration"].(map[string]int); ok {
			return typed["min"], typed["max"], true
		}
		return 0, 0, false
	}
	minValue, minOK := customModelJSONInt(raw["min"])
	maxValue, maxOK := customModelJSONInt(raw["max"])
	if !minOK || !maxOK {
		return 0, 0, false
	}
	return minValue, maxValue, true
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
		template.Attributes = cloneCustomModelJSONMap(template.Attributes)
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
	} else if videoAPIType != "" && videoAPIType != "grok" && videoAPIType != "agnes" && videoAPIType != "multimodal" {
		return "", nil, "", ErrCustomModelConfigInvalid
	}
	return modelName, normalized, videoAPIType, nil
}

const (
	customModelMediaMaxFiles        = 32
	customModelMediaMaxFileSizeMB   = 256
	customModelMediaMaxDurationSecs = 600
)

// normalizeCustomModelMediaLimits 归一化模型级媒体限制。
// 空对象表示不做限制；非空时总是带上 allow_upload，其余键按需保留。
func normalizeCustomModelMediaLimits(raw map[string]any) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	for key := range raw {
		switch key {
		case "allow_upload", "file_types", "max_files", "max_file_size_mb", "duration":
		default:
			return nil, ErrCustomModelConfigInvalid
		}
	}

	normalized := map[string]any{}
	allowUpload := false
	if value, ok := raw["allow_upload"]; ok && value != nil {
		flag, ok := value.(bool)
		if !ok {
			return nil, ErrCustomModelConfigInvalid
		}
		allowUpload = flag
	}
	normalized["allow_upload"] = allowUpload

	fileTypes, err := normalizeCustomModelMediaFileTypes(raw["file_types"])
	if err != nil {
		return nil, err
	}
	if len(fileTypes) > 0 {
		normalized["file_types"] = fileTypes
	}

	maxFiles, err := normalizeCustomModelMediaCountMap(raw["max_files"], 0, customModelMediaMaxFiles)
	if err != nil {
		return nil, err
	}
	if len(maxFiles) > 0 {
		normalized["max_files"] = maxFiles
	}

	maxFileSize, err := normalizeCustomModelMediaCountMap(raw["max_file_size_mb"], 1, customModelMediaMaxFileSizeMB)
	if err != nil {
		return nil, err
	}
	if len(maxFileSize) > 0 {
		normalized["max_file_size_mb"] = maxFileSize
	}

	duration, err := normalizeCustomModelMediaDuration(raw["duration"])
	if err != nil {
		return nil, err
	}
	if len(duration) > 0 {
		normalized["duration"] = duration
	}

	return normalized, nil
}

func normalizeCustomModelMediaFileTypes(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	var items []string
	switch typed := value.(type) {
	case []string:
		items = typed
	case []any:
		items = make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, ErrCustomModelConfigInvalid
			}
			items = append(items, text)
		}
	default:
		return nil, ErrCustomModelConfigInvalid
	}

	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		switch item {
		case "image", "video", "audio":
		default:
			return nil, ErrCustomModelConfigInvalid
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result, nil
}

func normalizeCustomModelMediaCountMap(value any, minValue, maxValue int) (map[string]int, error) {
	if value == nil {
		return nil, nil
	}
	raw := map[string]any{}
	switch typed := value.(type) {
	case map[string]any:
		raw = typed
	case map[string]int:
		for key, item := range typed {
			raw[key] = item
		}
	default:
		return nil, ErrCustomModelConfigInvalid
	}

	result := make(map[string]int, len(raw))
	for key, item := range raw {
		key = strings.ToLower(strings.TrimSpace(key))
		switch key {
		case "image", "video", "audio":
		default:
			return nil, ErrCustomModelConfigInvalid
		}
		number, ok := customModelJSONInt(item)
		if !ok || number < minValue || number > maxValue {
			return nil, ErrCustomModelConfigInvalid
		}
		result[key] = number
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func normalizeCustomModelMediaDuration(value any) (map[string]int, error) {
	if value == nil {
		return nil, nil
	}
	raw := map[string]any{}
	switch typed := value.(type) {
	case map[string]any:
		raw = typed
	case map[string]int:
		for key, item := range typed {
			raw[key] = item
		}
	default:
		return nil, ErrCustomModelConfigInvalid
	}

	minValue, hasMin := customModelJSONInt(raw["min"])
	maxValue, hasMax := customModelJSONInt(raw["max"])
	if !hasMin || !hasMax {
		return nil, ErrCustomModelConfigInvalid
	}
	if minValue < 1 || maxValue < minValue || maxValue > customModelMediaMaxDurationSecs {
		return nil, ErrCustomModelConfigInvalid
	}
	return map[string]int{"min": minValue, "max": maxValue}, nil
}

func customModelJSONInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float32:
		converted := int(typed)
		if float32(converted) != typed {
			return 0, false
		}
		return converted, true
	case float64:
		converted := int(typed)
		if float64(converted) != typed {
			return 0, false
		}
		return converted, true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
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
	config.MediaLimits = cloneCustomModelJSONMap(config.MediaLimits)
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
