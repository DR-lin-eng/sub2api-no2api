package dto

import (
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

type CustomModelConfig struct {
	ID           int64     `json:"id"`
	ModelName    string    `json:"model_name"`
	PrefixMatch  bool      `json:"prefix_match"`
	Capabilities []string  `json:"capabilities"`
	TemplateID   *int64    `json:"template_id,omitempty"`
	TemplateName string    `json:"template_name,omitempty"`
	VideoAPIType string    `json:"video_api_type,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CustomModelRequestTemplate struct {
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	RequestAdapter map[string]any `json:"request_adapter"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type CreateCustomModelConfigRequest struct {
	ModelName    string   `json:"model_name" binding:"required"`
	PrefixMatch  bool     `json:"prefix_match"`
	Capabilities []string `json:"capabilities"`
	TemplateID   *int64   `json:"template_id"`
	VideoAPIType string   `json:"video_api_type"`
}

type UpdateCustomModelConfigRequest struct {
	PrefixMatch  *bool           `json:"prefix_match"`
	Capabilities *[]string       `json:"capabilities"`
	TemplateID   json.RawMessage `json:"template_id"`
	VideoAPIType json.RawMessage `json:"video_api_type"`
}

func CustomModelConfigFromService(item *service.CustomModelConfig) *CustomModelConfig {
	if item == nil {
		return nil
	}
	return &CustomModelConfig{
		ID:           item.ID,
		ModelName:    item.ModelName,
		PrefixMatch:  item.PrefixMatch,
		Capabilities: append([]string(nil), item.Capabilities...),
		TemplateID:   item.TemplateID,
		TemplateName: item.TemplateName,
		VideoAPIType: item.VideoAPIType,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

func CustomModelRequestTemplateFromService(item *service.CustomModelRequestTemplate) *CustomModelRequestTemplate {
	if item == nil {
		return nil
	}
	adapter := item.RequestAdapter
	if adapter == nil {
		adapter = map[string]any{}
	}
	return &CustomModelRequestTemplate{
		ID:             item.ID,
		Name:           item.Name,
		Description:    item.Description,
		RequestAdapter: adapter,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}
