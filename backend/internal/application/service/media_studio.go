package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const SettingKeyMediaStudioGroupRoutes = "media_studio_group_routes"
const MediaStudioAPIKeyName = "Media Studio"

// MaxMediaStudioUploadBytes 是单个参考素材上传的硬上限，避免把超大文件读入内存。
// 与默认的 server.max_request_body_size（256MB）保持一致。
const MaxMediaStudioUploadBytes = 256 << 20

var (
	ErrMediaStudioGroupRouteInvalid = infraerrors.BadRequest(
		"MEDIA_STUDIO_GROUP_ROUTE_INVALID",
		"invalid media studio group route",
	)
	ErrMediaStudioGroupUnavailable = infraerrors.Forbidden(
		"MEDIA_STUDIO_GROUP_UNAVAILABLE",
		"media studio group is not available",
	)
	ErrMediaStudioUploadRejected = infraerrors.BadRequest(
		"MEDIA_STUDIO_UPLOAD_REJECTED",
		"media studio upload rejected by model limits",
	)
	ErrMediaStudioStorageUnavailable = infraerrors.ServiceUnavailable(
		"MEDIA_STUDIO_STORAGE_UNAVAILABLE",
		"media studio object storage is not configured",
	)
)

type MediaStudioGroupRoute struct {
	GroupID   int64    `json:"group_id"`
	MediaType string   `json:"media_type"`
	Priority  int      `json:"priority"`
	Enabled   bool     `json:"enabled"`
	Models    []string `json:"models,omitempty"`
}

type MediaStudioGroupOption struct {
	GroupID   int64    `json:"group_id"`
	GroupName string   `json:"group_name"`
	Platform  string   `json:"platform"`
	MediaType string   `json:"media_type"`
	Models    []string `json:"models,omitempty"`
}

type MediaStudioConfig struct {
	Groups []MediaStudioGroupOption `json:"groups"`
	// StorageAvailable 表示对象存储是否可用；前端据此决定参考素材上传入口是否可点。
	StorageAvailable bool `json:"storage_available"`
}

type MediaStudioGroupRoutes []MediaStudioGroupRoute

type MediaStudioService struct {
	settingRepo             SettingRepository
	groupRepo               GroupRepository
	apiKeyService           *APIKeyService
	customModelCapabilities CustomModelCapabilityResolver
	imageStorage            *ImageStorageSettingService
	keyMu                   sync.Mutex
}

func NewMediaStudioService(
	settingRepo SettingRepository,
	groupRepo GroupRepository,
	apiKeyService *APIKeyService,
	customModelCapabilities CustomModelCapabilityResolver,
	imageStorage *ImageStorageSettingService,
) *MediaStudioService {
	return &MediaStudioService{
		settingRepo:             settingRepo,
		groupRepo:               groupRepo,
		apiKeyService:           apiKeyService,
		customModelCapabilities: customModelCapabilities,
		imageStorage:            imageStorage,
	}
}

func ProvideMediaStudioService(
	settingRepo SettingRepository,
	groupRepo GroupRepository,
	apiKeyService *APIKeyService,
	customModelCapabilities CustomModelCapabilityResolver,
	imageStorage *ImageStorageSettingService,
) *MediaStudioService {
	return NewMediaStudioService(
		settingRepo,
		groupRepo,
		apiKeyService,
		customModelCapabilities,
		imageStorage,
	)
}

func (s *MediaStudioService) GetGroupRoutes(ctx context.Context) (MediaStudioGroupRoutes, error) {
	if s == nil || s.settingRepo == nil {
		return emptyMediaStudioGroupRoutes(), nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyMediaStudioGroupRoutes)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return emptyMediaStudioGroupRoutes(), nil
		}
		return nil, fmt.Errorf("get media studio group routes: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return emptyMediaStudioGroupRoutes(), nil
	}

	var routes MediaStudioGroupRoutes
	if err := json.Unmarshal([]byte(raw), &routes); err != nil {
		var legacy map[string][]MediaStudioGroupRoute
		if legacyErr := json.Unmarshal([]byte(raw), &legacy); legacyErr != nil {
			return nil, fmt.Errorf("decode media studio group routes: %w", err)
		}
		routes = flattenLegacyMediaStudioGroupRoutes(legacy)
	}
	return s.resolveGroupRouteMediaTypes(ctx, normalizeMediaStudioGroupRoutes(routes)), nil
}

// resolveGroupRouteMediaTypes backfills the modality for routes saved before
// media types were part of the configuration. Each group keeps exactly one
// modality, so the first resolvable capability wins.
func (s *MediaStudioService) resolveGroupRouteMediaTypes(
	ctx context.Context,
	routes MediaStudioGroupRoutes,
) MediaStudioGroupRoutes {
	for index := range routes {
		if routes[index].MediaType != "" {
			continue
		}
		routes[index].MediaType = s.inferGroupMediaType(ctx, routes[index].Models)
	}
	return routes
}

func (s *MediaStudioService) inferGroupMediaType(ctx context.Context, models []string) string {
	if s.customModelCapabilities == nil {
		return "image"
	}
	for _, model := range models {
		for _, capability := range []string{"video", "audio", "image"} {
			ok, err := s.customModelCapabilities.HasCapability(ctx, model, capability)
			if err != nil {
				return "image"
			}
			if ok {
				return capability
			}
		}
	}
	return "image"
}

func (s *MediaStudioService) SaveGroupRoutes(ctx context.Context, routes MediaStudioGroupRoutes) (MediaStudioGroupRoutes, error) {
	normalized, err := s.validateGroupRoutes(ctx, routes)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode media studio group routes: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyMediaStudioGroupRoutes, string(payload)); err != nil {
		return nil, fmt.Errorf("save media studio group routes: %w", err)
	}
	return normalized, nil
}

func (s *MediaStudioService) GetAvailableGroups(ctx context.Context, userID int64) (*MediaStudioConfig, error) {
	routes, err := s.GetGroupRoutes(ctx)
	if err != nil {
		return nil, err
	}
	if s.apiKeyService == nil {
		return &MediaStudioConfig{StorageAvailable: s.storageAvailable()}, nil
	}

	available, err := s.apiKeyService.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get available media studio groups: %w", err)
	}
	availableByID := make(map[int64]Group, len(available))
	for _, group := range available {
		availableByID[group.ID] = group
	}

	return &MediaStudioConfig{
		Groups:           s.resolveGroupOptions(routes, availableByID),
		StorageAvailable: s.storageAvailable(),
	}, nil
}

func (s *MediaStudioService) storageAvailable() bool {
	if s.imageStorage == nil {
		return false
	}
	_, ok := s.imageStorage.Storage()
	return ok
}

type MediaStudioModel struct {
	ID         string         `json:"id"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Limits     map[string]any `json:"limits,omitempty"`
}

func (s *MediaStudioService) ListModels(
	ctx context.Context,
	userID int64,
	groupID int64,
	mediaType string,
) ([]MediaStudioModel, error) {
	mediaType = normalizeMediaStudioMediaType(mediaType)
	if mediaType == "" || groupID <= 0 {
		return nil, ErrMediaStudioGroupRouteInvalid
	}
	if s.customModelCapabilities == nil {
		return []MediaStudioModel{}, nil
	}

	config, err := s.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	var selected *MediaStudioGroupOption
	for index := range config.Groups {
		if config.Groups[index].GroupID == groupID {
			selected = &config.Groups[index]
			break
		}
	}
	if selected == nil || selected.MediaType != mediaType {
		return nil, ErrMediaStudioGroupUnavailable
	}

	models := make([]MediaStudioModel, 0, len(selected.Models))
	for _, model := range selected.Models {
		ok, err := s.customModelCapabilities.HasCapability(ctx, model, mediaType)
		if err != nil {
			return nil, fmt.Errorf("resolve media model capability: %w", err)
		}
		if !ok {
			continue
		}
		item := MediaStudioModel{ID: model}
		raw, configured, err := s.customModelCapabilities.ResolveModelAttributes(ctx, model)
		if err != nil {
			return nil, fmt.Errorf("resolve media model attributes: %w", err)
		}
		if configured {
			if block := modelAttributesForModality(raw, mediaType); len(block) > 0 {
				item.Attributes = block
			}
		}
		limits, limitsConfigured, err := s.customModelCapabilities.ResolveMediaLimits(ctx, model)
		if err != nil {
			return nil, fmt.Errorf("resolve media model limits: %w", err)
		}
		if limitsConfigured {
			item.Limits = limits
		}
		models = append(models, item)
	}
	return models, nil
}

// MediaStudioUpload 是一次参考素材上传的结果，URL 可直接作为视频/图片请求的参考地址。
type MediaStudioUpload struct {
	URL         string `json:"url"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

// UploadFile 把用户选择的参考素材写入对象存储并返回可访问 URL。
// 模型配置了媒体限制时按 allow_upload、file_types、max_files、max_file_size_mb 校验。
func (s *MediaStudioService) UploadFile(
	ctx context.Context,
	userID int64,
	modelName string,
	kind string,
	fileName string,
	contentType string,
	data []byte,
) (*MediaStudioUpload, error) {
	kind = normalizeMediaStudioMediaType(kind)
	if kind == "" || len(data) == 0 {
		return nil, ErrMediaStudioUploadRejected
	}
	if len(data) > MaxMediaStudioUploadBytes {
		return nil, ErrMediaStudioUploadRejected
	}
	if err := s.validateUploadLimits(ctx, modelName, kind, int64(len(data))); err != nil {
		return nil, err
	}
	if s.imageStorage == nil {
		return nil, ErrMediaStudioStorageUnavailable
	}
	storage, ok := s.imageStorage.Storage()
	if !ok {
		return nil, ErrMediaStudioStorageUnavailable
	}

	if strings.TrimSpace(contentType) == "" {
		contentType = http.DetectContentType(data)
	}
	key := fmt.Sprintf(
		"media-studio/uploads/%d/%s%s",
		userID,
		uuid.NewString(),
		mediaStudioUploadExtension(fileName, contentType),
	)
	url, err := storage.Save(ctx, key, contentType, data)
	if err != nil {
		return nil, fmt.Errorf("save media studio upload: %w", err)
	}
	return &MediaStudioUpload{
		URL:         url,
		Name:        strings.TrimSpace(fileName),
		Kind:        kind,
		Size:        int64(len(data)),
		ContentType: contentType,
	}, nil
}

func (s *MediaStudioService) validateUploadLimits(
	ctx context.Context,
	modelName string,
	kind string,
	size int64,
) error {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || s.customModelCapabilities == nil {
		return nil
	}
	limits, configured, err := s.customModelCapabilities.ResolveMediaLimits(ctx, modelName)
	if err != nil {
		return fmt.Errorf("resolve media model limits: %w", err)
	}
	if !configured {
		return nil
	}
	if allow, ok := limits["allow_upload"].(bool); ok && !allow {
		return ErrMediaStudioUploadRejected
	}
	if allowed, declared := mediaStudioLimitFileType(limits, kind); declared && !allowed {
		return ErrMediaStudioUploadRejected
	}
	if maxFiles, ok := mediaStudioLimitCount(limits, "max_files", kind); ok && maxFiles <= 0 {
		return ErrMediaStudioUploadRejected
	}
	if maxSizeMB, ok := mediaStudioLimitCount(limits, "max_file_size_mb", kind); ok {
		if maxSizeMB <= 0 || size > int64(maxSizeMB)<<20 {
			return ErrMediaStudioUploadRejected
		}
	}
	return nil
}

func mediaStudioLimitFileType(limits map[string]any, kind string) (bool, bool) {
	switch list := limits["file_types"].(type) {
	case []string:
		for _, item := range list {
			if item == kind {
				return true, true
			}
		}
		return false, true
	case []any:
		for _, item := range list {
			if value, ok := item.(string); ok && value == kind {
				return true, true
			}
		}
		return false, true
	default:
		return false, false
	}
}

func mediaStudioLimitCount(limits map[string]any, key, kind string) (int, bool) {
	switch raw := limits[key].(type) {
	case map[string]any:
		value, ok := raw[kind]
		if !ok {
			return 0, false
		}
		return customModelJSONInt(value)
	case map[string]int:
		count, ok := raw[kind]
		return count, ok
	default:
		return 0, false
	}
}

var mediaStudioContentTypeExtensions = map[string]string{
	"image/jpeg":      ".jpg",
	"image/jpg":       ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"image/gif":       ".gif",
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
	"video/webm":      ".webm",
	"video/x-matroska": ".mkv",
	"audio/mpeg":      ".mp3",
	"audio/mp3":       ".mp3",
	"audio/wav":       ".wav",
	"audio/x-wav":     ".wav",
	"audio/ogg":       ".ogg",
	"audio/aac":       ".aac",
	"audio/mp4":       ".m4a",
}

func mediaStudioUploadExtension(fileName, contentType string) string {
	if ext := strings.ToLower(path.Ext(strings.TrimSpace(fileName))); ext != "" && len(ext) <= 8 {
		return ext
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return mediaStudioContentTypeExtensions[mediaType]
}

func (s *MediaStudioService) EnsureAPIKey(
	ctx context.Context,
	userID int64,
	mediaType string,
	groupID int64,
) (*APIKey, error) {
	mediaType = normalizeMediaStudioMediaType(mediaType)
	if mediaType == "" || groupID <= 0 {
		return nil, ErrMediaStudioGroupRouteInvalid
	}

	config, err := s.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	var selected *MediaStudioGroupOption
	for index := range config.Groups {
		if config.Groups[index].GroupID == groupID {
			selected = &config.Groups[index]
			break
		}
	}
	if selected == nil || selected.MediaType != mediaType {
		return nil, ErrMediaStudioGroupUnavailable
	}

	s.keyMu.Lock()
	defer s.keyMu.Unlock()

	name := MediaStudioAPIKeyName
	keys, err := s.apiKeyService.SearchAPIKeys(ctx, userID, name, 1000)
	if err != nil {
		return nil, fmt.Errorf("find media studio api key: %w", err)
	}
	var existing *APIKey
	for _, key := range keys {
		if !strings.EqualFold(strings.TrimSpace(key.Name), name) {
			continue
		}
		if key.GroupID == nil || *key.GroupID != groupID {
			continue
		}
		if existing == nil || key.ID < existing.ID {
			keyCopy := key
			existing = &keyCopy
		}
	}
	if existing != nil {
		return existing, nil
	}

	groupIDValue := selected.GroupID
	created, err := s.apiKeyService.CreateMediaStudioAPIKey(ctx, userID, CreateAPIKeyRequest{
		Name:    name,
		GroupID: &groupIDValue,
	})
	if err != nil {
		if retryKeys, retryErr := s.apiKeyService.SearchAPIKeys(ctx, userID, name, 1000); retryErr == nil {
			for index := range retryKeys {
				key := &retryKeys[index]
				if strings.EqualFold(strings.TrimSpace(key.Name), name) &&
					key.GroupID != nil &&
					*key.GroupID == groupID {
					return &retryKeys[index], nil
				}
			}
		}
		return nil, fmt.Errorf("create media studio api key: %w", err)
	}
	return created, nil
}

func isMediaStudioAPIKeyName(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), MediaStudioAPIKeyName)
}

func IsMediaStudioAPIKey(apiKey *APIKey, groupID int64) bool {
	if apiKey == nil || groupID <= 0 || !isMediaStudioAPIKeyName(apiKey.Name) {
		return false
	}
	if apiKey.GroupID != nil {
		return *apiKey.GroupID == groupID
	}
	return apiKey.Group != nil && apiKey.Group.ID == groupID
}

func (s *MediaStudioService) validateGroupRoutes(ctx context.Context, routes MediaStudioGroupRoutes) (MediaStudioGroupRoutes, error) {
	if err := ensureSingleMediaStudioGroupMediaType(routes); err != nil {
		return nil, err
	}
	normalized := normalizeMediaStudioGroupRoutes(routes)
	for index := range normalized {
		entry := &normalized[index]
		if entry.GroupID <= 0 || entry.Priority < 0 {
			return nil, ErrMediaStudioGroupRouteInvalid
		}
		mediaType := normalizeMediaStudioMediaType(entry.MediaType)
		if mediaType == "" {
			return nil, fmt.Errorf("%w: group %d has an invalid media type", ErrMediaStudioGroupRouteInvalid, entry.GroupID)
		}
		entry.MediaType = mediaType
		if len(entry.Models) > 200 {
			return nil, fmt.Errorf("%w: group %d has too many models", ErrMediaStudioGroupRouteInvalid, entry.GroupID)
		}
		for _, model := range entry.Models {
			if len(model) > 255 {
				return nil, fmt.Errorf("%w: group %d has an invalid model name", ErrMediaStudioGroupRouteInvalid, entry.GroupID)
			}
		}
		group, err := s.groupRepo.GetByID(ctx, entry.GroupID)
		if err != nil {
			return nil, fmt.Errorf("get media studio group %d: %w", entry.GroupID, err)
		}
		if group == nil || !group.IsActive() {
			return nil, ErrMediaStudioGroupUnavailable
		}
	}
	return normalized, nil
}

func (s *MediaStudioService) resolveGroupOptions(
	entries []MediaStudioGroupRoute,
	available map[int64]Group,
) []MediaStudioGroupOption {
	options := make([]MediaStudioGroupOption, 0, len(entries))
	for _, entry := range entries {
		if !entry.Enabled {
			continue
		}
		group, ok := available[entry.GroupID]
		if !ok || !group.IsActive() {
			continue
		}
		options = append(options, MediaStudioGroupOption{
			GroupID:   group.ID,
			GroupName: group.Name,
			Platform:  group.Platform,
			MediaType: entry.MediaType,
			Models:    append([]string(nil), entry.Models...),
		})
	}
	return options
}

// ensureSingleMediaStudioGroupMediaType rejects payloads that bind one group to
// more than one modality.
func ensureSingleMediaStudioGroupMediaType(routes MediaStudioGroupRoutes) error {
	seen := make(map[int64]string, len(routes))
	for _, entry := range routes {
		if entry.GroupID <= 0 {
			continue
		}
		mediaType := normalizeMediaStudioMediaType(entry.MediaType)
		if mediaType == "" {
			continue
		}
		if existing, ok := seen[entry.GroupID]; ok && existing != mediaType {
			return fmt.Errorf("%w: group %d mixes media types", ErrMediaStudioGroupRouteInvalid, entry.GroupID)
		}
		seen[entry.GroupID] = mediaType
	}
	return nil
}

func normalizeMediaStudioGroupRoutes(routes MediaStudioGroupRoutes) MediaStudioGroupRoutes {
	byGroup := make(map[int64]MediaStudioGroupRoute, len(routes))
	for _, entry := range routes {
		if entry.GroupID <= 0 {
			continue
		}
		current, exists := byGroup[entry.GroupID]
		if !exists || entry.Priority < current.Priority {
			current.Priority = entry.Priority
		}
		current.GroupID = entry.GroupID
		current.Enabled = current.Enabled || entry.Enabled
		if current.MediaType == "" {
			current.MediaType = normalizeMediaStudioMediaType(entry.MediaType)
		}
		current.Models = append(current.Models, entry.Models...)
		byGroup[entry.GroupID] = current
	}

	normalized := make(MediaStudioGroupRoutes, 0, len(byGroup))
	for _, entry := range byGroup {
		entry.Models = normalizeMediaStudioModels(entry.Models)
		normalized = append(normalized, entry)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Priority != normalized[j].Priority {
			return normalized[i].Priority < normalized[j].Priority
		}
		return normalized[i].GroupID < normalized[j].GroupID
	})
	for index := range normalized {
		normalized[index].Priority = index
	}
	return normalized
}

func flattenLegacyMediaStudioGroupRoutes(
	legacy map[string][]MediaStudioGroupRoute,
) MediaStudioGroupRoutes {
	flattened := make(MediaStudioGroupRoutes, 0)
	for _, mediaType := range []string{"image", "video", "audio"} {
		for _, entry := range legacy[mediaType] {
			if entry.MediaType == "" {
				entry.MediaType = mediaType
			}
			flattened = append(flattened, entry)
		}
	}
	return flattened
}

func normalizeMediaStudioModels(models []string) []string {
	normalized := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, model)
	}
	return normalized
}

func emptyMediaStudioGroupRoutes() MediaStudioGroupRoutes {
	return MediaStudioGroupRoutes{}
}

func normalizeMediaStudioMediaType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image", "images":
		return "image"
	case "video", "videos":
		return "video"
	case "audio", "audios":
		return "audio"
	default:
		return ""
	}
}
