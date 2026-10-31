package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
)

const multimodalVideoOwnerResolverKey = "multimodal_video_owner_resolver"
const multimodalVideoOwnerResultKey = "multimodal_video_owner_result"

type MultimodalVideoOwnerResolver func(context.Context, *service.APIKey, string) (int64, error)

type multimodalVideoOwnerResult struct {
	requestID                            string
	userID, apiKeyID, groupID, accountID int64
}

// WithMultimodalVideoOwnerResolver supplies a trusted routing capability to the
// existing auth middleware. The resolver is never invoked before authentication
// or outside composite video reads, and cannot be supplied through HTTP input.
func WithMultimodalVideoOwnerResolver(auth APIKeyAuthMiddleware, resolve MultimodalVideoOwnerResolver) APIKeyAuthMiddleware {
	return func(c *gin.Context) {
		if resolve != nil {
			c.Set(multimodalVideoOwnerResolverKey, resolve)
		}
		auth(c)
	}
}

func resolveCompositeMultimodalVideoOwner(c *gin.Context, apiKey *service.APIKey) (bool, error) {
	if apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformComposite ||
		!isMultimodalVideoTaskRead(c.Request.Method, c.Request.URL.Path) {
		return false, nil
	}
	if !apiKey.IsActive() || apiKey.IsExpired() || apiKey.IsQuotaExhausted() {
		return false, nil
	}
	value, _ := c.Get(multimodalVideoOwnerResolverKey)
	resolve, ok := value.(MultimodalVideoOwnerResolver)
	if !ok || resolve == nil {
		return false, nil
	}
	requestID := c.Param("request_id")
	accountID, err := resolve(c.Request.Context(), apiKey, requestID)
	if err != nil {
		return false, err
	}
	SetMultimodalVideoTaskAccount(c, apiKey, requestID, accountID)
	return accountID > 0, nil
}

// SetMultimodalVideoTaskAccount caches the result only for this authenticated
// owner, group and request ID. Zero records a checked-but-unmatched lookup.
func SetMultimodalVideoTaskAccount(c *gin.Context, apiKey *service.APIKey, requestID string, accountID int64) {
	if c == nil || apiKey == nil || apiKey.GroupID == nil {
		return
	}
	c.Set(multimodalVideoOwnerResultKey, multimodalVideoOwnerResult{
		requestID: requestID, userID: apiKey.UserID, apiKeyID: apiKey.ID,
		groupID: *apiKey.GroupID, accountID: accountID,
	})
}

func GetMultimodalVideoTaskAccount(c *gin.Context, apiKey *service.APIKey, requestID string) (int64, bool) {
	if c == nil || apiKey == nil || apiKey.GroupID == nil {
		return 0, false
	}
	value, _ := c.Get(multimodalVideoOwnerResultKey)
	owner, ok := value.(multimodalVideoOwnerResult)
	if !ok || owner.requestID != requestID || owner.userID != apiKey.UserID ||
		owner.apiKeyID != apiKey.ID || owner.groupID != *apiKey.GroupID {
		return 0, false
	}
	return owner.accountID, true
}

func isMultimodalVideoTaskRead(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 0 && parts[0] == "v1" {
		parts = parts[1:]
	}
	if (len(parts) != 2 && len(parts) != 3) || parts[0] != "videos" ||
		strings.TrimSpace(parts[1]) == "" || len(parts[1]) > 255 {
		return false
	}
	switch parts[1] {
	case "generations", "edits", "extensions":
		return false
	}
	return len(parts) == 2 || parts[2] == "content"
}
