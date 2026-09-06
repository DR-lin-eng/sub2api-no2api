package admin

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// ListPermissionGroups exposes names/IDs for role selectors without exposing system settings.
func (h *UserHandler) ListPermissionGroups(c *gin.Context) {
	groups, err := h.settingService.GetPermissionGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"groups": groups})
}

// BasicUser is an explicit allowlist for the support profile dialog. It never
// embeds the admin DTO, API keys, identity bindings or notification recipients.
type BasicUser struct {
	ID               int64                         `json:"id"`
	Email            string                        `json:"email"`
	Username         string                        `json:"username"`
	Role             string                        `json:"role"`
	Status           string                        `json:"status"`
	Balance          float64                       `json:"balance"`
	AvailableBalance *float64                      `json:"available_balance,omitempty"`
	Concurrency      int                           `json:"concurrency"`
	RPMLimit         int                           `json:"rpm_limit"`
	Notes            string                        `json:"notes"`
	SchedulingTier   service.RequestSchedulingTier `json:"scheduling_tier"`
	CreatedAt        time.Time                     `json:"created_at"`
	LastActiveAt     *time.Time                    `json:"last_active_at"`
	LastUsedAt       *time.Time                    `json:"last_used_at"`
}

func (h *UserHandler) GetBasic(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return
	}
	user, err := h.adminService.GetUser(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, BasicUser{ID: user.ID, Email: user.Email, Username: user.Username,
		Role: user.Role, Status: user.Status, Balance: user.Balance, AvailableBalance: user.AvailableBalance,
		Concurrency: user.Concurrency, RPMLimit: user.RPMLimit, Notes: user.Notes, SchedulingTier: user.SchedulingTier,
		CreatedAt: user.CreatedAt, LastActiveAt: user.LastActiveAt, LastUsedAt: user.LastUsedAt})
}

// RequireManagedUser prevents delegated staff from taking over other staff accounts.
// It also checks every explicit batch target before allowing any writes.
func (h *UserHandler) RequireManagedUser(c *gin.Context) {
	role, ok := middleware.GetUserRoleFromContext(c)
	if !ok || role == service.RoleAdmin {
		c.Next()
		return
	}
	ids := []int64{}
	if raw := c.Param("id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid user ID")
			c.Abort()
			return
		}
		ids = append(ids, id)
	} else if c.Request.Method == http.MethodPost && c.FullPath() != "/api/v1/admin/users" {
		var batch struct {
			All     bool    `json:"all"`
			UserIDs []int64 `json:"user_ids"`
		}
		if err := c.ShouldBindBodyWith(&batch, binding.JSON); err != nil {
			response.BadRequest(c, "Invalid batch request")
			c.Abort()
			return
		}
		body, _ := c.Get(gin.BodyBytesKey)
		if data, ok := body.([]byte); ok {
			c.Request.Body = io.NopCloser(bytes.NewReader(data))
		}
		if batch.All || len(batch.UserIDs) > 500 {
			response.Forbidden(c, "Staff must select up to 500 explicit users")
			c.Abort()
			return
		}
		ids = batch.UserIDs
	}
	for _, id := range ids {
		target, err := h.adminService.GetUser(c.Request.Context(), id)
		if err != nil {
			response.ErrorFrom(c, err)
			c.Abort()
			return
		}
		if target.Role != service.RoleUser {
			response.Forbidden(c, "Only administrators can access staff accounts")
			c.Abort()
			return
		}
	}
	c.Next()
}

func delegatedUserFieldsAllowed(c *gin.Context, credentials, billing bool) bool {
	role, ok := middleware.GetUserRoleFromContext(c)
	if !ok || role == service.RoleAdmin {
		return true
	}
	permissions := middleware.GetAdminPermissionsFromContext(c)
	has := func(required string) bool {
		for _, p := range permissions {
			if p == required {
				return true
			}
		}
		return false
	}
	if credentials && !has(service.PermissionUsersCredentials) {
		response.Forbidden(c, "Permission required: users.credentials")
		return false
	}
	if billing && !has(service.PermissionUsersBilling) {
		response.Forbidden(c, "Permission required: users.billing")
		return false
	}
	return true
}
