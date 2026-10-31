package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSSizeReplaySafety(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"text", `{"input":"full conversation","store":false}`, true},
		{"full tool history", `{"input":[{"type":"function_call","call_id":"call_1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"large result"}],"tools":[{"type":"function","name":"read"}]}`, true},
		{"local namespace tools", `{"input":"hello","tools":[{"type":"namespace","name":"local","tools":[{"type":"function","name":"read"}]}]}`, true},
		{"incremental response", `{"previous_response_id":"resp_server_state","input":"next"}`, false},
		{"missing tool call", `{"input":[{"type":"function_call_output","call_id":"missing","output":"not full history"}]}`, false},
		{"unresolved reference", `{"input":[{"type":"item_reference","id":"server_only"}]}`, false},
		{"store side effect", `{"input":"hello","store":true}`, false},
		{"background side effect", `{"input":"hello","background":true}`, false},
		{"conversation state", `{"input":"hello","conversation":"conv_server_only"}`, false},
		{"hosted tool", `{"input":"hello","tools":[{"type":"mcp"}]}`, false},
		{"hosted image generation", `{"input":"hello","tools":[{"type":"image_generation"}]}`, false},
		{"nested hosted tool", `{"input":"hello","tools":[{"type":"namespace","tools":[{"type":"mcp"}]}]}`, false},
		{"Lite hosted tool", `{"input":[{"type":"additional_tools","tools":[{"type":"web_search"}]}]}`, false},
		{"ambiguous storage", `{"input":"hello","store":false,"store":true}`, false},
		{"ambiguous tool type", `{"input":"hello","tools":[{"type":"function","type":"mcp"}]}`, false},
		{"ambiguous input", `{"input":"hello","input":[{"type":"function_call_output","call_id":"missing"}]}`, false},
		{"invalid JSON", `{"input":`, false},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, openAIWSHTTPReplaySafeBody([]byte(tc.body))) })
	}
}

func TestOpenAIWSSizeHTTPRecoveryGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []string{"normal", "cancelled", "output delivered", "terminal written", "native WS ingress", "incremental enforced", "original store true", "disabled"} {
		t.Run(tc, func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
			account := &Account{ID: 17, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ctx := context.Background()
			body := []byte(`{"input":"full history","store":false}`)
			original := body
			switch tc {
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			case "output delivered":
				_, err := c.Writer.WriteString("data: already delivered\n\n")
				require.NoError(t, err)
			case "terminal written":
				MarkResponseCommitted(c)
			case "native WS ingress":
				SetOpenAIClientTransport(c, OpenAIClientTransportWS)
			case "incremental enforced":
				c.Set(codexSimulationAttemptContextKey, &codexSimulationAttempt{continuation: &codexContinuationAttempt{mode: codexContinuationEnforce, classification: codexContinuationClassification{kind: codexContinuationIncremental}}})
			case "original store true":
				original = []byte(`{"input":"full history","store":true}`)
			case "disabled":
				svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = false
			}
			got := svc.prepareOpenAIWSHTTPFallback(ctx, c, account, wrapOpenAIWSFallback("message_too_big", errors.New("close 1009")), body, original)
			require.Equal(t, tc == "normal", got)
		})
	}
}

func TestOpenAIWSSizeHTTPThresholdAndRetryBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 1024
	account := &Account{ID: 18, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	small := []byte(`{"input":"hello","store":false}`)
	large := []byte(`{"input":"` + strings.Repeat("x", 1024) + `","store":false}`)
	require.False(t, svc.shouldBypassOpenAIWSForSize(context.Background(), c, account, small, small))
	require.True(t, svc.shouldBypassOpenAIWSForSize(context.Background(), c, account, large, large))
	svc.markOpenAIWSHTTPRecovery(c, account, small, "ws_message_too_big_http")
	require.True(t, svc.shouldBypassOpenAIWSForSize(context.Background(), c, account, small, small), "reselecting the same account must not replenish its WS attempt")
	other := &Account{ID: 19, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, svc.shouldBypassOpenAIWSForSize(context.Background(), c, other, small, small))
}

func TestOpenAIWSInlineImageHTTPRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	account := &Account{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"inline image", `{"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}]}`, true},
		{"hosted image tool", `{"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}],"tools":[{"type":"image_generation"}]}`, true},
		{"hosted MCP tool", `{"input":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}],"tools":[{"type":"mcp"}]}`, true},
		{"explicit storage", `{"input":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}],"store":true}`, true},
		{"legacy image URL", `{"input":[{"content":[{"type":"image_url","image_url":{"url":"DATA:IMAGE/JPEG;base64,AQID"}}]}]}`, true},
		{"paired image tool output", `{"input":[{"type":"function_call","call_id":"c"},{"type":"function_call_output","call_id":"c","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}]}`, true},
		{"remote image", `{"input":[{"type":"input_image","image_url":"https://example.com/photo.png"}]}`, false},
		{"file reference", `{"input":[{"type":"input_image","file_id":"file_image"}]}`, false},
		{"text mentions data URL", `{"input":[{"content":[{"type":"input_text","text":"data:image/png;base64,AQID"}]}]}`, false},
		{"tool declaration mentions image", `{"input":"hello","tools":[{"type":"function","parameters":{"type":"input_image","image_url":"data:image/png;base64,AQID"}}]}`, false},
		{"incremental image", `{"previous_response_id":"resp_previous","input":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}`, false},
		{"unpaired image tool output", `{"input":[{"type":"function_call_output","call_id":"missing","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			require.Equal(t, tc.want, svc.shouldBypassOpenAIWSForSize(context.Background(), c, account, body, body))
			require.Equal(t, tc.want, svc.shouldBridgeOpenAIWSHTTP(account, body, ""))
			require.Equal(t, tc.want, svc.shouldBridgeOpenAIWSPassthroughFirstMessage(account, body))
		})
	}
	image := []byte(`{"input":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = false
	require.False(t, svc.shouldBypassOpenAIWSForSize(context.Background(), c, account, image, image))
	require.False(t, svc.shouldBridgeOpenAIWSHTTP(account, image, ""))
	require.False(t, svc.shouldBridgeOpenAIWSPassthroughFirstMessage(account, image))
}

func TestOpenAIWSInitialRoutingDoesNotRelaxReplaySafety(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	account := &Account{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, field := range []string{`"store":true`, `"background":true`, `"conversation":"conv_1"`, `"tools":[{"type":"image_generation"}]`, `"tools":[{"type":"mcp"}]`} {
		t.Run(field, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}],` + field + `}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			require.True(t, svc.shouldBypassOpenAIWSForSize(context.Background(), c, account, body, body))
			require.False(t, svc.prepareOpenAIWSHTTPFallback(context.Background(), c, account, wrapOpenAIWSFallback("message_too_big", errors.New("close 1009")), body, body))
		})
	}
}
