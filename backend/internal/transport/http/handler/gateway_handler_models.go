package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/shared/claude"
	"github.com/Wei-Shaw/sub2api/internal/shared/geminicli"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/Wei-Shaw/sub2api/internal/shared/xai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
)

// Models handles listing available models
// GET /v1/models
// Returns models based on account configurations (model_mapping whitelist)
// Falls back to default models if no whitelist is configured
func (h *GatewayHandler) Models(c *gin.Context) {
	apiKey, _ := middleware2.GetAPIKeyFromContext(c)

	var groupID *int64
	var platform string

	if apiKey != nil && apiKey.Group != nil {
		groupID = &apiKey.Group.ID
		platform = apiKey.Group.Platform
	}
	if forcedPlatform, ok := middleware2.GetForcePlatformFromContext(c); ok && strings.TrimSpace(forcedPlatform) != "" {
		platform = forcedPlatform
	}

	if platform == service.PlatformComposite {
		availableModels := h.compositeAvailableModels(c.Request.Context(), groupID)
		if apiKey != nil && apiKey.Group != nil && apiKey.Group.CustomModelsListEnabled() {
			availableModels = filterModelsByCustomList(availableModels, defaultModelIDsForPlatform(service.PlatformComposite), apiKey.Group.ModelsListConfig.Models)
			writeCustomModelsList(c, service.PlatformComposite, availableModels)
			return
		}
		if len(availableModels) > 0 {
			writeModelsList(c, service.PlatformComposite, availableModels)
			return
		}
		writeModelsList(c, service.PlatformComposite, defaultModelIDsForPlatform(service.PlatformComposite))
		return
	}

	// Get available models from account configurations for the selected group platform.
	availableModels := h.gatewayService.GetAvailableModels(c.Request.Context(), groupID, platform)
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.CustomModelsListEnabled() {
		fallbackModels := defaultModelIDsForPlatform(platform)
		availableModels = filterModelsByCustomList(customModelsListSource(platform, availableModels, fallbackModels), fallbackModels, apiKey.Group.ModelsListConfig.Models)
		writeCustomModelsList(c, platform, availableModels)
		return
	}

	if len(availableModels) > 0 {
		writeModelsList(c, platform, availableModels)
		return
	}

	// Fallback to default models
	if platform == service.PlatformOpenAI {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   openai.DefaultModels,
		})
		return
	}

	if platform == service.PlatformGemini {
		c.JSON(http.StatusOK, gin.H{
			"object": "list",
			"data":   geminicli.DefaultModels,
		})
		return
	}
	if platform == service.PlatformGrok {
		writeGrokModelsList(c, xai.DefaultModelIDs())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   claude.DefaultModels,
	})
}

func (h *GatewayHandler) compositeAvailableModels(ctx context.Context, groupID *int64) []string {
	if h == nil || h.gatewayService == nil {
		return nil
	}
	seen := make(map[string]struct{})
	models := make([]string, 0)
	schedulablePlatforms := h.gatewayService.GetSchedulablePlatforms(ctx, groupID)
	for _, platform := range []string{service.PlatformAnthropic, service.PlatformGemini, service.PlatformOpenAI, service.PlatformAntigravity, service.PlatformGrok} {
		platformModels := h.gatewayService.GetAvailableModels(ctx, groupID, platform)
		if len(platformModels) == 0 {
			if _, ok := schedulablePlatforms[platform]; ok {
				platformModels = defaultModelIDsForPlatform(platform)
			}
		}
		for _, model := range platformModels {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			models = append(models, model)
		}
	}
	return models
}

func writeModelsList(c *gin.Context, platform string, modelIDs []string) {
	if platform == service.PlatformGrok {
		writeGrokModelsList(c, modelIDs)
		return
	}
	models := make([]claude.Model, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		models = append(models, claude.Model{
			ID:          modelID,
			Type:        "model",
			DisplayName: modelID,
			CreatedAt:   "2024-01-01T00:00:00Z",
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

func writeCustomModelsList(c *gin.Context, platform string, modelIDs []string) {
	if platform == service.PlatformOpenAI {
		writeOpenAIModelsList(c, modelIDs)
		return
	}
	writeModelsList(c, platform, modelIDs)
}

type grokReasoningEffortOption struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	Default bool   `json:"default,omitempty"`
}

type grokModelListItem struct {
	xai.Model
	SupportsReasoningEffort bool                        `json:"supportsReasoningEffort,omitempty"`
	ReasoningEffort         string                      `json:"reasoningEffort,omitempty"`
	ReasoningEfforts        []grokReasoningEffortOption `json:"reasoningEfforts,omitempty"`
}

func writeGrokModelsList(c *gin.Context, modelIDs []string) {
	defaults := xai.DefaultModels()
	defaultsByID := make(map[string]xai.Model, len(defaults))
	for _, model := range defaults {
		defaultsByID[model.ID] = model
	}

	models := make([]grokModelListItem, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		model, ok := defaultsByID[modelID]
		if !ok {
			model = xai.Model{
				ID:          modelID,
				Object:      "model",
				OwnedBy:     "xai",
				DisplayName: modelID,
			}
		}
		item := grokModelListItem{Model: model}
		if grokModelSupportsConfigurableReasoning(modelID) {
			item.SupportsReasoningEffort = true
			item.ReasoningEffort = "high"
			item.ReasoningEfforts = []grokReasoningEffortOption{
				{Value: "low", Label: "Low"},
				{Value: "medium", Label: "Medium"},
				{Value: "high", Label: "High", Default: true},
			}
		}
		models = append(models, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

func grokModelSupportsConfigurableReasoning(modelID string) bool {
	switch strings.ToLower(strings.TrimSpace(modelID)) {
	case "grok-4.5", "grok-4.5-latest", "grok", "grok-latest", "grok-build", "grok-build-latest", "grok-build-0.1":
		return true
	default:
		return false
	}
}

func writeOpenAIModelsList(c *gin.Context, modelIDs []string) {
	defaultsByID := make(map[string]openai.Model, len(openai.DefaultModels))
	for _, model := range openai.DefaultModels {
		defaultsByID[model.ID] = model
	}

	models := make([]openai.Model, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		if model, ok := defaultsByID[modelID]; ok {
			models = append(models, model)
			continue
		}
		models = append(models, openai.Model{
			ID:          modelID,
			Object:      "model",
			Created:     1704067200,
			OwnedBy:     "openai",
			Type:        "model",
			DisplayName: modelID,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

func customModelsListSource(platform string, availableModels, fallbackModels []string) []string {
	if platform == service.PlatformAnthropic && len(availableModels) > 0 {
		return mergeModelIDs(availableModels, fallbackModels)
	}
	return availableModels
}

func filterModelsByCustomList(availableModels, fallbackModels, selectedModels []string) []string {
	if len(selectedModels) == 0 {
		return availableModels
	}
	source := availableModels
	if len(source) == 0 {
		source = fallbackModels
	}
	if len(source) == 0 {
		return nil
	}

	allowed := make([]string, 0, len(source))
	for _, model := range source {
		model = strings.TrimSpace(model)
		if model != "" {
			allowed = append(allowed, model)
		}
	}

	seen := make(map[string]struct{}, len(selectedModels))
	filtered := make([]string, 0, len(selectedModels))
	for _, model := range selectedModels {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if !customModelsListAllowsModel(allowed, model) {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		filtered = append(filtered, model)
	}
	return filtered
}

func customModelsListAllowsModel(availablePatterns []string, model string) bool {
	for _, pattern := range availablePatterns {
		if pattern == model {
			return true
		}
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(model, strings.TrimSuffix(pattern, "*")) {
			return true
		}
	}
	return false
}

func defaultModelIDsForPlatform(platform string) []string {
	switch platform {
	case service.PlatformOpenAI:
		return openai.DefaultModelIDs()
	case service.PlatformGemini:
		ids := make([]string, 0, len(geminicli.DefaultModels))
		for _, model := range geminicli.DefaultModels {
			ids = append(ids, model.ID)
		}
		return ids
	case service.PlatformAntigravity:
		models := antigravity.DefaultModels()
		ids := make([]string, 0, len(models))
		for _, model := range models {
			ids = append(ids, model.ID)
		}
		return ids
	case service.PlatformAnthropic:
		ids := make([]string, 0, len(claude.DefaultModels)+len(antigravity.DefaultModels()))
		for _, model := range claude.DefaultModels {
			ids = append(ids, model.ID)
		}
		for _, model := range antigravity.DefaultModels() {
			ids = append(ids, model.ID)
		}
		return mergeModelIDs(ids, nil)
	case service.PlatformGrok:
		return xai.DefaultModelIDs()
	case service.PlatformComposite:
		ids := make([]string, 0)
		seen := make(map[string]struct{})
		for _, concretePlatform := range []string{service.PlatformAnthropic, service.PlatformGemini, service.PlatformOpenAI, service.PlatformAntigravity, service.PlatformGrok} {
			for _, id := range defaultModelIDsForPlatform(concretePlatform) {
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
		return ids
	default:
		ids := make([]string, 0, len(claude.DefaultModels))
		for _, model := range claude.DefaultModels {
			ids = append(ids, model.ID)
		}
		return ids
	}
}

func mergeModelIDs(primary, secondary []string) []string {
	seen := make(map[string]struct{}, len(primary)+len(secondary))
	merged := make([]string, 0, len(primary)+len(secondary))
	for _, models := range [][]string{primary, secondary} {
		for _, model := range models {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			merged = append(merged, model)
		}
	}
	return merged
}

// AntigravityModels 返回 Antigravity 支持的全部模型
// GET /antigravity/models
func (h *GatewayHandler) AntigravityModels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   antigravity.DefaultModels(),
	})
}

func cloneAPIKeyWithGroup(apiKey *service.APIKey, group *service.Group) *service.APIKey {
	if apiKey == nil || group == nil {
		return apiKey
	}
	cloned := *apiKey
	groupID := group.ID
	cloned.GroupID = &groupID
	cloned.Group = group
	return &cloned
}
