package admin

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"
	"github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"
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

// A migration-capable key can export user credentials. Creating scope grants
// and rotating the secret therefore observes the site's interactive step-up policy.
func (h *SettingHandler) authorizeKeyGrant(c *gin.Context) bool {
	enabled, err := h.settingService.ReadStepUpEnabled(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Unable to verify step-up policy")
		return false
	}
	if !enabled {
		return true
	}
	if h.totpService == nil || h.userService == nil {
		middleware.AbortWithError(c, 503, "STEP_UP_UNAVAILABLE", "Step-up verification service unavailable")
		return false
	}
	return middleware.EnforceStepUpAlways(c, h.totpService, h.userService)
}

// CreateAdminAPIKey creates a scoped machine credential. The full key is returned once.
func (h *SettingHandler) CreateAdminAPIKey(c *gin.Context) {
	var req adminAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if migrationScopes(req.Scopes) && !h.authorizeKeyGrant(c) {
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
	// Extending an expired key can re-enable an existing migration grant without a scopes field.
	if (req.ExpiresAt.Present || (req.Scopes != nil && migrationScopes(*req.Scopes))) && !h.authorizeKeyGrant(c) {
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

func migrationScopes(scopes []string) bool {
	for _, scope := range scopes {
		if strings.HasPrefix(strings.TrimSpace(scope), "admin.migration.") {
			return true
		}
	}
	return false
}
func (h *SettingHandler) RotateAdminAPIKey(c *gin.Context) {
	// Require the grant on rotations since this operation returns a fresh
	// secret and another administrator may update scopes concurrently.
	if !h.authorizeKeyGrant(c) {
		return
	}
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
