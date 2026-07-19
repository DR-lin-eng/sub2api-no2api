package admin

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type adminAPIKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type adminAPIKeyUpdateRequest struct {
	Name      *string           `json:"name"`
	Scopes    *[]string         `json:"scopes"`
	ExpiresAt optionalAdminTime `json:"expires_at"`
}

type optionalAdminTime struct {
	Present bool
	Value   *time.Time
}

func (v *optionalAdminTime) UnmarshalJSON(data []byte) error {
	v.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		v.Value = nil
		return nil
	}
	var value time.Time
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	v.Value = &value
	return nil
}

func adminAPIKeyCreatedBy(c *gin.Context) int64 {
	value, ok := c.Get(string(middleware.ContextKeyUser))
	if !ok {
		return 0
	}
	subject, ok := value.(middleware.AuthSubject)
	if !ok {
		return 0
	}
	return subject.UserID
}

// ListAdminAPIKeys returns metadata only; key material and digests are never exposed.
func (h *SettingHandler) ListAdminAPIKeys(c *gin.Context) {
	keys, err := h.settingService.ListAdminAPIKeys(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": keys})
}

// CreateAdminAPIKey creates a scoped machine credential. The full key is returned once.
func (h *SettingHandler) CreateAdminAPIKey(c *gin.Context) {
	var req adminAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	key, secret, err := h.settingService.CreateAdminAPIKey(c.Request.Context(), service.AdminAPIKeyCreateInput{
		Name: req.Name, Scopes: req.Scopes, ExpiresAt: req.ExpiresAt,
	}, adminAPIKeyCreatedBy(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, gin.H{"key": secret, "metadata": key})
}

func (h *SettingHandler) UpdateAdminAPIKey(c *gin.Context) {
	var req adminAPIKeyUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	var expiresAt **time.Time
	if req.ExpiresAt.Present {
		expiresAt = &req.ExpiresAt.Value
	}
	key, err := h.settingService.UpdateAdminAPIKey(c.Request.Context(), c.Param("id"), service.AdminAPIKeyUpdateInput{
		Name: req.Name, Scopes: req.Scopes, ExpiresAt: expiresAt,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, key)
}

func (h *SettingHandler) RotateAdminAPIKey(c *gin.Context) {
	key, secret, err := h.settingService.RotateAdminAPIKey(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"key": secret, "metadata": key})
}

func (h *SettingHandler) RevokeAdminAPIKey(c *gin.Context) {
	if err := h.settingService.RevokeAdminAPIKey(c.Request.Context(), c.Param("id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Admin API key revoked"})
}
