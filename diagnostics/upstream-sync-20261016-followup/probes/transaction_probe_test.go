//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/websearch"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUpstreamFollowupTransactionProbe(t *testing.T) {
	cfg := &config.Config{}
	svc := &OpenAIGatewayService{cfg: cfg, toolCorrector: NewCodexToolCorrector()}
	for _, stream := range []bool{false, true} {
		raw := `{"id":"resp_probe","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":3,"output_tokens":2}}`
		if stream {
			raw = `data: {"type":"response.completed","response":` + raw + "}\n\n"
		}
		c, rec := newAntigravityCompatContext(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))}
		if stream {
			_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformGrok}, time.Now(), "grok-4.5", "grok-4.5")
			require.NoError(t, err)
			found := false
			for _, line := range strings.Split(rec.Body.String(), "\n") {
				if data, ok := strings.CutPrefix(line, "data: "); ok {
					found = found || gjson.Get(data, "response.created_at").Int() > 0
				}
			}
			fmt.Printf("OBSERVED responses_stream_created_at=%t\n", found)
		} else {
			_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, &Account{Platform: PlatformGrok}, "grok-4.5", "grok-4.5")
			require.NoError(t, err)
			fmt.Printf("OBSERVED responses_json_created_at=%t\n", gjson.Get(rec.Body.String(), "created_at").Int() > 0)
		}
	}
	SetWebSearchManager(websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "fixture"}}, nil))
	defer SetWebSearchManager(nil)
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{Enabled: true, Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "fixture"}}})
	defer clearGlobalWebSearchConfig()
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{featureKeyWebSearchEmulation: WebSearchModeEnabled}}
	searchSvc := &GatewayService{settingService: newSettingServiceForWebSearchTest(true)}
	fmt.Printf("OBSERVED openai_apikey_search=%t\n", searchSvc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	fmt.Printf("OBSERVED oauth_gpt55_supported=%t apikey_gpt55_supported=%t\n", oauth.IsModelSupported("gpt-5.5"), account.IsModelSupported("gpt-5.5"))
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1}}`))}}}
	geminiSvc := &GeminiMessagesCompatService{cfg: cfg, httpUpstream: upstream}
	body := []byte(`{"model":"gemini-3.8-flash","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}]}`)
	c, _ := newAntigravityCompatContext(http.MethodPost, "/v1/chat/completions", body)
	_, err := geminiSvc.ForwardAsChatCompletions(context.Background(), c, &Account{ID: 611, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}, body)
	require.NoError(t, err)
	fmt.Printf("OBSERVED gemini_forwarded_thinking=%q\n", gjson.GetBytes(upstream.requestBodies[0], "generationConfig.thinkingConfig.thinkingLevel").String())
}

func BenchmarkUpstreamFollowupModelSupport(b *testing.B) {
	for _, kind := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		b.Run(kind, func(b *testing.B) {
			account := &Account{Platform: PlatformOpenAI, Type: kind}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				account.IsModelSupported("gpt-6.1-sol")
			}
		})
	}
}
