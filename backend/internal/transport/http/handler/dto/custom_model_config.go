package dto

import (
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
)

type CustomModelConfig struct {
	ID           int64     `json:"id"`
	ModelName    string    `json:"model_name"`
	PrefixMatch  bool      `json:"prefix_match"`
	Capabilities []string  `json:"capabilities"`
	TemplateID   *int64    `json:"template_id,omitempty"`
	TemplateName string    `json:"template_name,omitempty"`
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
}

type UpdateCustomModelConfigRequest struct {
	PrefixMatch  *bool           `json:"prefix_match"`
	Capabilities []string        `json:"capabilities"`
	TemplateID   json.RawMessage `json:"template_id"`
}

func CustomModelConfigFromEnt(e *ent.CustomModelConfig, templateName string) *CustomModelConfig {
	if e == nil {
		return nil
	}
	return &CustomModelConfig{
		ID:           int64(e.ID),
		ModelName:    e.ModelName,
		PrefixMatch:  e.PrefixMatch,
		Capabilities: e.Capabilities,
		TemplateID:   e.TemplateID,
		TemplateName: templateName,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}

func CustomModelRequestTemplateFromEnt(e *ent.CustomModelRequestTemplate) *CustomModelRequestTemplate {
	if e == nil {
		return nil
	}
	adapter := e.RequestAdapter
	if adapter == nil {
		adapter = map[string]any{}
	}
	return &CustomModelRequestTemplate{
		ID:             int64(e.ID),
		Name:           e.Name,
		Description:    e.Description,
		RequestAdapter: adapter,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}
