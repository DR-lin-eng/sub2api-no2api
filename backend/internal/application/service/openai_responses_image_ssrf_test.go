package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesImagePlanRejectsPrivateImagesAndMasks(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, mask := range []bool{false, true} {
			for _, rawURL := range []string{
				"http://127.0.0.1/admin", "http://10.0.0.1/image.png", "http://172.16.0.1/image.png",
				"http://192.168.1.1/image.png", "http://169.254.169.254/latest/meta-data/",
				"http://100.100.100.200/latest/meta-data/", "http://localhost/image.png",
				"http://[::1]/image.png", "http://[::ffff:127.0.0.1]/image.png",
				"http://[fc00::1]/image.png", "http://[fe80::1%25eth0]/image.png",
				"file:///etc/passwd", "gopher://127.0.0.1/",
			} {
				name := rawURL
				if stream {
					name += "/stream"
				}
				if mask {
					name += "/mask"
				}
				t.Run(name, func(t *testing.T) {
					stub := &openAIResponsesImagePlanHTTPStub{}
					cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
						AllowPrivateHosts: true, AllowInsecureHTTP: true,
					}}}
					svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: stub}
					body := responsesImageSSRFTestBody(t, rawURL, mask, stream)
					plan, err := CompileOpenAIResponsesImagePlan(body)
					require.NoError(t, err)
					c, _ := testOpenAIResponsesImageContext(body)
					err = svc.PrepareOpenAIResponsesImagePlan(t.Context(), c, plan)
					require.Error(t, err)
					require.Zero(t, stub.callCount(), "unsafe input contacted the HTTP port")
					_, err = svc.buildOpenAIResponsesImageAPIBridgeRequest(t.Context(), c, nil, body, "gpt-5.4", "gpt-image-2")
					require.Error(t, err)
					require.Zero(t, stub.callCount(), "legacy bridge contacted the HTTP port")
				})
			}
		}
	}
}

func TestOpenAIResponsesImageDownloadRequiresPublicDestinationPolicy(t *testing.T) {
	stub := &openAIResponsesImagePlanHTTPStub{response: func(req *http.Request) *http.Response {
		require.True(t, HTTPUpstreamPublicDestination(req.Context()))
		require.Empty(t, req.Header.Get("Authorization"))
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(bytes.NewReader(testOpenAIResponsesPNG()))}
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: stub}
	for _, mask := range []bool{false, true} {
		body := responsesImageSSRFTestBody(t, "https://cdn.example.com/image.png", mask, false)
		plan, err := CompileOpenAIResponsesImagePlan(body)
		require.NoError(t, err)
		c, _ := testOpenAIResponsesImageContext(body)
		require.NoError(t, svc.PrepareOpenAIResponsesImagePlan(context.Background(), c, plan))
	}
	require.Equal(t, 2, stub.callCount())
}

func responsesImageSSRFTestBody(t *testing.T, rawURL string, mask, stream bool) []byte {
	t.Helper()
	inputURL := rawURL
	tool := map[string]any{"type": "image_generation", "action": "edit", "model": "gpt-image-2"}
	if mask {
		inputURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(testOpenAIResponsesPNG())
		tool["input_image_mask"] = map[string]any{"image_url": rawURL}
	}
	body, err := json.Marshal(map[string]any{
		"model": "gpt-5.4", "stream": stream,
		"input": []any{map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "input_text", "text": "Edit this image"},
			map[string]any{"type": "input_image", "image_url": inputURL},
		}}},
		"tools": []any{tool},
	})
	require.NoError(t, err)
	return body
}
