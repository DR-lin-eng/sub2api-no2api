package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const SettingKeyMediaStudioGroupRoutes = "media_studio_group_routes"
const MediaStudioAPIKeyName = "Media Studio"

var (
	ErrMediaStudioGroupRouteInvalid = infraerrors.BadRequest(
		"MEDIA_STUDIO_GROUP_ROUTE_INVALID",
		"invalid media studio group route",
	)
	ErrMediaStudioGroupUnavailable = infraerrors.Forbidden(
		"MEDIA_STUDIO_GROUP_UNAVAILABLE",
		"media studio group is not available",
	)
)

type MediaStudioGroupRoute struct {
	GroupID  int64    `json:"group_id"`
	Priority int      `json:"priority"`
	Enabled  bool     `json:"enabled"`
	Models   []string `json:"models,omitempty"`
}

type MediaStudioGroupOption struct {
	GroupID   int64    `json:"group_id"`
	GroupName string   `json:"group_name"`
	Platform  string   `json:"platform"`
	Models    []string `json:"models,omitempty"`
}

type MediaStudioConfig struct {
	Groups []MediaStudioGroupOption `json:"groups"`
}

type MediaStudioGroupRoutes []MediaStudioGroupRoute

type MediaStudioService struct {
	settingRepo             SettingRepository
	groupRepo               GroupRepository
	apiKeyService           *APIKeyService
	customModelCapabilities CustomModelCapabilityResolver
	keyMu                   sync.Mutex
}

func NewMediaStudioService(
	settingRepo SettingRepository,
	groupRepo GroupRepository,
	apiKeyService *APIKeyService,
	customModelCapabilities CustomModelCapabilityResolver,
) *MediaStudioService {
	return &MediaStudioService{
		settingRepo:             settingRepo,
		groupRepo:               groupRepo,
		apiKeyService:           apiKeyService,
		customModelCapabilities: customModelCapabilities,
	}
}

func ProvideMediaStudioService(
	settingRepo SettingRepository,
	groupRepo GroupRepository,
	apiKeyService *APIKeyService,
	customModelCapabilities CustomModelCapabilityResolver,
) *MediaStudioService {
	return NewMediaStudioService(
		settingRepo,
		groupRepo,
		apiKeyService,
		customModelCapabilities,
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
	return normalizeMediaStudioGroupRoutes(routes), nil
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
		return &MediaStudioConfig{}, nil
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
		Groups: s.resolveGroupOptions(routes, availableByID),
	}, nil
}

func (s *MediaStudioService) ListModels(
	ctx context.Context,
	userID int64,
	groupID int64,
	mediaType string,
) ([]string, error) {
	mediaType = normalizeMediaStudioMediaType(mediaType)
	if mediaType == "" || groupID <= 0 {
		return nil, ErrMediaStudioGroupRouteInvalid
	}
	if s.customModelCapabilities == nil {
		return []string{}, nil
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
	if selected == nil {
		return nil, ErrMediaStudioGroupUnavailable
	}

	models := make([]string, 0, len(selected.Models))
	for _, model := range selected.Models {
		ok, err := s.customModelCapabilities.HasCapability(ctx, model, mediaType)
		if err != nil {
			return nil, fmt.Errorf("resolve media model capability: %w", err)
		}
		if ok {
			models = append(models, model)
		}
	}
	return models, nil
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
	if selected == nil {
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
	normalized := normalizeMediaStudioGroupRoutes(routes)
	for _, entry := range normalized {
		if entry.GroupID <= 0 || entry.Priority < 0 {
			return nil, ErrMediaStudioGroupRouteInvalid
		}
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
			Models:    append([]string(nil), entry.Models...),
		})
	}
	return options
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
		flattened = append(flattened, legacy[mediaType]...)
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
