package dto

type CustomModelConfigDTO struct {
	ID            int    `json:"id"`
	ModelName     string `json:"model_name"`
	SupportsImage bool   `json:"supports_image"`
	SupportsVideo bool   `json:"supports_video"`
	SupportsAudio bool   `json:"supports_audio"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

type CreateCustomModelConfigRequest struct {
	ModelName     string `json:"model_name" binding:"required"`
	SupportsImage bool   `json:"supports_image"`
	SupportsVideo bool   `json:"supports_video"`
	SupportsAudio bool   `json:"supports_audio"`
}

type UpdateCustomModelConfigRequest struct {
	ModelName     string `json:"model_name" binding:"required"`
	SupportsImage bool   `json:"supports_image"`
	SupportsVideo bool   `json:"supports_video"`
	SupportsAudio bool   `json:"supports_audio"`
}

type CustomModelConfigResponse struct {
	Success bool                  `json:"success"`
	Data    *CustomModelConfigDTO `json:"data"`
}

type ListCustomModelConfigsResponse struct {
	Success bool                    `json:"success"`
	Data    []*CustomModelConfigDTO `json:"data"`
}
