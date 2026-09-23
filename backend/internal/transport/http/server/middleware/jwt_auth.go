package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"

	"github.com/gin-gonic/gin"
)

// NewJWTAuthMiddleware 创建 JWT 认证中间件
func NewJWTAuthMiddleware(
	authService *service.AuthService,
	userService *service.UserService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
) JWTAuthMiddleware {
	return JWTAuthMiddleware(jwtAuth(authService, userService, userService, settingService, auditService))
}

type jwtUserReader interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

type userActivityToucher interface {
	TouchLastActiveForUser(ctx context.Context, user *service.User)
}

// jwtAuth JWT认证中间件实现
func jwtAuth(
	authService *service.AuthService,
	userService jwtUserReader,
	activityToucher userActivityToucher,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// WebSocket upgrade requests cannot set Authorization headers in browsers.
		// For authenticated user WebSocket endpoints (e.g. chat), allow passing the
		// JWT via Sec-WebSocket-Protocol, mirroring adminAuth's handling:
		//   Sec-WebSocket-Protocol: sub2api-chat, jwt.<token>
		if isWebSocketUpgradeRequest(c) {
			if token := extractJWTFromWebSocketSubprotocol(c); token != "" {
				if !validateJWTForUser(c, token, authService, userService, activityToucher, settingService, auditService) {
					return
				}
				c.Next()
				return
			}
		}

		// 从Authorization header中提取token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			AbortWithError(c, 401, "UNAUTHORIZED", "Authorization header is required")
			return
		}

		// 验证Bearer scheme
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			AbortWithError(c, 401, "INVALID_AUTH_HEADER", "Authorization header format must be 'Bearer {token}'")
			return
		}

		tokenString := strings.TrimSpace(parts[1])
		if tokenString == "" {
			AbortWithError(c, 401, "EMPTY_TOKEN", "Token cannot be empty")
			return
		}

		if !validateJWTForUser(c, tokenString, authService, userService, activityToucher, settingService, auditService) {
			return
		}

		c.Next()
	}
}

// validateJWTForUser validates token and, on success, populates the gin
// context with the authenticated user (AuthSubject/role/email/session).
// Shared by the header-based path above and the WebSocket subprotocol path,
// since both need identical token/user/session-binding checks.
func validateJWTForUser(
	c *gin.Context,
	token string,
	authService *service.AuthService,
	userService jwtUserReader,
	activityToucher userActivityToucher,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
) bool {
	claims, err := authService.ValidateToken(token)
	if err != nil {
		if errors.Is(err, service.ErrTokenExpired) {
			AbortWithError(c, 401, "TOKEN_EXPIRED", "Token has expired")
			return false
		}
		AbortWithError(c, 401, "INVALID_TOKEN", "Invalid token")
		return false
	}

	// 从数据库获取最新的用户信息
	user, err := userService.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			AbortWithError(c, 401, "USER_NOT_FOUND", "User not found")
		} else {
			// A transient database/cache failure must not look like token
			// invalidation; clients can retry without destroying the session.
			AbortWithError(c, 500, "INTERNAL_ERROR", "Failed to load user")
		}
		return false
	}

	// 检查用户状态
	if !user.IsActive() {
		AbortWithError(c, 401, "USER_INACTIVE", "User account is not active")
		return false
	}

	// Security: Validate TokenVersion to ensure token hasn't been invalidated
	// This check ensures tokens issued before a password change are rejected
	if claims.TokenVersion != user.TokenVersion {
		AbortWithError(c, 401, "TOKEN_REVOKED", "Token has been revoked (password changed)")
		return false
	}

	// 会话绑定校验：始终绑定 UA，按可信代理配置可选绑定 IP（功能可在系统设置中关闭）
	if !enforceSessionBinding(c, authService, settingService, auditService, claims) {
		return false
	}

	setAuthSubject(c, user.ID, user.Concurrency, user.SchedulingTier)
	c.Set(string(ContextKeyUserRole), user.Role)
	c.Set(ContextKeyAuthEmail, user.Email)
	c.Set(ContextKeySessionID, claims.SessionID)
	if claims.ExpiresAt != nil {
		c.Set(string(ContextKeyJWTExpiresAt), claims.ExpiresAt.Time)
	}
	if activityToucher != nil {
		activityToucher.TouchLastActiveForUser(c.Request.Context(), user)
	}
	return true
}

// Deprecated: prefer GetAuthSubjectFromContext in auth_subject.go.
