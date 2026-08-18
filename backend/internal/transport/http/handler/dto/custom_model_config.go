package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
)

type CustomModelConfig struct {
	ID           int64     `json:"id"`
	ModelName    string    `json:"model_name"`
	Capabilities []string  `json:"capabilities"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateCustomModelConfigRequest struct {
	ModelName    string   `json:"model_name" binding:"required"`
	Capabilities []string `json:"capabilities"`
}

type UpdateCustomModelConfigRequest struct {
	Capabilities []string `json:"capabilities"`
}

func CustomModelConfigFromEnt(e *ent.CustomModelConfig) *CustomModelConfig {
	if e == nil {
		return nil
	}
	return &CustomModelConfig{
		ID:           int64(e.ID),
		ModelName:    e.ModelName,
		Capabilities: e.Capabilities,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}
