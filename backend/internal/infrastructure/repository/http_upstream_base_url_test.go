//go:build unit

package repository

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestHTTPUpstreamConfiguredHTTPWithoutOptIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/v1/models", http.StatusFound)
			return
		}
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, "Bearer fixture-only", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"fixture-model"}]}`)
	}))
	t.Cleanup(server.Close)
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		AllowPrivateHosts: true,
	}}}
	upstream := NewHTTPUpstream(cfg, nil)
	for _, path := range []string{"/v1/models", "/redirect"} {
		t.Run(path, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer fixture-only")
			resp, err := upstream.Do(req, "", 71, 1)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.JSONEq(t, `{"data":[{"id":"fixture-model"}]}`, string(body))
			t.Logf("configured HTTP upstream: HTTP %d %s", resp.StatusCode, body)
		})
	}
}
