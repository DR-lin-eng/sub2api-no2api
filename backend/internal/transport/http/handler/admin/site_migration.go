package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/sitemigration"
	"github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

// SiteMigration only accepts an explicitly scoped machine principal, even if
// mounted behind a different admin dispatcher in the future.
func SiteMigration(store sitemigration.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		mode, op := c.Param("mode"), c.Param("action")
		id := c.Query("id")
		scopes, _ := c.Get("admin_api_key_scopes")
		grant, _ := scopes.([]string)
		owner := c.GetString("admin_api_key_id")
		c.Header("Cache-Control", "no-store")
		c.Header(sitemigration.ClonePolicyHeader, sitemigration.ClonePolicyVersion)
		if !middleware.RequestIsHTTPS(c) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		full := slices.Contains(grant, "admin.migration."+mode)
		userKeys := slices.Contains(grant, "admin.migration.userkeys."+mode)
		if owner == "" || owner == "legacy" || c.GetString("auth_method") != "admin_api_key" || (mode != "import" && mode != "export") || (!full && !userKeys && !slices.Contains(grant, "admin.migration.users."+mode)) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		a := sitemigration.Auth{Owner: "remote/" + owner, Key: c.GetHeader("x-api-key"), UsersOnly: !full, UserKeys: userKeys}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
		defer cancel()
		var state sitemigration.State
		var err error
		switch op {
		case "probe":
			if c.Query("scope") == "clone" && c.GetHeader(sitemigration.ClonePolicyHeader) != sitemigration.ClonePolicyVersion {
				c.Status(http.StatusPreconditionFailed)
				return
			}
			if !a.Allows(c.Query("scope")) {
				c.Status(403)
				return
			}
			var existing bool
			if mode == "import" {
				existing, err = store.Existing(ctx)
			}
			if err == nil {
				c.JSON(200, existing)
				return
			}
		case "open":
			var v struct {
				Profile string
				Public  []byte
			}
			b, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1024))
			if e != nil || json.Unmarshal(b, &v) != nil || len(v.Public) != 32 {
				c.Status(400)
				return
			}
			if v.Profile == "clone" && c.GetHeader(sitemigration.ClonePolicyHeader) != sitemigration.ClonePolicyVersion {
				c.Status(http.StatusPreconditionFailed)
				return
			}
			state, err = store.Open(ctx, a, id, mode, v.Profile, v.Public)
		case "prepare":
			if mode != "export" {
				c.Status(403)
				return
			}
			state, err = store.Prepare(ctx, a, id)
		case "apply":
			if mode != "import" {
				c.Status(403)
				return
			}
			state, err = store.Apply(ctx, a, id)
		case "status":
			state, err = store.Status(ctx, a, id, mode)
		case "delete":
			state, err = store.Remove(ctx, a, id, mode)
		case "block", "report":
			n, e := strconv.Atoi(c.Query("n"))
			if e != nil || n < 0 {
				c.Status(400)
				return
			}
			if op == "block" && mode == "import" {
				b, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, sitemigration.BlockSize+64))
				if e != nil {
					c.Status(413)
					return
				}
				state, err = store.Receive(ctx, a, id, n, b)
			} else {
				var b []byte
				if op == "report" {
					if mode != "import" {
						c.Status(403)
						return
					}
					b, err = store.ReportPage(ctx, a, id, n)
				} else {
					b, err = store.Block(ctx, a, id, mode, 0, n)
				}
				if err == nil {
					c.Data(200, "application/octet-stream", b)
					return
				}
			}
		default:
			c.Status(404)
			return
		}
		if err != nil {
			code := 422
			switch {
			case errors.Is(err, sitemigration.ErrBillingPending):
				code = http.StatusFailedDependency
			case errors.Is(err, sitemigration.ErrDenied):
				code = 403
			case errors.Is(err, sitemigration.ErrMissing):
				code = 404
			case errors.Is(err, sitemigration.ErrInvalid):
				code = 400
			case errors.Is(err, sitemigration.ErrConflict):
				code = 409
			case errors.Is(err, sitemigration.ErrLimit):
				code = 413
			}
			// Driver errors can contain row values (including passwords and tokens).
			c.Status(code)
			return
		}
		c.JSON(200, state)
	}
}
