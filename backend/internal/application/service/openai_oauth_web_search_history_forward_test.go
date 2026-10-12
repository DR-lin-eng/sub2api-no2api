//go:build unit

package service

import (
	"context"
	"fmt"
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

// This is also the identical baseline/modified/rollback transaction fixture.
// It drives the real Forward path, rather than merely exercising the helper.
func TestForwardOAuthWebSearchHistory20261009(t *testing.T) {
	for _, tc := range []struct {
		name              string
		passthrough, lite bool
	}{
		{"transform", false, false}, {"passthrough", true, false}, {"transform-lite", false, true}, {"passthrough-lite", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
			if tc.lite {
				c.Request.Header.Set(responsesLiteHeader, "true")
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\ndata: [DONE]\n\n")),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 501, Name: "history-fixture", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
				Credentials: map[string]any{"access_token": "fixture-oauth", "chatgpt_account_id": "fixture-account"},
				Extra:       map[string]any{"openai_passthrough": tc.passthrough},
			}
			input := []byte(`{"model":"gpt-6-astra","stream":true,"instructions":"fixture","input":[{"type":"web_search_call","status":"completed","action":{"type":"search","query":"q"}},{"type":"message","role":"user","content":"summarize"},{"type":"compaction_trigger"}],"tools":[],"tool_choice":"auto"}`)
			result, err := svc.Forward(context.Background(), c, account, input)
			require.NoError(t, err)
			require.NotNil(t, result)
			body := upstream.lastBody
			items := gjson.GetBytes(body, "input").Array()
			carrier := "tools"
			if tc.lite {
				carrier = `input.#(type=="additional_tools").tools`
			}
			fmt.Printf("OBSERVED path=%s declared=%s choice=%s trigger_last=%v input_history=%s\n", tc.name, gjson.GetBytes(body, carrier+".0.type").String(), gjson.GetBytes(body, "tool_choice").String(), items[len(items)-1].Get("type").String() == "compaction_trigger", items[0].Get("type").String())
			require.Equal(t, "web_search", gjson.GetBytes(body, carrier+".0.type").String())
			require.Equal(t, "false", gjson.GetBytes(body, carrier+".0.external_web_access").Raw)
			require.Equal(t, "none", gjson.GetBytes(body, "tool_choice").String())
			require.Equal(t, "compaction_trigger", items[len(items)-1].Get("type").String())
			if tc.lite {
				require.Empty(t, gjson.GetBytes(body, "tools").Array())
			}
		})
	}
}
