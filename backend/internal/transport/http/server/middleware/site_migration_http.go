package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/migrationhttp"
	"github.com/gin-gonic/gin"
)

type migrationPeerCredential struct{ key string }

const migrationPeerCredentialContext = "migration_http_peer_credential"

// MigrationPeerKey is the ordinary TLS bearer or the key obtained from the
// authenticated HTTP envelope. Only this exact peer route sets the latter.
func MigrationPeerKey(c *gin.Context) (string, bool) {
	if c.FullPath() == "/api/v1/admin/site-migration/:mode/:action" {
		if v, ok := c.Get(migrationPeerCredentialContext); ok {
			if cred, ok := v.(migrationPeerCredential); ok {
				return cred.key, true
			}
		}
	}
	return c.GetHeader("x-api-key"), false
}
func serveMigrationHTTP(c *gin.Context, settings *service.SettingService, users *service.UserService) {
	const route = "/api/v1/admin/site-migration/:mode/:action"
	if c.FullPath() != route || c.Request.Method != http.MethodPost || len(c.Request.Header.Values(migrationhttp.Header)) != 1 || c.GetHeader(migrationhttp.Header) != migrationhttp.Version || c.Request.URL.EscapedPath() != c.Request.URL.Path || len(c.Request.URL.RequestURI()) > 512 || c.GetHeader("x-api-key") != "" || c.GetHeader("Authorization") != "" || c.GetHeader("Cookie") != "" {
		c.AbortWithStatus(400)
		return
	}
	if c.Request.URL.Path != "/api/v1/admin/site-migration/"+c.Param("mode")+"/"+c.Param("action") {
		c.AbortWithStatus(400)
		return
	}
	// Bind only parameters interpreted by the fixed migration RPC.
	q, e := url.ParseQuery(c.Request.URL.RawQuery)
	if e != nil {
		c.AbortWithStatus(400)
		return
	}
	for k, v := range q {
		allowed := k == "id" || k == "n" || (k == "scope" && c.Param("action") == "probe")
		if len(v) != 1 || !allowed {
			c.AbortWithStatus(400)
			return
		}
	}
	frame, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, migrationhttp.MaxFrame))
	if e != nil {
		c.AbortWithStatus(http.StatusRequestEntityTooLarge)
		return
	}
	opened, e := settings.OpenMigrationHTTP(c.Request, frame)
	if e != nil {
		c.AbortWithStatus(401)
		return
	}
	recorder := &migrationHTTPWriter{ResponseWriter: c.Writer, status: 200}
	original := c.Writer
	c.Writer = recorder
	defer func() {
		c.Writer = original
		// On panic the outer recovery sends an ordinary error; never publish a
		// successful response to a partially executed operation.
		if caught := recover(); caught != nil {
			panic(caught)
		}
		status := recorder.status
		if recorder.overflow {
			status = 500
			recorder.body.Reset()
		}
		body := opened.Reply.Seal(status, recorder.body.Bytes(), original.Header().Get(migrationhttp.CloneHeader))
		original.Header().Set("Cache-Control", "no-store")
		original.Header().Set(migrationhttp.Header, migrationhttp.Version)
		original.Header().Set("Content-Type", "application/octet-stream")
		original.Header().Set("Content-Length", strconv.Itoa(len(body)))
		original.WriteHeader(status)
		_, _ = original.Write(body)
	}()
	if !validateAdminAPIKey(c, opened.Key, settings, users) {
		return
	}
	if c.GetString("admin_api_key_id") == "legacy" || settings.ConsumeMigrationHTTP(c.Request.Context(), opened) != nil {
		c.AbortWithStatus(409)
		return
	}
	c.Set(migrationPeerCredentialContext, migrationPeerCredential{key: opened.Key})
	c.Request.Body = io.NopCloser(bytes.NewReader(opened.Body))
	c.Request.ContentLength = int64(len(opened.Body))
	c.Next()
}

// All current RPC responses fit in a single bounded frame. Capture the ordinary
// response (including errors and scope decisions) before authenticating it.
type migrationHTTPWriter struct {
	gin.ResponseWriter
	body              bytes.Buffer
	status            int
	written, overflow bool
}

func (w *migrationHTTPWriter) WriteHeader(code int) {
	if !w.written {
		w.status = code
	}
}
func (w *migrationHTTPWriter) WriteHeaderNow() { w.written = true }
func (w *migrationHTTPWriter) Status() int     { return w.status }
func (w *migrationHTTPWriter) Size() int {
	if !w.written {
		return -1
	}
	return w.body.Len()
}
func (w *migrationHTTPWriter) Written() bool { return w.written }
func (w *migrationHTTPWriter) Write(b []byte) (int, error) {
	w.written = true
	if w.body.Len()+len(b) > migrationhttp.MaxPayload {
		w.overflow = true
		return 0, io.ErrShortBuffer
	}
	return w.body.Write(b)
}
func (w *migrationHTTPWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *migrationHTTPWriter) Flush()                            { w.WriteHeaderNow() }
