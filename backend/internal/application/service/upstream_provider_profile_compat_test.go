//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesNativeAnthropicTrimmedBillingModel20261009(t *testing.T) {
	for _, platform := range []string{PlatformKimi, PlatformOpenCodeGo} {
		for _, stream := range []bool{false, true} {
			t.Run(platform+"/"+map[bool]string{true: "stream", false: "buffered"}[stream], func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				c.Request.Header.Set("User-Agent", "fixture")
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(
					"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_fixture\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"kimi-k2\",\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\n" +
						"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
						"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n" +
						"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
						"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
						"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))}}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
				account := &Account{ID: 7618, Platform: platform, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "fixture-key", "api_protocol": APIProtocolAnthropic, "base_url": "https://upstream.example"}}
				body := []byte(`{"model":" kimi-k2 ","stream":` + map[bool]string{true: "true", false: "false"}[stream] + `,"input":"hello"}`)
				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, "kimi-k2", result.BillingModel)
				require.Equal(t, "kimi-k2", gjson.GetBytes(upstream.lastBody, "model").String())
				require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, "/v1/messages"))
				require.Equal(t, 2, result.Usage.OutputTokens)
			})
		}
	}
}
