package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SessionBindingContext 全局中间件：将请求会话绑定注入 request context，供 token
// 签发路径（登录 / 刷新 / OAuth 回调）读取并写入会话绑定。trusted_proxies 为空时仅绑定
// User-Agent；成功配置后才通过 GetTrustedClientIP（走 trusted_proxies 链）绑定客户端 IP。
func SessionBindingContext(includeIP bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		binding := newSessionBinding(c, includeIP)
		c.Request = c.Request.WithContext(service.WithSessionBinding(c.Request.Context(), binding))
		c.Next()
	}
}

// requestSessionBinding 返回当前请求的会话指纹，优先取 SessionBindingContext
// 注入的解析结果；注入缺失时只保留 User-Agent，避免重新引入不确定的 IP 语义。
func requestSessionBinding(c *gin.Context) *service.SessionBinding {
	if binding := service.SessionBindingFromContext(c.Request.Context()); binding != nil {
		return binding
	}
	return &service.SessionBinding{UserAgent: normalizePersistentText(c.Request.UserAgent(), maxPersistentUserAgentBytes)}
}

// SecurityClientIP 返回当前请求用于安全敏感记录（审计日志等）的客户端 IP。
// 与会话绑定、API Key IP 限制共用 Gin trusted_proxies 信任链。
func SecurityClientIP(c *gin.Context) string {
	if binding := service.SessionBindingFromContext(c.Request.Context()); binding != nil &&
		strings.TrimSpace(binding.IP) != "" {
		return binding.IP
	}
	return ip.GetTrustedClientIP(c)
}

func newSessionBinding(c *gin.Context, includeIP bool) *service.SessionBinding {
	userAgent := normalizePersistentText(c.Request.UserAgent(), maxPersistentUserAgentBytes)
	c.Request.Header.Set("User-Agent", userAgent)
	binding := &service.SessionBinding{UserAgent: userAgent}
	if includeIP {
		binding.IP = ip.GetTrustedClientIP(c)
	}
	return binding
}

// enforceSessionBinding 校验 access token 的会话指纹（始终绑定 UA，按可信代理配置可选绑定 IP）。
// 指纹不匹配时：撤销该会话家族的所有 refresh token、写入审计安全事件、返回 401。
// 返回 false 表示请求已被中断。
//
// 兼容性：claims.BindingHash 为空（功能上线前签发的旧 token）时放行，
// 该会话在下一次 refresh 轮转时会自动获得绑定。
func enforceSessionBinding(
	c *gin.Context,
	authService *service.AuthService,
	settingService *service.SettingService,
	auditService *service.AuditLogService,
	claims *service.JWTClaims,
) bool {
	if settingService == nil || !settingService.IsSessionBindingEnabled(c.Request.Context()) {
		return true
	}
	if claims == nil || claims.BindingHash == "" {
		return true
	}
	binding := requestSessionBinding(c)
	current := binding.Hash()
	if current == "" || current == claims.BindingHash {
		return true
	}

	if authService != nil {
		_ = authService.RevokeSessionFamily(c.Request.Context(), claims.SessionID)
	}
	if auditService != nil {
		uid := claims.UserID
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		auditService.Record(&service.AuditLog{
			ActorUserID: &uid,
			ActorEmail:  claims.Email,
			ActorRole:   claims.Role,
			AuthMethod:  service.AuditAuthMethodJWT,
			Action:      service.AuditActionSessionBindingMismatch,
			Method:      c.Request.Method,
			Path:        path,
			ClientIP:    binding.IP,
			UserAgent:   normalizePersistentText(c.Request.UserAgent(), maxPersistentUserAgentBytes),
			StatusCode:  401,
		})
	}
	AbortWithError(c, 401, "SESSION_BINDING_MISMATCH", "Session network fingerprint changed, please login again")
	return false
}
