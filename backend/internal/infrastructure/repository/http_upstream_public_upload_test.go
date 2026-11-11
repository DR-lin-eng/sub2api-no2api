package repository

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestPublicImageUploadOnlyAllowsBoundedCredentialFreeHTTPSPut(t *testing.T) {
	for _, tc := range []struct {
		name, method, url, credential string
		upload                        bool
		length                        int64
		want                          string
	}{
		{"signed PNG", "PUT", "https://storage.example/a.png?sig=fixture", "", true, 12, ""},
		{"download context cannot upload", "PUT", "https://storage.example/a.png", "", false, 12, "GET or HEAD"},
		{"POST", "POST", "https://storage.example/a.png", "", true, 12, "HTTPS PUT"},
		{"plain HTTP", "PUT", "http://storage.example/a.png", "", true, 12, "HTTPS PUT"},
		{"bearer", "PUT", "https://storage.example/a.png", "authorization", true, 12, "account credentials"},
		{"cookie", "PUT", "https://storage.example/a.png", "cookie", true, 12, "account credentials"},
		{"workspace", "PUT", "https://storage.example/a.png", "ChatGPT-Account-ID", true, 12, "account credentials"},
		{"empty", "PUT", "https://storage.example/a.png", "", true, 0, "bounded image body"},
		{"unbounded", "PUT", "https://storage.example/a.png", "", true, -1, "bounded image body"},
		{"oversized", "PUT", "https://storage.example/a.png", "", true, 50<<20 + 1, "bounded image body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := service.WithHTTPUpstreamPublicDestination(t.Context())
			if tc.upload {
				ctx = service.WithHTTPUpstreamPublicUpload(ctx)
				require.True(t, service.HTTPUpstreamPublicDestination(ctx))
				require.True(t, service.HTTPUpstreamRedirectsDisabled(ctx))
			}
			req, err := http.NewRequestWithContext(ctx, tc.method, tc.url, strings.NewReader("image-bytes!"))
			require.NoError(t, err)
			req.ContentLength = tc.length
			if tc.credential != "" {
				req.Header[tc.credential] = []string{"fixture-secret"}
			}
			err = validatePublicDestinationMethod(req)
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestPublicImageUploadKeepsPrivateHostBoundary(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(201)
	}))
	defer server.Close()
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowPrivateHosts = true
	upstream := NewHTTPUpstream(cfg, nil)
	req, err := http.NewRequestWithContext(service.WithHTTPUpstreamPublicUpload(t.Context()), http.MethodPut, server.URL, bytes.NewReader([]byte("PNG fixture")))
	require.NoError(t, err)
	_, err = upstream.Do(req, "", 1, 1)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "requires GET or HEAD", "upload must reach public destination checks")
	require.Zero(t, hits.Load(), "private model-relay exceptions cannot permit private storage uploads")
}
