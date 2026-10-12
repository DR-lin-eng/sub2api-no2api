//go:build ssrf_docker

package repository_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/infrastructure/repository"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The script attaches three internal Docker networks. The public-looking IP
// below belongs only to this container's isolated fixture; no internet traffic
// or real cloud metadata is used.
func TestResponsesImageSSRFDocker(t *testing.T) {
	if os.Getenv("RUN_RESPONSES_IMAGE_SSRF_DOCKER") != "1" {
		t.Skip("run backend/scripts/test-responses-image-ssrf.sh for isolated Docker fixtures")
	}
	baseline := os.Getenv("SSRF_EXPECT_VULNERABLE") == "1"
	gin.SetMode(gin.TestMode)
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}
	var mu sync.Mutex
	hits := map[string]int{}
	count := func(key string) int { mu.Lock(); defer mu.Unlock(); return hits[key] }
	var privateURL, metadataURL, publicURL string
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	privateURL = "http://10.253.77.20:" + port
	metadataURL = "http://169.254.169.254:" + port
	publicURL = "http://public-image.test:" + port
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("case")
		switch r.URL.Path {
		case "/private-image", "/latest/meta-data/iam/security-credentials/fixture":
			mu.Lock()
			hits[key]++
			mu.Unlock()
			if strings.HasPrefix(r.URL.Path, "/latest/") {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"Role":"ssrf-docker-fixture"}`)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		case "/redirect-private":
			http.Redirect(w, r, privateURL+"/private-image?"+r.URL.RawQuery, http.StatusFound)
		case "/redirect-metadata":
			http.Redirect(w, r, metadataURL+"/latest/meta-data/iam/security-credentials/fixture?"+r.URL.RawQuery, http.StatusFound)
		case "/redirect-private-dns":
			http.Redirect(w, r, "http://private-image.test:"+port+"/private-image?"+r.URL.RawQuery, http.StatusFound)
		case "/redirect-public":
			http.Redirect(w, r, "/public-image", http.StatusFound)
		case "/v1/images/edits":
			mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "multipart/form-data" {
				http.Error(w, "invalid multipart", 400)
				return
			}
			form, err := multipart.NewReader(r.Body, params["boundary"]).ReadForm(1 << 20)
			if err != nil {
				http.Error(w, "invalid multipart", 400)
				return
			}
			defer func() { _ = form.RemoveAll() }()
			if len(form.File["image"]) != 1 {
				http.Error(w, "missing image", 400)
				return
			}
			image, err := form.File["image"][0].Open()
			if err != nil {
				http.Error(w, "cannot open image", 400)
				return
			}
			data, err := io.ReadAll(image)
			_ = image.Close()
			if err != nil || !bytes.Equal(png, data) {
				http.Error(w, "image bytes changed", 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"created": 1710000010, "data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(png)}}})
		default:
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	newGateway := func(enabled, allowPrivate bool) *service.OpenAIGatewayService {
		cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled: enabled, AllowPrivateHosts: allowPrivate, AllowInsecureHTTP: true,
			UpstreamHosts: []string{"api.openai.com"},
		}}}
		return service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil,
			repository.NewHTTPUpstream(cfg, nil), nil, nil, nil, nil, nil, nil, nil, nil)
	}
	targets := []struct {
		name, rawURL       string
		indirect, nonImage bool
	}{
		{"private_literal", privateURL + "/private-image", false, false},
		{"loopback", "http://127.0.0.1:" + port + "/private-image", false, false},
		{"metadata", metadataURL + "/latest/meta-data/iam/security-credentials/fixture", false, true},
		{"private_dns", "http://private-image.test:" + port + "/private-image", true, false},
		{"redirect_private", publicURL + "/redirect-private", true, false},
		{"redirect_metadata", publicURL + "/redirect-metadata", true, true},
		{"redirect_private_dns", publicURL + "/redirect-private-dns", true, false},
	}
	for _, mode := range []struct {
		name                  string
		enabled, allowPrivate bool
	}{
		{"defaults", false, true}, {"private_off_only", false, false}, {"strict_upstream", true, false},
	} {
		gateway := newGateway(mode.enabled, mode.allowPrivate)
		for _, target := range targets {
			for _, stream := range []bool{false, true} {
				for _, mask := range []bool{false, true} {
					name := fmt.Sprintf("%s/%s/stream=%t/mask=%t", mode.name, target.name, stream, mask)
					t.Run(name, func(t *testing.T) {
						parsed, err := url.Parse(target.rawURL)
						require.NoError(t, err)
						query := parsed.Query()
						query.Set("case", name)
						parsed.RawQuery = query.Encode()
						body := dockerResponsesImageBody(t, parsed.String(), png, stream, mask)
						plan, err := service.CompileOpenAIResponsesImagePlan(body)
						require.NoError(t, err)
						c, _ := dockerResponsesContext(body)
						err = gateway.PrepareOpenAIResponsesImagePlan(t.Context(), c, plan)
						wantHit := baseline && (mode.allowPrivate || (!mode.enabled && target.indirect))
						if wantHit {
							require.Equal(t, 1, count(name), "baseline SSRF did not reach fixture")
							if target.nonImage {
								require.ErrorContains(t, err, "non-image")
							} else {
								require.NoError(t, err)
							}
						} else {
							require.Error(t, err, "unsafe image was accepted")
							require.Zero(t, count(name), "unsafe target was contacted before rejection")
						}
						if !stream && !mask {
							t.Logf("EVIDENCE mode=%s target=%s requests=%d rejected=%t", mode.name, target.name, count(name), err != nil)
						}
					})
				}
			}
		}
	}
	// Exercise the real multipart Images forwarding and both Responses output
	// formats, including a relative public redirect after DNS pinning.
	gateway := newGateway(false, true)
	account := &service.Account{ID: 77, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "docker-fixture-only", "base_url": publicURL + "/v1"},
		Extra:       map[string]any{"openai_force_image_api": true}}
	for _, stream := range []bool{false, true} {
		for _, source := range []string{"/public-image", "/redirect-public"} {
			t.Run(fmt.Sprintf("public_compatibility/stream=%t/%s", stream, source), func(t *testing.T) {
				body := dockerResponsesImageBody(t, publicURL+source, png, stream, false)
				c, recorder := dockerResponsesContext(body)
				result, err := gateway.ForwardOpenAIResponsesViaImagesAPI(t.Context(), c, account, body, "gpt-5.4", "gpt-image-2", time.Now())
				require.NoError(t, err)
				require.Equal(t, "/v1/images/edits", result.UpstreamEndpoint)
				require.Equal(t, http.StatusOK, recorder.Code)
				if stream {
					require.Contains(t, recorder.Body.String(), "response.completed")
				} else {
					require.Contains(t, recorder.Body.String(), `"status":"completed"`)
				}
			})
		}
	}
}

func dockerResponsesContext(body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, recorder
}

func dockerResponsesImageBody(t *testing.T, rawURL string, png []byte, stream, mask bool) []byte {
	t.Helper()
	inputURL := rawURL
	tool := map[string]any{"type": "image_generation", "model": "gpt-image-2", "action": "edit"}
	if mask {
		inputURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
		tool["input_image_mask"] = map[string]any{"image_url": rawURL}
	}
	body, err := json.Marshal(map[string]any{"model": "gpt-5.4", "stream": stream, "tools": []any{tool},
		"input": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "input_text", "text": "Edit this image"}, map[string]any{"type": "input_image", "image_url": inputURL},
		}}}})
	require.NoError(t, err)
	return body
}
