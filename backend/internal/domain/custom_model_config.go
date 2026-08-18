package domain

import (
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

var (
	ErrCustomModelConfigNotFound     = infraerrors.NotFound("CUSTOM_MODEL_CONFIG_NOT_FOUND", "custom model config not found")
	ErrCustomModelConfigDuplicate    = infraerrors.Conflict("CUSTOM_MODEL_CONFIG_DUPLICATE", "custom model config already exists")
	ErrCustomModelConfigInvalidName  = infraerrors.BadRequest("CUSTOM_MODEL_CONFIG_INVALID_NAME", "invalid model name")
	ErrCustomModelConfigNoCapability = infraerrors.BadRequest("CUSTOM_MODEL_CONFIG_NO_CAPABILITY", "at least one capability must be enabled")
)

// CustomModelConfig 自定义模型配置
type CustomModelConfig struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ModelName    string    `gorm:"uniqueIndex;not null;size:255" json:"model_name"`
	IsImageModel bool      `gorm:"not null;default:false" json:"is_image_model"`
	IsVideoModel bool      `gorm:"not null;default:false" json:"is_video_model"`
	IsAudioModel bool      `gorm:"not null;default:false" json:"is_audio_model"`
	Description  string    `gorm:"type:text" json:"description"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName 指定表名
func (CustomModelConfig) TableName() string {
	return "custom_model_configs"
}

// Validate 验证配置合法性
func (c *CustomModelConfig) Validate() error {
	// 模型名称不能为空
	trimmed := strings.TrimSpace(c.ModelName)
	if trimmed == "" {
		return ErrCustomModelConfigInvalidName
	}
	c.ModelName = trimmed

	// 至少需要启用一个能力
	if !c.IsImageModel && !c.IsVideoModel && !c.IsAudioModel {
		return ErrCustomModelConfigNoCapability
	}

	return nil
}

// HasImageCapability 是否支持图片生成
func (c *CustomModelConfig) HasImageCapability() bool {
	return c.IsImageModel
}

// HasVideoCapability 是否支持视频生成
func (c *CustomModelConfig) HasVideoCapability() bool {
	return c.IsVideoModel
}

// HasAudioCapability 是否支持音频生成
func (c *CustomModelConfig) HasAudioCapability() bool {
	return c.IsAudioModel
}
