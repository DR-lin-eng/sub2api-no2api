package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

const maxEmbeddedCapabilityRequestBytes = 16 << 10

type issueEmbeddedCapabilityRequest struct {
	MenuID       string `json:"menu_id" binding:"required"`
	TargetOrigin string `json:"target_origin" binding:"required"`
}

type verifyEmbeddedCapabilityRequest struct {
	Token    string `json:"token" binding:"required"`
	Audience string `json:"audience" binding:"required"`
}

// IssueEmbeddedCapability returns a short-lived permission proof signed in a
// domain that the normal Sub2API JWT middleware does not accept.
func (h *AuthHandler) IssueEmbeddedCapability(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxEmbeddedCapabilityRequestBytes)
	var req issueEmbeddedCapabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	target, err := h.settingSvc.ResolveEmbeddedCapabilityTarget(
		c.Request.Context(),
		req.MenuID,
		req.TargetOrigin,
		user.Role,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	issued, err := h.authService.IssueEmbeddedCapability(user, target)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"token":         issued.Token,
		"token_type":    "embedded_capability",
		"expires_at":    issued.ExpiresAt,
		"menu_id":       issued.Target.MenuID,
		"target_origin": issued.Target.Origin,
	})
}

// VerifyEmbeddedCapability performs permission introspection only. It never
// creates a session, refresh token, cookie, or access token.
func (h *AuthHandler) VerifyEmbeddedCapability(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxEmbeddedCapabilityRequestBytes)
	var req verifyEmbeddedCapabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	claims, err := h.authService.VerifyEmbeddedCapability(c.Request.Context(), req.Token, req.Audience)
	if err != nil {
		if errors.Is(err, service.ErrEmbeddedCapabilityInvalid) {
			response.Unauthorized(c, "Invalid embedded capability")
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	if requestOrigin := strings.TrimSpace(c.GetHeader("Origin")); requestOrigin != "" && requestOrigin != claims.Origin {
		response.Forbidden(c, "Embedded capability audience mismatch")
		return
	}
	response.Success(c, gin.H{
		"valid":       true,
		"user_id":     claims.UserID,
		"role":        claims.Role,
		"menu_id":     claims.MenuID,
		"audience":    claims.Origin,
		"permissions": claims.Permissions,
		"expires_at":  claims.ExpiresAt,
	})
}
