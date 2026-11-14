package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
)

// Voucher secrets grant spendable value, so generic read/write wildcards do
// not imply disclosure. Existing human administrator authorization is retained.
func CanReadRedeemSecrets(c *gin.Context) bool {
	if c.GetString("auth_method") != "admin_api_key" {
		return true
	}
	value, exists := c.Get("admin_api_key_scopes")
	if !exists {
		return false
	}
	scopes, ok := value.([]string)
	if !ok {
		return false
	}
	for _, scope := range scopes {
		if scope == service.AdminAPIKeyScopeRedeemSecrets {
			return true
		}
	}
	return false
}

func RequireRedeemSecrets(c *gin.Context) bool {
	if CanReadRedeemSecrets(c) {
		return true
	}
	AbortWithError(c, 403, "ADMIN_API_KEY_SCOPE_REQUIRED", "Admin API key requires admin.redeem.secrets for voucher secrets")
	return false
}
