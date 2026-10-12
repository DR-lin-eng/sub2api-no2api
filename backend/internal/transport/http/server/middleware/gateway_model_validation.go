package middleware

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/shared/httputil"
	"github.com/Wei-Shaw/sub2api/internal/shared/requestmodel"
	"github.com/gin-gonic/gin"
)

// GatewayModelValidation runs independently of the optional model allowlist,
// before routing, model rewriting, scheduling, or billing interprets the body.
func GatewayModelValidation() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			c.Next()
			return
		}
		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			status, message := http.StatusBadRequest, "Failed to read request body"
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status, message = http.StatusRequestEntityTooLarge, "Request body is too large"
			}
			groupModelAllowlistErrorWriter(c)(c, status, message)
			c.Abort()
			return
		}
		requestmodel.ResetRequestBody(c.Request, body)
		if err := requestmodel.ValidateModelFields(c.GetHeader("Content-Type"), body); err != nil {
			groupModelAllowlistErrorWriter(c)(c, http.StatusBadRequest, err.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}
