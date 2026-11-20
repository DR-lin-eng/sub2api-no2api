package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/sitemigration"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/migrationhttp"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"
	"github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

// SiteMigrationPanel only accepts a human full administrator over HTTPS.
// The peer protocol remains machine-key-only and has a distinct route.
func SiteMigrationPanel(p *sitemigration.Panel, settings *SettingHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		who, ok := middleware.GetAuthSubjectFromContext(c)
		role, _ := middleware.GetUserRoleFromContext(c)
		if !ok || who.UserID <= 0 || role != service.RoleAdmin || c.GetString("auth_method") != "jwt" {
			middleware.AbortWithError(c, 403, "ADMIN_SESSION_REQUIRED", "Administrator session required")
			return
		}
		if !middleware.RequestIsHTTPS(c) {
			middleware.AbortWithError(c, 403, "MIGRATION_HTTPS_REQUIRED", "HTTPS required for site migration")
			return
		}
		ctx := c.Request.Context()
		if c.Request.Method == http.MethodGet && c.Param("id") == "" {
			jobs, e := p.List(ctx, who.UserID)
			if e != nil {
				panelError(c, e)
				return
			}
			response.Success(c, jobs)
			return
		}
		if settings == nil || settings.settingService == nil {
			response.InternalError(c, "Migration authorization unavailable")
			return
		}
		if !settings.authorizeKeyGrant(c) {
			return
		}
		if c.Request.Method == http.MethodGet {
			e := p.Report(ctx, who.UserID, c.Param("id"), func(size int64) io.Writer {
				c.Header("Content-Disposition", `attachment; filename="migration-errors-`+c.Param("id")+`.csv"`)
				c.Header("Content-Type", "text/csv; charset=utf-8")
				c.Header("Content-Length", strconv.FormatInt(size, 10))
				c.Header("X-Content-Type-Options", "nosniff")
				return c.Writer
			})
			if e != nil && !c.Writer.Written() {
				panelError(c, e)
			}
			return
		}
		if c.ContentType() != "application/json" {
			response.BadRequest(c, "JSON body required")
			return
		}
		var in sitemigration.PanelInput
		dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 2048))
		dec.DisallowUnknownFields()
		if dec.Decode(&in) != nil || dec.Decode(new(json.RawMessage)) != io.EOF {
			response.BadRequest(c, "Invalid migration form")
			return
		}
		resume := c.Param("id") != ""
		if resume {
			if in.AllowHTTP || in.ID != "" || in.Direction != "" || in.Peer != "" || in.Profile != "" {
				response.BadRequest(c, "Resume accepts only a key")
				return
			}
			in.ID = c.Param("id")
		}
		j, e := p.Start(ctx, who.UserID, in, resume)
		if e != nil {
			panelError(c, e)
			return
		}
		response.Accepted(c, j)
	}
}
func panelError(c *gin.Context, e error) {
	switch {
	case errors.Is(e, migrationhttp.ErrKey):
		middleware.AbortWithError(c, 400, "MIGRATION_HTTP_KEY_REQUIRED", "Create or rotate the scoped migration key on the upgraded peer")
	case errors.Is(e, sitemigration.ErrPeerAddress):
		middleware.AbortWithError(c, 400, "MIGRATION_PEER_ADDRESS", "Invalid migration peer")
	case errors.Is(e, sitemigration.ErrInvalid):
		response.BadRequest(c, "Invalid migration request")
	case errors.Is(e, sitemigration.ErrMissing):
		response.NotFound(c, "Migration task not found")
	case errors.Is(e, sitemigration.ErrConflict):
		response.Error(c, 409, "Migration state changed")
	case errors.Is(e, sitemigration.ErrLimit):
		response.Error(c, 429, "Migration limit reached")
	default:
		response.InternalError(c, "Migration service unavailable")
	}
}
