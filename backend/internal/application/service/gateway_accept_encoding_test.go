//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestGatewayServiceAcceptEncodingOnWire guards the raw-header casing used by
// buildUpstreamRequest.  A lower-case map key bypasses net/http's automatic
// gzip negotiation and can result in two wire fields when a caller supplied
// Accept-Encoding; canonical casing must produce exactly one value on HTTP/1
// and HTTP/2 alike.
func TestGatewayServiceAcceptEncodingOnWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []struct {
		name  string
		major int
	}{
		{name: "HTTP1", major: 1},
		{name: "HTTP2", major: 2},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			type received struct {
				values []string
				major  int
			}
			gotCh := make(chan received, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotCh <- received{values: r.Header.Values("Accept-Encoding"), major: r.ProtoMajor}
				w.WriteHeader(http.StatusNoContent)
			}))
			server.EnableHTTP2 = protocol.major == 2
			server.StartTLS()
			defer server.Close()

			client := server.Client()
			client.Timeout = 5 * time.Second
			account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"base_url": server.URL, "api_key": "test-key"}}
			for _, tc := range []struct {
				name     string
				encoding string
				want     string
			}{
				{name: "gzip", encoding: "gzip", want: "gzip"},
				{name: "multiple", encoding: "gzip, deflate, br", want: "gzip, deflate, br"},
				{name: "identity", encoding: "identity", want: "identity"},
				{name: "omitted", want: "gzip"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					if tc.encoding != "" {
						c.Request.Header.Set("Accept-Encoding", tc.encoding)
					}
					svc := &GatewayService{cfg: &config.Config{}}
					req, _, err := svc.buildUpstreamRequest(context.Background(), c, account,
						[]byte(`{"model":"claude-sonnet-4-6","messages":[]}`), "test-token", "oauth", "claude-sonnet-4-6", false, false)
					require.NoError(t, err)
					// buildUpstreamRequest validates and sets the account URL; use the
					// test server's TLS transport while retaining the generated path.
					req.URL.Scheme = "https"
					req.URL.Host = req.URL.Host
					// The account URL is already the test server URL. The client from
					// httptest trusts its ephemeral certificate.
					resp, err := client.Do(req)
					require.NoError(t, err)
					_, err = io.Copy(io.Discard, resp.Body)
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					wire := <-gotCh
					require.Equal(t, protocol.major, wire.major)
					require.Equal(t, []string{tc.want}, wire.values)
				})
			}
		})
	}
}
